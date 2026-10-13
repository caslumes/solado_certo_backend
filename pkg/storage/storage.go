package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
)

var (
	ErrNotFound   = errors.New("file not found")
	ErrInvalidKey = errors.New("invalid file key")
)

type Area string

const (
	AreaPublic  Area = "public"
	AreaPrivate Area = "private"
)

func (a Area) IsValid() bool {
	return a == AreaPublic || a == AreaPrivate
}

type File struct {
	io.ReadCloser
	Size int64
}

type Storage interface {
	Save(ctx context.Context, area Area, folder string, extension string, content io.Reader) (key string, err error)
	Open(ctx context.Context, area Area, key string) (*File, error)
	Delete(ctx context.Context, area Area, key string) error
}

const publicPathPrefix = "/files/public/"

func PublicPath(key string) string {
	return publicPathPrefix + key
}

var contentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".pdf":  "application/pdf",
}

func ContentType(key string) string {
	if contentType, ok := contentTypes[strings.ToLower(path.Ext(key))]; ok {
		return contentType
	}
	return "application/octet-stream"
}

var (
	folderPattern    = regexp.MustCompile(`^[a-z0-9_-]+$`)
	extensionPattern = regexp.MustCompile(`^\.[a-z0-9]+$`)
	keyPattern       = regexp.MustCompile(`^[a-z0-9_-]+/[0-9a-f]{32}\.[a-z0-9]+$`)
)
