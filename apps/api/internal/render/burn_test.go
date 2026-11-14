package render

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
)

// TestBurnCaptions renders real captions with FFmpeg and libass and checks
// they appear on the frame while their cue is showing, and not after. It
// runs where ffmpeg and the Inter font are installed (the media worker
// image); CAVI_TEST_FONTS_DIR points at the fonts.
func TestBurnCaptions(t *testing.T) {
	fonts := os.Getenv("CAVI_TEST_FONTS_DIR")
	r := ffmpeg.New()
	if fonts == "" || r.Available() != nil {
		t.Skip("needs ffmpeg and CAVI_TEST_FONTS_DIR")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=640x360:r=25:d=4",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono", "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", src).CombinedOutput(); err != nil {
		t.Fatalf("make source: %v %s", err, out)
	}
	c := Composition{Version: 2, SourceAssetID: "x", StartMS: 0, EndMS: 3000, Aspect: "9:16", Focus: 0.5,
		Captions: &Captions{Style: CaptionsBoxed, Cues: []Cue{{StartMS: 200, EndMS: 1500, Text: "Burned in\ncaptions"}}}}
	srcInfo := Source{Width: 640, Height: 360, DurationMS: 4000, HasAudio: true}
	if err := c.Validate(srcInfo); err != nil {
		t.Fatal(err)
	}
	plan := Plan(c, srcInfo)
	plan.Subtitles = filepath.Join(dir, "captions.ass")
	plan.FontsDir = fonts
	if err := os.WriteFile(plan.Subtitles, []byte(ASS(*c.Captions, c.Aspect)), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.mp4")
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	if err := r.Clip(context.Background(), srv.URL+"/src.mp4", out, plan, time.Minute); err != nil {
		t.Fatal(err)
	}
	// Bright pixels in the lower half, while the caption shows and after.
	bright := func(at string) int {
		raw, err := exec.Command("ffmpeg", "-v", "error", "-ss", at, "-i", out, "-frames:v", "1",
			"-vf", "crop=1080:960:0:960,scale=270:240", "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1").Output()
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Count(raw, []byte{255}) + bytes.Count(raw, []byte{254}) + bytes.Count(raw, []byte{253})
	}
	if dst := os.Getenv("CAVI_TEST_FRAME_OUT"); dst != "" {
		_ = exec.Command("ffmpeg", "-v", "error", "-y", "-ss", "0.8", "-i", out, "-frames:v", "1", dst).Run()
	}
	during, after := bright("0.8"), bright("2.5")
	t.Logf("bright pixels while captioned: %d, after: %d", during, after)
	if during < 500 || after > 20 {
		t.Fatalf("captions not burned in as expected (during %d, after %d)", during, after)
	}
}
