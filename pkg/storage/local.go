package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	for _, area := range []Area{AreaPublic, AreaPrivate} {
		if err := os.MkdirAll(filepath.Join(root, string(area)), 0o755); err != nil {
			return nil, fmt.Errorf("create storage area %s: %w", area, err)
		}
	}

	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) Save(ctx context.Context, area Area, folder string, extension string, content io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !area.IsValid() || !folderPattern.MatchString(folder) || !extensionPattern.MatchString(extension) {
		return "", ErrInvalidKey
	}

	dir := filepath.Join(s.root, string(area), folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name, err := randomName()
	if err != nil {
		return "", err
	}

	temp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())

	if _, err := io.Copy(temp, content); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temp.Name(), filepath.Join(dir, name+extension)); err != nil {
		return "", err
	}

	return folder + "/" + name + extension, nil
}

func (s *LocalStorage) Open(ctx context.Context, area Area, key string) (*File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filePath, err := s.path(area, key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, ErrNotFound
	}

	return &File{ReadCloser: file, Size: info.Size()}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, area Area, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filePath, err := s.path(area, key)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStorage) path(area Area, key string) (string, error) {
	if !area.IsValid() || !keyPattern.MatchString(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(s.root, string(area), filepath.FromSlash(key)), nil
}

func randomName() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
