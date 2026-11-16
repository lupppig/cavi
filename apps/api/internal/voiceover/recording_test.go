package voiceover

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/storage"
)

// recStore adds what recordings need to memStore: signed uploads and Stat.
type recStore struct{ *memStore }

func (r recStore) PresignUpload(_ context.Context, key, contentType, _ string, size int64, ttl time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: "https://storage.test/" + key + "?put", Method: "PUT", Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: time.Now().Add(ttl)}, nil
}

func (r recStore) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.objects[key]
	if !ok {
		return storage.ObjectInfo{}, storage.ErrNotFound
	}
	return storage.ObjectInfo{Size: int64(len(b))}, nil
}

// fakeDecoder returns PCM: ms of a tone (or silence), or an error.
type fakeDecoder struct {
	ms     int
	silent bool
	err    error
}

func (f fakeDecoder) Voice(context.Context, string, int, time.Duration) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	n := recordingRate * f.ms / 1000
	pcm := make([]byte, 2*n)
	if !f.silent {
		for i := range n {
			binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(9000*math.Sin(float64(i)/9))))
		}
	}
	return pcm, nil
}

func TestRecordings(t *testing.T) {
	w := newWorld(t)
	store := recStore{w.store}
	w.svc = NewService(w.db, store)
	ana, vic := w.user("Ana"), w.user("Vic")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)
	line := "Take a tour inside the largest university campus in Botswana."

	var verr *account.ValidationError
	if _, _, err := w.svc.CreateRecording(w.ctx, ana.id, org, proj, line, "application/x-msdownload", 1000); !errors.As(err, &verr) {
		t.Fatalf("bad type: %v", err)
	}
	if _, _, err := w.svc.CreateRecording(w.ctx, ana.id, org, proj, line, "audio/webm", MaxRecordingBytes+1); !errors.As(err, &verr) {
		t.Fatalf("too big: %v", err)
	}
	if _, _, err := w.svc.CreateRecording(w.ctx, vic.id, org, proj, line, "audio/webm", 1000); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	v, up, err := w.svc.CreateRecording(w.ctx, ana.id, org, proj, line, "audio/webm;codecs=opus", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusUploading || v.ModelLabel != "Your voice" || up.Method != "PUT" || up.Headers["Content-Type"] != "audio/webm" {
		t.Fatalf("recording = %+v, upload %+v", v, up)
	}
	// Not listed while uploading; completing needs the upload.
	if l, _ := w.svc.List(w.ctx, ana.id, org, proj); len(l) != 0 {
		t.Fatalf("uploading recording listed: %+v", l)
	}
	if _, err := w.svc.CompleteRecording(w.ctx, ana.id, org, proj, v.ID); !errors.Is(err, ErrNotReceived) {
		t.Fatalf("complete before upload: %v", err)
	}
	_ = store.Put(w.ctx, recordingKey(org, proj, v.ID), "audio/webm", []byte("webm bytes"))
	v, err = w.svc.CompleteRecording(w.ctx, ana.id, org, proj, v.ID)
	if err != nil || v.Status != StatusQueued {
		t.Fatalf("complete = %+v, %v", v, err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	j := w.job(v.ID)
	if err := NewRecorder(w.db, store, fakeDecoder{ms: 3200}, nil, log).Process(w.ctx, j); err != nil {
		t.Fatal(err)
	}
	ready, err := Load(w.ctx, w.db, org, proj, v.ID)
	if err != nil || ready.DurationMS != 3200 || len(ready.Words) != len(strings.Fields(line)) || !strings.HasSuffix(ready.StorageKey, ".wav") {
		t.Fatalf("ready = %+v, %v", ready, err)
	}
	if got, _ := w.svc.Get(w.ctx, vic.id, org, proj, v.ID); got.Status != StatusReady || got.URL == "" {
		t.Fatalf("after processing = %+v", got)
	}

	// Silence, or a recording that won't decode, fails with a reason.
	for _, d := range []fakeDecoder{{ms: 3000, silent: true}, {err: errors.New("ffmpeg: invalid data")}} {
		r, _, err := w.svc.CreateRecording(w.ctx, ana.id, org, proj, line, "audio/ogg", 500)
		if err != nil {
			t.Fatal(err)
		}
		_ = store.Put(w.ctx, recordingKey(org, proj, r.ID), "audio/ogg", []byte("ogg"))
		if _, err := w.svc.CompleteRecording(w.ctx, ana.id, org, proj, r.ID); err != nil {
			t.Fatal(err)
		}
		rj := w.job(r.ID)
		rec := NewRecorder(w.db, store, d, nil, log)
		err = rec.Process(w.ctx, rj)
		if !jobs.IsPermanent(err) {
			t.Fatalf("decoder %+v: %v", d, err)
		}
		_ = rec.Failed(w.ctx, rj, err)
		if got, _ := w.svc.Get(w.ctx, ana.id, org, proj, r.ID); got.Status != StatusFailed || got.Error == "" {
			t.Fatalf("failed recording = %+v", got)
		}
	}

	// Abandoned uploads are swept after a day.
	stale, _, _ := w.svc.CreateRecording(w.ctx, ana.id, org, proj, line, "audio/webm", 100)
	if _, err := w.db.ExecContext(w.ctx, `UPDATE voiceovers SET created_at = now() - interval '25 hours' WHERE id = ?`, stale.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := w.svc.Sweep(w.ctx); err != nil || n < 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	var queued int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM storage_deletions WHERE storage_key = ?`, recordingKey(org, proj, stale.ID)).Scan(&queued)
	if queued != 1 {
		t.Fatalf("abandoned recording's object queued %d times", queued)
	}
}
