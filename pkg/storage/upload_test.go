package storage

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

var (
	jpegHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	pngHeader  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	webpHeader = []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
	pdfHeader  = []byte("%PDF-1.7\n")
)

func TestReadUploadDetectsTypeFromContent(t *testing.T) {
	tests := map[string]struct {
		content   []byte
		allowed   AllowedTypes
		extension string
	}{
		"jpeg":            {jpegHeader, Images, ".jpg"},
		"png":             {pngHeader, Images, ".png"},
		"webp":            {webpHeader, Images, ".webp"},
		"pdf as document": {pdfHeader, Documents, ".pdf"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			upload, err := ReadUpload(bytes.NewReader(test.content), 1024, test.allowed)
			if err != nil {
				t.Fatalf("ReadUpload: %v", err)
			}
			if upload.Extension != test.extension || !bytes.Equal(upload.Content, test.content) {
				t.Fatalf("upload = %+v", upload)
			}
		})
	}
}

func TestReadUploadRejectsUnsupportedContent(t *testing.T) {
	tests := map[string][]byte{
		"pdf as image":          pdfHeader,
		"html named like a jpg": []byte("<html><script>alert(1)</script></html>"),
		"plain text":            []byte("not an image"),
		"empty":                 {},
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ReadUpload(bytes.NewReader(content), 1024, Images)
			if !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("err = %v, want %v", err, ErrUnsupportedType)
			}
		})
	}
}

func TestReadUploadEnforcesSize(t *testing.T) {
	exact := append(append([]byte{}, pngHeader...), make([]byte, 92)...)

	if _, err := ReadUpload(bytes.NewReader(exact), 100, Images); err != nil {
		t.Fatalf("file at the limit: %v", err)
	}
	if _, err := ReadUpload(bytes.NewReader(append(exact, 0)), 100, Images); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("file over the limit: err = %v, want %v", err, ErrTooLarge)
	}
}

func TestKeyFromPublicPath(t *testing.T) {
	key := "ads/0123456789abcdef0123456789abcdef.jpg"
	tests := map[string]bool{
		PublicPath(key):                       true,
		"https://placehold.co/600x600?text=a": false,
		"/files/public/../../secret":          false,
		"/files/private/" + key:               false,
		"":                                    false,
	}

	for path, want := range tests {
		got, ok := KeyFromPublicPath(path)
		if ok != want || (ok && got != key) {
			t.Errorf("KeyFromPublicPath(%q) = %q, %v", path, got, ok)
		}
	}
}

func TestMemoryStorage(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	key, err := s.Save(ctx, AreaPublic, "ads", ".png", strings.NewReader("png"))
	if err != nil || !keyPattern.MatchString(key) || !s.Has(AreaPublic, key) || s.Has(AreaPrivate, key) {
		t.Fatalf("Save: key = %q, err = %v", key, err)
	}
	file, err := s.Open(ctx, AreaPublic, key)
	if err != nil || file.Size != 3 {
		t.Fatalf("Open: %+v, %v", file, err)
	}
	if err := s.Delete(ctx, AreaPublic, key); err != nil || s.Count() != 0 {
		t.Fatalf("Delete: err = %v, count = %d", err, s.Count())
	}
	if _, err := s.Open(ctx, AreaPublic, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete: %v", err)
	}
	if _, err := s.Save(ctx, AreaPublic, "../x", ".png", strings.NewReader("")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Save with bad folder: %v", err)
	}
}
