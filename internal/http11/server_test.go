package http11

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"notes/internal/notes"
	"notes/internal/rest"
)

func TestKeepAliveAndChunked(t *testing.T) {
	srv := New(rest.New(notes.NewStore()))
	client, server := net.Pipe()
	go srv.ServeConn(server)
	r := bufio.NewReader(client)

	go client.Write([]byte("POST /notes HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n5\r\n{\"tit\r\n16\r\nle\":\"a\",\"content\":\"b\"}\r\n0\r\n\r\n"))
	status, headers, body := readResponse(t, r)
	if status != "201" || headers["ETag"] == "" || !strings.Contains(body, `"title": "a"`) {
		t.Fatalf("bad create: %s %v %s", status, headers, body)
	}

	go client.Write([]byte("GET /notes/1 HTTP/1.1\r\nIf-None-Match: " + headers["ETag"] + "\r\n\r\n"))
	status, _, _ = readResponse(t, r)
	if status != "304" {
		t.Fatalf("expected 304 on same connection, got %s", status)
	}

	go client.Write([]byte("GET /nope HTTP/1.1\r\nConnection: close\r\n\r\n"))
	status, headers, _ = readResponse(t, r)
	if status != "404" || headers["Connection"] != "close" {
		t.Fatalf("expected 404 close, got %s %v", status, headers)
	}
}

func TestHTTP09StyleLineOnHTTP11Port(t *testing.T) {
	srv := New(rest.New(notes.NewStore()))
	client, server := net.Pipe()
	go srv.ServeConn(server)
	go client.Write([]byte("GET /notes\r\n"))
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "{") {
		t.Fatalf("expected bare body, got %q %v", line, err)
	}
}

func readResponse(t *testing.T, r *bufio.Reader) (string, map[string]string, string) {
	t.Helper()
	statusLine, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	status := strings.Fields(statusLine)[1]
	headers := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		k, v, _ := strings.Cut(line, ":")
		headers[k] = strings.TrimSpace(v)
	}
	body := ""
	if cl := headers["Content-Length"]; cl != "" {
		var n int
		for _, c := range cl {
			n = n*10 + int(c-'0')
		}
		buf := make([]byte, n)
		if _, err := io_ReadFull(r, buf); err != nil {
			t.Fatal(err)
		}
		body = string(buf)
	}
	return status, headers, body
}

func io_ReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
