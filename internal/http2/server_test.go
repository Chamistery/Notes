package http2

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/http2"

	"notes/internal/notes"
	"notes/internal/rest"
)

func TestH2cRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go New(rest.New(notes.NewStore())).Serve(ln)

	client := &http.Client{Transport: &http2.Transport{AllowHTTP: true, DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr)
	}}}
	base := "http://" + ln.Addr().String()

	res, err := client.Post(base+"/notes", "application/json", strings.NewReader(`{"title":"h2","content":"body"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 201 || res.Header.Get("Etag") == "" || !strings.Contains(string(b), `"h2"`) {
		t.Fatalf("bad create: %d %v %s", res.StatusCode, res.Header, b)
	}

	req, _ := http.NewRequest("GET", base+"/notes/1", nil)
	req.Header.Set("If-None-Match", res.Header.Get("Etag"))
	res2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != 304 {
		t.Fatalf("expected 304, got %d", res2.StatusCode)
	}

	req, _ = http.NewRequest("DELETE", base+"/notes/1", nil)
	req.Header.Set("If-Match", `"stale"`)
	res3, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()
	if res3.StatusCode != 412 {
		t.Fatalf("expected 412, got %d", res3.StatusCode)
	}
}
