// Package storage is CAVI's boundary with object storage. Production uses
// Cloudflare R2; local development uses SeaweedFS. Both speak the S3 API,
// so one implementation serves both. Nothing outside this package should
// import the AWS SDK.
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means the object does not exist.
var ErrNotFound = errors.New("storage: object not found")

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size int64
}

// Upload is a signed instruction for the browser to upload one object.
// The browser must send exactly Headers with the PUT request.
type Upload struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// Store is the object storage the application uses.
type Store interface {
	// PresignUpload returns a signed PUT for key. The content type and the
	// download filename (stored as Content-Disposition: attachment) are part
	// of the signature, so the browser can't change them.
	PresignUpload(ctx context.Context, key, contentType, filename string, size int64, ttl time.Duration) (Upload, error)
	// PresignDownload returns a signed GET for key. Media elements (<video>,
	// <img>) play it directly; opening the URL downloads it under the
	// filename given at upload.
	PresignDownload(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
	// Stat returns the object's metadata, or ErrNotFound.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// ReadHead returns up to n bytes from the start of the object.
	ReadHead(ctx context.Context, key string, n int64) ([]byte, error)
	// Delete removes the object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}
