package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// fakeStore is an in-memory storage.Store keyed by the signed URL path.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

const fakeStorageURL = "https://storage.test/"

func (f *fakeStore) PresignUpload(_ context.Context, key, contentType, filename string, _ int64, ttl time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: fakeStorageURL + key, Method: http.MethodPut,
		Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: time.Now().Add(ttl)}, nil
}
func (f *fakeStore) PresignDownload(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return fakeStorageURL + key + "?sig=x", time.Now().Add(ttl), nil
}
func (f *fakeStore) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return storage.ObjectInfo{}, storage.ErrNotFound
	}
	return storage.ObjectInfo{Size: int64(len(b))}, nil
}
func (f *fakeStore) ReadHead(_ context.Context, key string, n int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return b[:min(int64(len(b)), n)], nil
}
func (f *fakeStore) Put(_ context.Context, key, _ string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = body
	return nil
}
func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

// uploadTo plays the browser's part: store body at the signed URL.
func (f *fakeStore) uploadTo(url string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[strings.TrimPrefix(url, fakeStorageURL)] = body
}

var testMP4 = append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 40)...)

func TestMediaOverHTTP(t *testing.T) {
	w := newOrgWorld(t)
	ana, eve := w.person("ana"), w.person("eve")
	org := decode[organisationResponse](t, w.call(ana, "POST", "/api/v1/organisations", `{"name":"httptest-media"}`))
	proj := decode[projectResponse](t, w.call(ana, "POST", "/api/v1/organisations/"+org.ID+"/projects", `{"name":"Shoot"}`))
	base := "/api/v1/organisations/" + org.ID + "/projects/" + proj.ID + "/media"
	size := strconv.Itoa(len(testMP4))

	expect(t, w.call(ana, "POST", base+"/uploads", `{"filename":"a.mp4","size_bytes":10,"extra":1}`), http.StatusBadRequest, CodeInvalidRequest)
	expect(t, w.call(ana, "POST", base+"/uploads", `{"filename":"a.exe","content_type":"application/x-msdownload","size_bytes":10}`), http.StatusUnprocessableEntity, CodeValidation)
	expect(t, w.call(eve, "POST", base+"/uploads", `{"filename":"a.mp4","size_bytes":10}`), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "POST", base+"/uploads", `{"filename":"a.mp4","size_bytes":10}`, "https://evil.example"), http.StatusForbidden, CodeForbidden)

	// Start, try to finish too early, upload, finish.
	rec := w.call(ana, "POST", base+"/uploads", `{"filename":"Take 1.mp4","content_type":"video/mp4","size_bytes":`+size+`}`)
	expect(t, rec, http.StatusCreated, "")
	up := decode[uploadResponse](t, rec)
	if up.Asset.Status != "uploading" || up.Asset.URL != nil || up.Asset.Kind != nil || up.Upload.Method != "PUT" ||
		up.Upload.Headers["Content-Type"] != "video/mp4" || !strings.Contains(up.Upload.URL, "/projects/"+proj.ID+"/assets/"+up.Asset.ID) {
		t.Fatalf("upload = %+v", up)
	}
	one := base + "/" + up.Asset.ID
	expect(t, w.call(ana, "POST", one+"/complete", ""), http.StatusConflict, CodeUploadIncomplete)
	w.store.uploadTo(up.Upload.URL, testMP4)
	rec = w.call(ana, "POST", one+"/complete", "")
	expect(t, rec, http.StatusOK, "")
	done := decode[assetResponse](t, rec)
	if done.Status != "uploaded" || done.Kind == nil || *done.Kind != "video" || done.URL == nil || done.UploadedAt == nil || !done.CanDelete {
		t.Fatalf("completed = %+v", done)
	}

	page := decode[assetPage](t, w.call(ana, "GET", base, ""))
	if len(page.Data) != 1 || page.NextCursor != nil || page.Data[0].Filename != "Take 1.mp4" {
		t.Fatalf("list = %+v", page)
	}
	expect(t, w.call(eve, "GET", base, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "GET", one, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "GET", base+"?limit=0", ""), http.StatusUnprocessableEntity, CodeValidation)

	// A disguised file is rejected and discarded.
	rec = w.call(ana, "POST", base+"/uploads", `{"filename":"x.mp4","content_type":"video/mp4","size_bytes":12}`)
	bad := decode[uploadResponse](t, rec)
	w.store.uploadTo(bad.Upload.URL, []byte("<html></html"))
	expect(t, w.call(ana, "POST", base+"/"+bad.Asset.ID+"/complete", ""), http.StatusUnprocessableEntity, CodeUploadRejected)
	expect(t, w.call(ana, "GET", base+"/"+bad.Asset.ID, ""), http.StatusNotFound, CodeNotFound)

	expect(t, w.call(ana, "DELETE", one, ""), http.StatusNoContent, "")
	expect(t, w.call(ana, "GET", one, ""), http.StatusNotFound, CodeNotFound)
}
