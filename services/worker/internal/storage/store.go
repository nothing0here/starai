package storage

import (
	"context"
	"io"
)

type Store interface {
	Upload(ctx context.Context, objectName, contentType string, r io.Reader, size int64) (string, error)
	ReadAll(ctx context.Context, objectName string, maxBytes int64) ([]byte, error)
	ObjectKeyFromURL(rawURL string) string
}
