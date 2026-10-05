package rest

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"notes/internal/notes"
)

type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

type Handler struct {
	store *notes.Store
}

func New(store *notes.Store) *Handler {
	return &Handler{store: store}
}

type noteList struct {
	XMLName xml.Name     `json:"-" xml:"notes"`
	Notes   []notes.Note `json:"notes" xml:"note"`
}

type notePayload struct {
	XMLName xml.Name `json:"-" xml:"note"`
	Title   string   `json:"title" xml:"title"`
	Content string   `json:"content" xml:"content"`
}

type errorPayload struct {
	XMLName xml.Name `json:"-" xml:"error"`
	Error   string   `json:"error" xml:"message"`
}

func header(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func (h *Handler) Handle(req Request) Response {
	format := pickFormat(header(req.Headers, "Accept"), header(req.Headers, "Content-Type"))
	segments := strings.Split(strings.Trim(strings.SplitN(req.Path, "?", 2)[0], "/"), "/")
	if segments[0] != "notes" || len(segments) > 2 {
		return h.reply(format, 404, nil, errorPayload{Error: "not found"})
	}
	id := ""
	if len(segments) == 2 {
		id = segments[1]
	}
	switch {
	case req.Method == "GET" && id == "":
		return h.reply(format, 200, nil, noteList{Notes: h.store.List()})
	case req.Method == "POST" && id == "":
		p, err := decode(req, format)
		if err != nil {
			return h.reply(format, 400, nil, errorPayload{Error: err.Error()})
		}
		n, err := h.store.Create(p.Title, p.Content)
		if err != nil {
			return h.fail(format, err)
		}
		return h.reply(format, 201, map[string]string{"ETag": n.ETag(), "Location": "/notes/" + n.ID}, wrap(n))
	case req.Method == "GET" && id != "":
		n, err := h.store.Get(id)
		if err != nil {
			return h.fail(format, err)
		}
		if inm := header(req.Headers, "If-None-Match"); inm != "" && notes.ETagMatches(inm, n.ETag()) {
			return Response{Status: 304, Headers: map[string]string{"ETag": n.ETag()}}
		}
		return h.reply(format, 200, map[string]string{"ETag": n.ETag()}, wrap(n))
	case (req.Method == "PUT" || req.Method == "PATCH") && id != "":
		p, err := decode(req, format)
		if err != nil {
			return h.reply(format, 400, nil, errorPayload{Error: err.Error()})
		}
		if req.Method == "PATCH" {
			if cur, err := h.store.Get(id); err == nil {
				if p.Title == "" {
					p.Title = cur.Title
				}
				if p.Content == "" {
					p.Content = cur.Content
				}
			}
		}
		n, err := h.store.Update(id, header(req.Headers, "If-Match"), p.Title, p.Content)
		if err != nil {
			return h.failWithETag(format, err, n)
		}
		return h.reply(format, 200, map[string]string{"ETag": n.ETag()}, wrap(n))
	case req.Method == "DELETE" && id != "":
		if err := h.store.Delete(id, header(req.Headers, "If-Match")); err != nil {
			return h.fail(format, err)
		}
		return Response{Status: 204, Headers: map[string]string{}}
	}
	return h.reply(format, 405, map[string]string{"Allow": "GET, POST, PUT, PATCH, DELETE"}, errorPayload{Error: "method not allowed"})
}

type noteDoc struct {
	XMLName xml.Name `json:"-" xml:"note"`
	notes.Note
}

func wrap(n notes.Note) any { return noteDoc{Note: n} }

func (h *Handler) fail(format string, err error) Response {
	return h.failWithETag(format, err, notes.Note{})
}

func (h *Handler) failWithETag(format string, err error, n notes.Note) Response {
	status := 500
	switch {
	case errors.Is(err, notes.ErrNotFound):
		status = 404
	case errors.Is(err, notes.ErrPreconditionFailed):
		status = 412
	case errors.Is(err, notes.ErrInvalid):
		status = 422
	}
	hdr := map[string]string{}
	if status == 412 && n.ID != "" {
		hdr["ETag"] = n.ETag()
	}
	return h.reply(format, status, hdr, errorPayload{Error: err.Error()})
}

func (h *Handler) reply(format string, status int, hdr map[string]string, payload any) Response {
	if hdr == nil {
		hdr = map[string]string{}
	}
	var body []byte
	if format == "xml" {
		body, _ = xml.MarshalIndent(payload, "", "  ")
		body = append([]byte(xml.Header), body...)
		hdr["Content-Type"] = "application/xml; charset=utf-8"
	} else {
		body, _ = json.MarshalIndent(payload, "", "  ")
		hdr["Content-Type"] = "application/json; charset=utf-8"
	}
	body = append(body, '\n')
	return Response{Status: status, Headers: hdr, Body: body}
}

func pickFormat(accept, contentType string) string {
	if strings.Contains(accept, "xml") {
		return "xml"
	}
	if strings.Contains(accept, "json") {
		return "json"
	}
	if strings.Contains(contentType, "xml") {
		return "xml"
	}
	return "json"
}

func decode(req Request, format string) (notePayload, error) {
	var p notePayload
	if len(req.Body) == 0 {
		return p, fmt.Errorf("empty body")
	}
	ct := header(req.Headers, "Content-Type")
	bodyIsXML := strings.Contains(ct, "xml") || (ct == "" && format == "xml")
	var err error
	if bodyIsXML {
		err = xml.Unmarshal(req.Body, &p)
	} else {
		err = json.Unmarshal(req.Body, &p)
	}
	if err != nil {
		return p, fmt.Errorf("malformed body: %v", err)
	}
	return p, nil
}

func StatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 204:
		return "No Content"
	case 304:
		return "Not Modified"
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 412:
		return "Precondition Failed"
	case 413:
		return "Payload Too Large"
	case 422:
		return "Unprocessable Entity"
	case 505:
		return "HTTP Version Not Supported"
	default:
		return "Internal Server Error"
	}
}
