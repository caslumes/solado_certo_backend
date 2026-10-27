package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrTooLarge        = errors.New("file too large")
	ErrUnsupportedType = errors.New("unsupported file type")
)

type AllowedTypes map[string]string

var (
	Images    = AllowedTypes{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	Documents = AllowedTypes{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "application/pdf": ".pdf"}
)

type Upload struct {
	Content     []byte
	ContentType string
	Extension   string
}

func ReadUpload(content io.Reader, maxSize int64, allowed AllowedTypes) (*Upload, error) {
	data, err := io.ReadAll(io.LimitReader(content, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrTooLarge, maxSize)
	}

	contentType := http.DetectContentType(data)
	extension, ok := allowed[contentType]
	if !ok || len(data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, contentType)
	}

	return &Upload{Content: data, ContentType: contentType, Extension: extension}, nil
}

func (u *Upload) Reader() io.Reader {
	return bytes.NewReader(u.Content)
}

func KeyFromPublicPath(path string) (string, bool) {
	key, ok := strings.CutPrefix(path, publicPathPrefix)
	if !ok || !keyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}
