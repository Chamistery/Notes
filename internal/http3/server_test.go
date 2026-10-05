package http3

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/quic-go/quic-go/http3"

	"notes/internal/notes"
	"notes/internal/rest"
)

func TestHTTP3RoundTrip(t *testing.T) {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	tlsConf, err := SelfSignedTLS()
	if err != nil {
		t.Fatal(err)
	}
	go New(rest.New(notes.NewStore())).Serve(udp, tlsConf)

	tr := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer tr.Close()
	client := &http.Client{Transport: tr}
	base := "https://" + udp.LocalAddr().String()

	res, err := client.Post(base+"/notes", "application/json", strings.NewReader(`{"title":"h3","content":"udp"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 201 || res.Header.Get("Etag") == "" || !strings.Contains(string(b), `"h3"`) {
		t.Fatalf("bad create: %d %v %s", res.StatusCode, res.Header, b)
	}
	req, _ := http.NewRequest("PUT", base+"/notes/1", strings.NewReader(`{"title":"h3","content":"v2"}`))
	req.Header.Set("If-Match", res.Header.Get("Etag"))
	res2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != 200 || res2.Header.Get("Etag") == res.Header.Get("Etag") {
		t.Fatalf("expected updated etag, got %d %v", res2.StatusCode, res2.Header)
	}
}
