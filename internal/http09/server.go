package http09

import (
	"bufio"
	"log"
	"net"
	"strings"
	"time"

	"notes/internal/rest"
)

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
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && line == "" {
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "GET" {
		_, _ = conn.Write([]byte("HTTP/0.9 supports only: GET <path>\r\n"))
		return
	}
	if len(fields) > 2 {
		log.Printf("http09: version token %q ignored, answering as HTTP/0.9", fields[2])
	}
	res := s.handler.Handle(rest.Request{Method: "GET", Path: fields[1], Headers: map[string]string{}})
	if res.Status >= 400 {
		_, _ = conn.Write([]byte(rest.StatusText(res.Status) + "\r\n"))
	}
	_, _ = conn.Write(res.Body)
}
