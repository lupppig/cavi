package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// openTestStore connects to the local S3-compatible store, or skips.
// Set CAVI_TEST_STORAGE_ENDPOINT (make api-test-integration does). It uses
// its own bucket so the test's CORS rules don't replace the dev bucket's.
func openTestStore(t *testing.T) *S3 {
	t.Helper()
	endpoint := os.Getenv("CAVI_TEST_STORAGE_ENDPOINT")
	if endpoint == "" {
		t.Skip("CAVI_TEST_STORAGE_ENDPOINT not set")
	}
	s := NewS3(S3Config{
		Endpoint:        endpoint,
		Region:          "us-east-1",
		Bucket:          "cavi-integration-test",
		AccessKeyID:     os.Getenv("CAVI_STORAGE_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("CAVI_STORAGE_SECRET_ACCESS_KEY"),
		PathStyle:       true,
	})
	if err := s.Setup(context.Background(), []string{"http://localhost:3001"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func put(t *testing.T, u Upload, body []byte, override map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(u.Method, u.URL, bytes.NewReader(body))
	for k, v := range u.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range override {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

func TestS3RoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	key := "test/" + hex.EncodeToString(b) + "/original"
	t.Cleanup(func() { _ = s.Delete(context.Background(), key) })
	body := []byte("\x00\x00\x00\x18ftypisom-hello-world-payload")

	up, err := s.PresignUpload(ctx, key, "video/mp4", "clip one.mp4", int64(len(body)), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if up.Method != http.MethodPut || up.Headers["Content-Type"] != "video/mp4" ||
		up.Headers["Content-Disposition"] != `attachment; filename="clip one.mp4"` {
		t.Fatalf("upload = %+v", up)
	}

	// The signature covers the content type.
	if code := put(t, up, body, map[string]string{"Content-Type": "text/html"}); code < 400 {
		t.Fatalf("upload with a different content type was accepted (%d)", code)
	}
	if code := put(t, up, body, map[string]string{"Content-Disposition": "inline"}); code < 400 {
		t.Fatalf("upload with a different disposition was accepted (%d)", code)
	}
	if code := put(t, up, body, nil); code != http.StatusOK {
		t.Fatalf("upload status = %d", code)
	}

	info, err := s.Stat(ctx, key)
	if err != nil || info.Size != int64(len(body)) {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	head, err := s.ReadHead(ctx, key, 12)
	if err != nil || string(head) != string(body[:12]) {
		t.Fatalf("head = %q, %v", head, err)
	}

	url, _, err := s.PresignDownload(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("download = %d %q", res.StatusCode, got)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="clip one.mp4"` {
		t.Errorf("content-disposition = %q", cd)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("content-type = %q", ct)
	}

	// Browsers can preflight uploads from allowed origins.
	pre, _ := http.NewRequest(http.MethodOptions, up.URL, nil)
	pre.Header.Set("Origin", "http://localhost:3001")
	pre.Header.Set("Access-Control-Request-Method", "PUT")
	pre.Header.Set("Access-Control-Request-Headers", "content-type,content-disposition")
	res, err = http.DefaultClient.Do(pre)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3001" {
		t.Errorf("preflight: %d, allow-origin %q", res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
	}

	thumb := key + "-thumb"
	if err := s.Put(ctx, thumb, "image/jpeg", []byte("\xff\xd8\xffthumb")); err != nil {
		t.Fatal(err)
	}
	if info, err := s.Stat(ctx, thumb); err != nil || info.Size != 8 {
		t.Fatalf("put then stat = %+v, %v", info, err)
	}
	_ = s.Delete(ctx, thumb)

	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stat after delete: %v", err)
	}
	if _, err := s.ReadHead(ctx, key, 4); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read after delete: %v", err)
	}
}
