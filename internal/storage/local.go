package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-service/internal/security"
)

type Local struct {
	root string
}

func NewLocal(root string) (*Local, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}

func (l *Local) SaveUploadedFile(file multipart.File, originalName string) (relativePath, mimeType, shaValue string, size int64, err error) {
	defer file.Close()

	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if readErr != nil && readErr != io.EOF {
		return "", "", "", 0, readErr
	}

	now := time.Now()
	subdir := filepath.Join("uploads", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(filepath.Join(l.root, subdir), 0o755); err != nil {
		return "", "", "", 0, err
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	filename := security.RandomString(16) + ext
	relativePath = filepath.ToSlash(filepath.Join(subdir, filename))
	fullPath := filepath.Join(l.root, filepath.FromSlash(relativePath))

	dst, err := os.Create(fullPath)
	if err != nil {
		return "", "", "", 0, err
	}
	defer dst.Close()

	hash := sha256.New()
	reader := io.MultiReader(bytes.NewReader(head[:n]), file)
	written, err := io.Copy(io.MultiWriter(dst, hash), reader)
	if err != nil {
		return "", "", "", 0, err
	}

	mimeType = http.DetectContentType(head[:n])
	return relativePath, mimeType, hex.EncodeToString(hash.Sum(nil)), written, nil
}

func (l *Local) Open(relativePath string) (*os.File, error) {
	fullPath, err := l.resolvePath(relativePath)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
}

func (l *Local) Remove(relativePath string) error {
	fullPath, err := l.resolvePath(relativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = l.cleanupEmptyDirs(filepath.Dir(fullPath))
	return nil
}

func (l *Local) ReadText(relativePath string, limit int64) (string, bool, error) {
	file, err := l.Open(relativePath)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", false, err
	}

	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return string(data), truncated, nil
}

func (l *Local) resolvePath(relativePath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	fullPath := filepath.Join(l.root, clean)
	rootClean := filepath.Clean(l.root)

	rel, err := filepath.Rel(rootClean, fullPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file path")
	}
	return fullPath, nil
}

func (l *Local) cleanupEmptyDirs(dir string) error {
	rootClean := filepath.Clean(l.root)
	current := filepath.Clean(dir)

	for current != rootClean {
		entries, err := os.ReadDir(current)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(current); err != nil {
			return err
		}
		current = filepath.Dir(current)
	}
	return nil
}
