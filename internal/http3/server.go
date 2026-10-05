package http3

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"notes/internal/rest"
)

type Server struct {
	handler *rest.Handler
}

func New(h *rest.Handler) *Server { return &Server{handler: h} }

func (s *Server) Serve(udp *net.UDPConn, tlsConf *tls.Config) error {
	tr := &quic.Transport{Conn: udp}
	ln, err := tr.ListenEarly(http3.ConfigureTLSConfig(tlsConf), &quic.Config{MaxIdleTimeout: 30 * time.Second})
	if err != nil {
		return err
	}
	srv := &http3.Server{Handler: http.HandlerFunc(s.serveHTTP)}
	return srv.ServeListener(ln)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := rest.Request{Method: r.Method, Path: r.URL.RequestURI(), Headers: map[string]string{}, Body: body}
	for k := range r.Header {
		req.Headers[k] = r.Header.Get(k)
	}
	res := s.handler.Handle(req)
	for k, v := range res.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Server", "notes-http3")
	w.WriteHeader(res.Status)
	if res.Status != 304 && res.Status != 204 {
		_, _ = w.Write(res.Body)
	}
}

func SelfSignedTLS(hosts ...string) (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "notes-http3"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * 365 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS13}, nil
}
