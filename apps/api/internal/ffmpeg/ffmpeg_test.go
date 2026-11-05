package ffmpeg

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseProbe(t *testing.T) {
	phone := `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"12.480000","bit_rate":"8123456"},
	"streams":[{"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,"avg_frame_rate":"30000/1001",
	"side_data_list":[{"rotation":-90}]},{"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000"}]}`
	p, err := parseProbe([]byte(phone))
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 1080 || p.Height != 1920 || p.FrameRate != 29.97 || p.Duration != 12480*time.Millisecond ||
		p.VideoCodec != "hevc" || p.AudioCodec != "aac" || p.AudioChannels != 2 || p.SampleRate != 48000 || p.BitRate != 8123456 {
		t.Fatalf("phone video = %+v", p)
	}

	mp3 := `{"format":{"format_name":"mp3","duration":"183.2"},"streams":[
	{"codec_type":"audio","codec_name":"mp3","channels":2,"sample_rate":"44100"},
	{"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}}]}`
	p, err = parseProbe([]byte(mp3))
	if err != nil || p.HasVideo || !p.HasAudio || p.Width != 0 {
		t.Fatalf("mp3 with cover art = %+v, %v", p, err)
	}

	png := `{"format":{"format_name":"png_pipe"},"streams":[{"codec_type":"video","codec_name":"png","width":800,"height":450,"avg_frame_rate":"0/0"}]}`
	p, err = parseProbe([]byte(png))
	if err != nil || p.Width != 800 || p.Duration != 0 || p.FrameRate != 0 {
		t.Fatalf("png = %+v, %v", p, err)
	}

	for _, bad := range []string{`{"format":{},"streams":[]}`, `not json`} {
		if _, err := parseProbe([]byte(bad)); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestScrubRemovesSignedURLs(t *testing.T) {
	in := "https://storage.example/o?X-Amz-Signature=abc"
	got := scrub(in+": Server returned 403 Forbidden (access denied) see http://other/x?sig=1", in)
	if strings.Contains(got, "Signature") || strings.Contains(got, "sig=") || !strings.Contains(got, "403") {
		t.Fatalf("scrubbed = %q", got)
	}
}

// TestRealFFmpeg runs the binaries against generated media served over
// HTTP. It runs where ffmpeg is installed (the media worker image).
func TestRealFFmpeg(t *testing.T) {
	r := New()
	if err := r.Available(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	gen := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", video)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "fake.mp4"), []byte("\x00\x00\x00\x18ftypisom not really a video at all, sorry"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	ctx := context.Background()

	p, err := r.Probe(ctx, srv.URL+"/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 640 || p.Height != 360 || p.FrameRate != 25 || p.VideoCodec != "h264" || p.AudioCodec != "aac" ||
		p.Duration < 1900*time.Millisecond || p.Duration > 2200*time.Millisecond {
		t.Fatalf("probe = %+v", p)
	}
	jpg, err := r.Frame(ctx, srv.URL+"/clip.mp4", time.Second, 320)
	if err != nil {
		t.Fatal(err)
	}
	if len(jpg) < 1000 {
		t.Fatalf("thumbnail is %d bytes", len(jpg))
	}

	if _, err := r.Probe(ctx, srv.URL+"/fake.mp4"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("fake: err = %v", err)
	}
	_, err = r.Probe(ctx, srv.URL+"/missing.mp4?X-Amz-Signature=secret")
	if err == nil || errors.Is(err, ErrUnreadable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("missing: err = %v", err)
	}
	// Local files are refused even if a URL is crafted to point at one.
	if _, err := r.Probe(ctx, "file:///etc/passwd"); err == nil {
		t.Fatal("file:// input was read")
	}
}
