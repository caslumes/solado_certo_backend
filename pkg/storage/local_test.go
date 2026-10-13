package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func newLocalStorage(t *testing.T) (*LocalStorage, string) {
	t.Helper()
	root := t.TempDir()
	s, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	return s, root
}

func save(t *testing.T, s *LocalStorage, area Area, content string) string {
	t.Helper()
	key, err := s.Save(context.Background(), area, "ads", ".jpg", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return key
}

func read(t *testing.T, file *File) string {
	t.Helper()
	defer file.Close()
	b, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

func TestNewLocalStorageCreatesAreas(t *testing.T) {
	_, root := newLocalStorage(t)

	for _, area := range []string{"public", "private"} {
		if info, err := os.Stat(filepath.Join(root, area)); err != nil || !info.IsDir() {
			t.Fatalf("area %s was not created: %v", area, err)
		}
	}
}

func TestSaveAndOpen(t *testing.T) {
	s, root := newLocalStorage(t)

	key := save(t, s, AreaPublic, "photo bytes")

	if !regexp.MustCompile(`^ads/[0-9a-f]{32}\.jpg$`).MatchString(key) {
		t.Fatalf("key = %q", key)
	}
	file, err := s.Open(context.Background(), AreaPublic, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if file.Size != int64(len("photo bytes")) {
		t.Fatalf("size = %d", file.Size)
	}
	if got := read(t, file); got != "photo bytes" {
		t.Fatalf("content = %q", got)
	}

	entries, err := os.ReadDir(filepath.Join(root, "public", "ads"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("files in folder = %d, want only the saved file", len(entries))
	}
}

func TestSaveGivesEachFileANewKey(t *testing.T) {
	s, _ := newLocalStorage(t)

	if first, second := save(t, s, AreaPublic, "a"), save(t, s, AreaPublic, "a"); first == second {
		t.Fatalf("both files got key %q", first)
	}
}

func TestSaveRejectsInvalidArguments(t *testing.T) {
	s, root := newLocalStorage(t)
	tests := map[string]struct {
		area      Area
		folder    string
		extension string
	}{
		"unknown area":             {"secret", "ads", ".jpg"},
		"folder with traversal":    {AreaPublic, "../ads", ".jpg"},
		"nested folder":            {AreaPublic, "ads/2026", ".jpg"},
		"empty folder":             {AreaPublic, "", ".jpg"},
		"extension without dot":    {AreaPublic, "ads", "jpg"},
		"extension with separator": {AreaPublic, "ads", ".jpg/x"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := s.Save(context.Background(), test.area, test.folder, test.extension, strings.NewReader("x"))
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("err = %v, want %v", err, ErrInvalidKey)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(root, "ads")); !os.IsNotExist(err) {
		t.Fatal("a folder was created outside the areas")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

func TestSaveLeavesNoFileWhenContentFails(t *testing.T) {
	s, root := newLocalStorage(t)

	if _, err := s.Save(context.Background(), AreaPublic, "ads", ".jpg", failingReader{}); err == nil {
		t.Fatal("Save succeeded with a failing reader")
	}

	entries, err := os.ReadDir(filepath.Join(root, "public", "ads"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("files left behind: %d", len(entries))
	}
}

func TestOpenKeepsAreasApart(t *testing.T) {
	s, _ := newLocalStorage(t)
	key := save(t, s, AreaPrivate, "prescription")

	_, err := s.Open(context.Background(), AreaPublic, key)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrNotFound)
	}
}

func TestOpenRejectsKeysNotCreatedBySave(t *testing.T) {
	s, root := newLocalStorage(t)
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	keys := []string{
		"../secret.txt",
		"ads/../../secret.txt",
		"/etc/passwd",
		"ads",
		"ads/photo.jpg",
		"ads/0123456789abcdef0123456789abcdef",
		"",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			_, err := s.Open(context.Background(), AreaPublic, key)
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("err = %v, want %v", err, ErrInvalidKey)
			}
		})
	}
}

func TestOpenReturnsNotFoundForMissingFile(t *testing.T) {
	s, _ := newLocalStorage(t)

	_, err := s.Open(context.Background(), AreaPublic, "ads/0123456789abcdef0123456789abcdef.jpg")

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrNotFound)
	}
}

func TestDeleteRemovesFileAndIsIdempotent(t *testing.T) {
	s, _ := newLocalStorage(t)
	key := save(t, s, AreaPublic, "photo")

	for i := range 2 {
		if err := s.Delete(context.Background(), AreaPublic, key); err != nil {
			t.Fatalf("Delete #%d: %v", i+1, err)
		}
	}

	if _, err := s.Open(context.Background(), AreaPublic, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete: err = %v, want %v", err, ErrNotFound)
	}
}

func TestDeleteRejectsInvalidKey(t *testing.T) {
	s, _ := newLocalStorage(t)

	if err := s.Delete(context.Background(), AreaPublic, "../secret.txt"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidKey)
	}
}

func TestOperationsStopOnCancelledContext(t *testing.T) {
	s, _ := newLocalStorage(t)
	key := save(t, s, AreaPublic, "photo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Save(ctx, AreaPublic, "ads", ".jpg", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save: err = %v", err)
	}
	if _, err := s.Open(ctx, AreaPublic, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open: err = %v", err)
	}
	if err := s.Delete(ctx, AreaPublic, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete: err = %v", err)
	}
}

func TestContentType(t *testing.T) {
	tests := map[string]string{
		"ads/a.jpg":  "image/jpeg",
		"ads/a.PNG":  "image/png",
		"ads/a.webp": "image/webp",
		"docs/a.pdf": "application/pdf",
		"ads/a.html": "application/octet-stream",
		"ads/a":      "application/octet-stream",
	}

	for key, want := range tests {
		if got := ContentType(key); got != want {
			t.Errorf("ContentType(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestPublicPath(t *testing.T) {
	if got := PublicPath("ads/a.jpg"); got != "/files/public/ads/a.jpg" {
		t.Fatalf("PublicPath = %q", got)
	}
}
