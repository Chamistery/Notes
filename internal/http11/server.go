package http11

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"notes/internal/rest"
)

const maxBody = 1 << 20

type Server struct {
	handler *rest.Handler
}

func New(h *rest.Handler) *Server { return &Server{handler: h} }

func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.ServeConn(conn)
	}
}

type request struct {
	rest.Request
	version   string
	keepAlive bool
}

var errBadRequest = errors.New("bad request")

func (s *Server) ServeConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		req, err := parseRequest(r)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			status := 400
			if errors.Is(err, errTooLarge) {
				status = 413
			} else if errors.Is(err, errVersion) {
				status = 505
			}
			writeResponse(conn, "HTTP/1.1", rest.Response{Status: status, Headers: map[string]string{"Content-Type": "text/plain"}, Body: []byte(err.Error() + "\n")}, false)
			return
		}
		if req.version == "HTTP/0.9" {
			res := s.handler.Handle(req.Request)
			_, _ = conn.Write(res.Body)
			return
		}
		res := s.handler.Handle(req.Request)
		writeResponse(conn, req.version, res, req.keepAlive)
		if !req.keepAlive {
			return
		}
	}
}

var (
	errTooLarge = errors.New("payload too large")
	errVersion  = errors.New("http version not supported")
)

func parseRequest(r *bufio.Reader) (request, error) {
	line, err := readLine(r)
	if err != nil {
		return request{}, err
	}
	for line == "" {
		if line, err = readLine(r); err != nil {
			return request{}, err
		}
	}
	parts := strings.Split(line, " ")
	req := request{Request: rest.Request{Headers: map[string]string{}}}
	switch len(parts) {
	case 2:
		if parts[0] != "GET" {
			return req, fmt.Errorf("%w: HTTP/0.9 allows only GET", errBadRequest)
		}
		req.Method, req.Path, req.version = parts[0], parts[1], "HTTP/0.9"
		return req, nil
	case 3:
		req.Method, req.Path, req.version = parts[0], parts[1], parts[2]
	default:
		return req, fmt.Errorf("%w: malformed request line", errBadRequest)
	}
	if req.version != "HTTP/1.1" && req.version != "HTTP/1.0" {
		return req, errVersion
	}
	for {
		line, err := readLine(r)
		if err != nil {
			return req, err
		}
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return req, fmt.Errorf("%w: malformed header %q", errBadRequest, line)
		}
		req.Headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	req.keepAlive = req.version == "HTTP/1.1"
	if conn := headerValue(req.Headers, "Connection"); conn != "" {
		req.keepAlive = !strings.EqualFold(conn, "close") && (req.version == "HTTP/1.1" || strings.EqualFold(conn, "keep-alive"))
	}
	if strings.EqualFold(headerValue(req.Headers, "Transfer-Encoding"), "chunked") {
		body, err := readChunked(r)
		if err != nil {
			return req, err
		}
		req.Body = body
		return req, nil
	}
	if cl := headerValue(req.Headers, "Content-Length"); cl != "" {
		n, err := strconv.Atoi(cl)
		if err != nil || n < 0 {
			return req, fmt.Errorf("%w: bad Content-Length", errBadRequest)
		}
		if n > maxBody {
			return req, errTooLarge
		}
		req.Body = make([]byte, n)
		if _, err := io.ReadFull(r, req.Body); err != nil {
			return req, err
		}
	}
	return req, nil
}

func readChunked(r *bufio.Reader) ([]byte, error) {
	var body []byte
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		sizeHex, _, _ := strings.Cut(line, ";")
		size, err := strconv.ParseInt(strings.TrimSpace(sizeHex), 16, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("%w: bad chunk size", errBadRequest)
		}
		if size == 0 {
			for {
				trailer, err := readLine(r)
				if err != nil || trailer == "" {
					return body, err
				}
			}
		}
		if int64(len(body))+size > maxBody {
			return nil, errTooLarge
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, err
		}
		body = append(body, chunk...)
		if _, err := readLine(r); err != nil {
			return nil, err
		}
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		if line != "" && errors.Is(err, io.EOF) {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func headerValue(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func writeResponse(w io.Writer, version string, res rest.Response, keepAlive bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d %s\r\n", version, res.Status, rest.StatusText(res.Status))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123))
	b.WriteString("Server: notes-http11\r\n")
	names := make([]string, 0, len(res.Headers))
	for k := range res.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(&b, "%s: %s\r\n", k, res.Headers[k])
	}
	if res.Status != 304 && res.Status != 204 {
		fmt.Fprintf(&b, "Content-Length: %d\r\n", len(res.Body))
	}
	if keepAlive {
		b.WriteString("Connection: keep-alive\r\n")
	} else {
		b.WriteString("Connection: close\r\n")
	}
	b.WriteString("\r\n")
	if res.Status != 304 && res.Status != 204 {
		b.Write(res.Body)
	}
	_, _ = io.WriteString(w, b.String())
}
