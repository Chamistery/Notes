package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"notes/internal/grpcsrv"
	"notes/internal/http09"
	"notes/internal/http11"
	"notes/internal/http2"
	"notes/internal/http3"
	"notes/internal/notes"
	"notes/internal/rest"
)

func main() {
	http09Addr := flag.String("http09", ":8009", "HTTP/0.9 listen address (TCP)")
	http11Addr := flag.String("http11", ":8011", "HTTP/1.1 listen address (TCP)")
	http2Addr := flag.String("http2", ":8020", "HTTP/2 cleartext (h2c prior knowledge) listen address (TCP)")
	http3Addr := flag.String("http3", ":8030", "HTTP/3 listen address (UDP, self-signed TLS)")
	grpcAddr := flag.String("grpc", ":9090", "gRPC listen address (TCP)")
	flag.Parse()

	store := notes.NewStore()
	handler := rest.New(store)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 5)

	tcp := func(name, addr string, serve func(net.Listener) error) {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("%s: listen %s: %v", name, addr, err)
		}
		log.Printf("%-8s listening on tcp %s", name, ln.Addr())
		go func() {
			errCh <- serve(ln)
		}()
		go func() {
			<-ctx.Done()
			ln.Close()
		}()
	}

	tcp("http/0.9", *http09Addr, http09.New(handler).Serve)
	tcp("http/1.1", *http11Addr, http11.New(handler).Serve)
	tcp("http/2", *http2Addr, http2.New(handler).Serve)
	grpcServer := grpcsrv.NewServer(store)
	tcp("grpc", *grpcAddr, grpcServer.Serve)

	udpAddr, err := net.ResolveUDPAddr("udp", *http3Addr)
	if err != nil {
		log.Fatalf("http/3: %v", err)
	}
	udp, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("http/3: listen %s: %v", *http3Addr, err)
	}
	tlsConf, err := http3.SelfSignedTLS()
	if err != nil {
		log.Fatalf("http/3: tls: %v", err)
	}
	log.Printf("%-8s listening on udp %s", "http/3", udp.LocalAddr())
	go func() { errCh <- http3.New(handler).Serve(udp, tlsConf) }()
	go func() {
		<-ctx.Done()
		udp.Close()
	}()

	select {
	case <-ctx.Done():
		log.Print("shutting down")
		grpcServer.GracefulStop()
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Fatalf("server error: %v", err)
		}
	}
}
