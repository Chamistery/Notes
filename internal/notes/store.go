package notes

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound           = errors.New("note not found")
	ErrPreconditionFailed = errors.New("precondition failed")
	ErrInvalid            = errors.New("invalid note")
)

type Note struct {
	ID        string    `json:"id" xml:"id"`
	Title     string    `json:"title" xml:"title"`
	Content   string    `json:"content" xml:"content"`
	CreatedAt time.Time `json:"created_at" xml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" xml:"updated_at"`
	Version   uint64    `json:"version" xml:"version"`
}

func (n Note) ETag() string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", n.ID, n.Version, n.Title, n.Content)))
	return `"` + hex.EncodeToString(h[:8]) + `"`
}

func ETagMatches(ifMatch, etag string) bool {
	ifMatch = strings.TrimSpace(ifMatch)
	if ifMatch == "" || ifMatch == "*" {
		return true
	}
	for _, candidate := range strings.Split(ifMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

type Store struct {
	mu    sync.RWMutex
	notes map[string]Note
	seq   uint64
	now   func() time.Time
}

func NewStore() *Store {
	return &Store{notes: make(map[string]Note), now: time.Now}
}

func (s *Store) Create(title, content string) (Note, error) {
	if strings.TrimSpace(title) == "" {
		return Note{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	ts := s.now().UTC()
	n := Note{ID: fmt.Sprintf("%d", s.seq), Title: title, Content: content, CreatedAt: ts, UpdatedAt: ts, Version: 1}
	s.notes[n.ID] = n
	return n, nil
}

func (s *Store) Get(id string) (Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notes[id]
	if !ok {
		return Note{}, ErrNotFound
	}
	return n, nil
}

func (s *Store) List() []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Note, 0, len(s.notes))
	for _, n := range s.notes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt) || (out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID < out[j].ID)
	})
	return out
}

func (s *Store) Update(id, ifMatch, title, content string) (Note, error) {
	if strings.TrimSpace(title) == "" {
		return Note{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return Note{}, ErrNotFound
	}
	if !ETagMatches(ifMatch, n.ETag()) {
		return n, ErrPreconditionFailed
	}
	n.Title, n.Content = title, content
	n.Version++
	n.UpdatedAt = s.now().UTC()
	s.notes[id] = n
	return n, nil
}

func (s *Store) Delete(id, ifMatch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return ErrNotFound
	}
	if !ETagMatches(ifMatch, n.ETag()) {
		return ErrPreconditionFailed
	}
	delete(s.notes, id)
	return nil
}
