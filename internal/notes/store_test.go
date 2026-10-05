package notes

import (
	"errors"
	"testing"
)

func TestCRUDWithETag(t *testing.T) {
	s := NewStore()
	n, err := s.Create("a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 1 {
		t.Fatal("expected one note")
	}
	if _, err := s.Update(n.ID, `"wrong"`, "x", "y"); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected precondition failure, got %v", err)
	}
	updated, err := s.Update(n.ID, n.ETag(), "x", "y")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.ETag() == n.ETag() {
		t.Fatal("etag must change after update")
	}
	if err := s.Delete(n.ID, n.ETag()); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatal("stale etag must not delete")
	}
	if err := s.Delete(n.ID, updated.ETag()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expected not found")
	}
}

func TestETagMatches(t *testing.T) {
	if !ETagMatches("*", `"x"`) || !ETagMatches("", `"x"`) {
		t.Fatal("wildcard and empty must match")
	}
	if !ETagMatches(`"a", W/"x"`, `"x"`) {
		t.Fatal("list with weak tag must match")
	}
	if ETagMatches(`"a"`, `"x"`) {
		t.Fatal("mismatch expected")
	}
}
