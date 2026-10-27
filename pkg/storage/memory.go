package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

type MemoryStorage struct {
	mu    sync.Mutex
	files map[string][]byte
	next  int
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{files: map[string][]byte{}}
}

func (s *MemoryStorage) Save(_ context.Context, area Area, folder string, extension string, content io.Reader) (string, error) {
	if !area.IsValid() || !folderPattern.MatchString(folder) || !extensionPattern.MatchString(extension) {
		return "", ErrInvalidKey
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	key := fmt.Sprintf("%s/%032x%s", folder, s.next, extension)
	s.files[string(area)+"/"+key] = data
	return key, nil
}

func (s *MemoryStorage) Open(_ context.Context, area Area, key string) (*File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[string(area)+"/"+key]
	if !ok {
		return nil, ErrNotFound
	}
	return &File{ReadCloser: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data))}, nil
}

func (s *MemoryStorage) Delete(_ context.Context, area Area, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, string(area)+"/"+key)
	return nil
}

func (s *MemoryStorage) Has(area Area, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.files[string(area)+"/"+key]
	return ok
}

func (s *MemoryStorage) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.files)
}
