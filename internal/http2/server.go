package http2

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"notes/internal/rest"
)

const (
	preface      = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	maxFrameSize = 16384
	maxBody      = 1 << 20
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
		go s.ServeConn(conn)
	}
}

type stream struct {
	headers map[string]string
	body    bytes.Buffer
}

type connState struct {
	conn    net.Conn
	framer  *http2.Framer
	writeMu sync.Mutex
	encMu   sync.Mutex
	encBuf  bytes.Buffer
	enc     *hpack.Encoder
	dec     *hpack.Decoder
	streams map[uint32]*stream
	handler *rest.Handler
}

func (s *Server) ServeConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if err := readPreface(conn); err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("http2: %v", err)
			_, _ = io.WriteString(conn, "HTTP/1.1 505 HTTP Version Not Supported\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
		}
		return
	}
	c := &connState{conn: conn, framer: http2.NewFramer(conn, conn), streams: map[uint32]*stream{}, handler: s.handler}
	c.enc = hpack.NewEncoder(&c.encBuf)
	c.dec = hpack.NewDecoder(4096, nil)
	c.framer.ReadMetaHeaders = c.dec
	c.framer.SetMaxReadFrameSize(maxFrameSize)
	if err := c.writeFrame(func() error {
		return c.framer.WriteSettings(http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 100}, http2.Setting{ID: http2.SettingMaxFrameSize, Val: maxFrameSize})
	}); err != nil {
		return
	}
	for {
		f, err := c.framer.ReadFrame()
		if err != nil {
			var se http2.StreamError
			if errors.As(err, &se) {
				_ = c.writeFrame(func() error { return c.framer.WriteRSTStream(se.StreamID, se.Code) })
				continue
			}
			if !errors.Is(err, io.EOF) {
				_ = c.writeFrame(func() error { return c.framer.WriteGoAway(0, http2.ErrCodeProtocol, []byte(err.Error())) })
			}
			return
		}
		if err := c.dispatch(f); err != nil {
			_ = c.writeFrame(func() error { return c.framer.WriteGoAway(0, http2.ErrCodeProtocol, []byte(err.Error())) })
			return
		}
	}
}

func readPreface(r io.Reader) error {
	buf := make([]byte, len(preface))
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if string(buf) != preface {
		return fmt.Errorf("bad connection preface %q", buf)
	}
	return nil
}

func (c *connState) writeFrame(fn func() error) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return fn()
}

func (c *connState) dispatch(f http2.Frame) error {
	switch f := f.(type) {
	case *http2.SettingsFrame:
		if f.IsAck() {
			return nil
		}
		return c.writeFrame(c.framer.WriteSettingsAck)
	case *http2.PingFrame:
		if f.IsAck() {
			return nil
		}
		return c.writeFrame(func() error { return c.framer.WritePing(true, f.Data) })
	case *http2.WindowUpdateFrame, *http2.PriorityFrame:
		return nil
	case *http2.GoAwayFrame:
		return io.EOF
	case *http2.RSTStreamFrame:
		delete(c.streams, f.StreamID)
		return nil
	case *http2.MetaHeadersFrame:
		st := &stream{headers: map[string]string{}}
		for _, hf := range f.Fields {
			st.headers[hf.Name] = hf.Value
		}
		if f.StreamEnded() {
			go c.respond(f.StreamID, st)
			return nil
		}
		c.streams[f.StreamID] = st
		return nil
	case *http2.DataFrame:
		st, ok := c.streams[f.StreamID]
		if !ok {
			return c.writeFrame(func() error { return c.framer.WriteRSTStream(f.StreamID, http2.ErrCodeStreamClosed) })
		}
		if st.body.Len()+len(f.Data()) > maxBody {
			delete(c.streams, f.StreamID)
			return c.writeFrame(func() error { return c.framer.WriteRSTStream(f.StreamID, http2.ErrCodeFlowControl) })
		}
		st.body.Write(f.Data())
		if n := uint32(f.Length); n > 0 {
			if err := c.writeFrame(func() error {
				if err := c.framer.WriteWindowUpdate(0, n); err != nil {
					return err
				}
				return c.framer.WriteWindowUpdate(f.StreamID, n)
			}); err != nil {
				return err
			}
		}
		if f.StreamEnded() {
			delete(c.streams, f.StreamID)
			go c.respond(f.StreamID, st)
		}
		return nil
	default:
		return nil
	}
}

func (c *connState) respond(id uint32, st *stream) {
	req := rest.Request{Method: st.headers[":method"], Path: st.headers[":path"], Headers: map[string]string{}, Body: st.body.Bytes()}
	for k, v := range st.headers {
		if !strings.HasPrefix(k, ":") {
			req.Headers[k] = v
		}
	}
	res := c.handler.Handle(req)
	c.encMu.Lock()
	defer c.encMu.Unlock()
	c.encBuf.Reset()
	_ = c.enc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprint(res.Status)})
	_ = c.enc.WriteField(hpack.HeaderField{Name: "server", Value: "notes-http2"})
	names := make([]string, 0, len(res.Headers))
	for k := range res.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		_ = c.enc.WriteField(hpack.HeaderField{Name: strings.ToLower(k), Value: res.Headers[k]})
	}
	body := res.Body
	if res.Status == 304 || res.Status == 204 {
		body = nil
	} else {
		_ = c.enc.WriteField(hpack.HeaderField{Name: "content-length", Value: fmt.Sprint(len(body))})
	}
	headerBlock := append([]byte(nil), c.encBuf.Bytes()...)
	_ = c.writeFrame(func() error {
		if err := c.framer.WriteHeaders(http2.HeadersFrameParam{StreamID: id, BlockFragment: headerBlock, EndHeaders: true, EndStream: len(body) == 0}); err != nil {
			return err
		}
		for len(body) > 0 {
			chunk := body
			if len(chunk) > maxFrameSize {
				chunk = body[:maxFrameSize]
			}
			body = body[len(chunk):]
			if err := c.framer.WriteData(id, len(body) == 0, chunk); err != nil {
				return err
			}
		}
		return nil
	})
}
