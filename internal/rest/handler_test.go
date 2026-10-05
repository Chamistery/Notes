package rest

import (
	"strings"
	"testing"

	"notes/internal/notes"
)

func TestJSONFlowWithETag(t *testing.T) {
	h := New(notes.NewStore())
	created := h.Handle(Request{Method: "POST", Path: "/notes", Headers: map[string]string{"Content-Type": "application/json"}, Body: []byte(`{"title":"t","content":"c"}`)})
	if created.Status != 201 || created.Headers["ETag"] == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	etag := created.Headers["ETag"]
	notMod := h.Handle(Request{Method: "GET", Path: "/notes/1", Headers: map[string]string{"If-None-Match": etag}})
	if notMod.Status != 304 {
		t.Fatalf("expected 304, got %d", notMod.Status)
	}
	stale := h.Handle(Request{Method: "PUT", Path: "/notes/1", Headers: map[string]string{"If-Match": `"nope"`}, Body: []byte(`{"title":"t2","content":"c2"}`)})
	if stale.Status != 412 {
		t.Fatalf("expected 412, got %d", stale.Status)
	}
	ok := h.Handle(Request{Method: "PUT", Path: "/notes/1", Headers: map[string]string{"If-Match": etag}, Body: []byte(`{"title":"t2","content":"c2"}`)})
	if ok.Status != 200 || ok.Headers["ETag"] == etag {
		t.Fatalf("expected update with new etag, got %+v", ok)
	}
	del := h.Handle(Request{Method: "DELETE", Path: "/notes/1", Headers: map[string]string{"If-Match": ok.Headers["ETag"]}})
	if del.Status != 204 {
		t.Fatalf("expected 204, got %d", del.Status)
	}
}

func TestXML(t *testing.T) {
	h := New(notes.NewStore())
	res := h.Handle(Request{Method: "POST", Path: "/notes", Headers: map[string]string{"Content-Type": "application/xml", "Accept": "application/xml"}, Body: []byte(`<note><title>x</title><content>y</content></note>`)})
	if res.Status != 201 || !strings.Contains(string(res.Body), "<title>x</title>") {
		t.Fatalf("unexpected xml response: %d %s", res.Status, res.Body)
	}
	list := h.Handle(Request{Method: "GET", Path: "/notes", Headers: map[string]string{"Accept": "application/xml"}})
	if !strings.HasPrefix(string(list.Body), "<?xml") || !strings.Contains(string(list.Body), "<notes>") {
		t.Fatalf("unexpected xml list: %s", list.Body)
	}
}
