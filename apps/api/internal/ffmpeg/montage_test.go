package ffmpeg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMontageFilterGraph(t *testing.T) {
	m := Montage{Width: 1080, Height: 1920, FPS: 30, Parts: []MontagePart{
		{Input: "a", Length: time.Second, CropW: 100, CropH: 100, Audio: true,
			Blur: []Box{{X: 10, Y: 20, W: 30, H: 40, Span: Span{From: 0, To: time.Second}}},
			Mute: []Span{{From: 200 * time.Millisecond, To: 400 * time.Millisecond}}},
		{Input: "b", Length: 2 * time.Second, CropW: 100, CropH: 100},
	}}
	g, err := m.filterGraph()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"crop=30:40:10:20,gblur", "overlay=10:20:enable='between(t,0.000,1.000)'",
		"volume=0:enable='between(t,0.200,0.400)'", "anullsrc", "concat=n=2:v=1:a=1", "[ac]anull[aout]"} {
		if !strings.Contains(g, want) {
			t.Errorf("graph lacks %q:\n%s", want, g)
		}
	}
	m.Subtitles = "/tmp/x y.ass"
	if _, err := m.filterGraph(); err == nil {
		t.Error("unsafe subtitle path accepted")
	}
	m.Subtitles = ""
	m.Parts[0].Blur[0].W = 0
	if _, err := m.filterGraph(); err == nil {
		t.Error("empty blur box accepted")
	}
}

var meanVolume = regexp.MustCompile(`mean_volume: (-?[0-9.]+) dB`)

// loudness is the mean volume of path between from and from+length.
func loudness(t *testing.T, path string, from, length float64) float64 {
	t.Helper()
	out, _ := exec.Command("ffmpeg", "-hide_banner", "-ss", strconv.FormatFloat(from, 'f', 2, 64), "-t", strconv.FormatFloat(length, 'f', 2, 64),
		"-i", path, "-af", "volumedetect", "-vn", "-f", "null", "-").CombinedOutput()
	m := meanVolume.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no volume at %.1fs: %s", from, out)
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return v
}

// detail is the PNG size of a region of path's frame at time at: blurred
// regions compress far smaller.
func detail(t *testing.T, path string, at float64, crop string) int {
	t.Helper()
	png := filepath.Join(t.TempDir(), "f.png")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-y", "-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", path,
		"-frames:v", "1", "-vf", "crop="+crop, png).CombinedOutput(); err != nil {
		t.Fatalf("frame: %v %s", err, out)
	}
	fi, err := os.Stat(png)
	if err != nil {
		t.Fatal(err)
	}
	return int(fi.Size())
}

func TestRealMontage(t *testing.T) {
	r := New()
	if err := r.Available(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	gen := func(name string, args ...string) {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, append(args, filepath.Join(dir, name))...)...).CombinedOutput(); err != nil {
			t.Fatalf("generate %s: %v %s", name, err, out)
		}
	}
	gen("talk.mp4", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest")
	gen("broll.mp4", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30", "-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	gen("vo.wav", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=24000", "-t", "1")
	ass := filepath.Join(dir, "subs.ass")
	if err := os.WriteFile(ass, []byte("[Script Info]\nScriptType: v4.00+\nPlayResX: 360\nPlayResY: 640\n\n[V4+ Styles]\n"+
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n"+
		"Style: Hook,Inter Black,40,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,3,8,0,8,20,20,60,1\n\n"+
		"[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
		"Dialogue: 0,0:00:00.00,0:00:01.50,Hook,,0,0,0,,Diamonds built this\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	out := filepath.Join(dir, "montage.mp4")
	m := Montage{Width: 360, Height: 640, FPS: 30, Subtitles: ass,
		Voiceover: srv.URL + "/vo.wav", VoiceoverAt: 2500 * time.Millisecond, VoiceoverLength: time.Second, Duck: 0.2,
		Parts: []MontagePart{
			// 2 s of the talk, 9:16 from the centre, its first second muted
			// and its top-left blurred.
			{Input: srv.URL + "/talk.mp4", Start: time.Second, Length: 2 * time.Second, CropX: 219, CropW: 202, CropH: 360, Audio: true,
				Mute: []Span{{From: 0, To: time.Second}},
				Blur: []Box{{X: 219, Y: 0, W: 202, H: 180, Span: Span{From: 0, To: time.Second}}}},
			// 1.5 s of silent b-roll.
			{Input: srv.URL + "/broll.mp4", Length: 1500 * time.Millisecond, CropX: 437, CropW: 406, CropH: 720},
		}}
	if err := r.Montage(context.Background(), out, m, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	p, err := r.ProbeFile(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 360 || p.Height != 640 || p.AudioCodec != "aac" || p.Duration < 3400*time.Millisecond || p.Duration > 3700*time.Millisecond {
		t.Fatalf("montage = %+v", p)
	}
	if v := loudness(t, out, 0.1, 0.8); v > -60 {
		t.Errorf("muted range is %.1f dB, want silence", v)
	}
	if v := loudness(t, out, 1.2, 0.6); v < -40 {
		t.Errorf("unmuted talk is %.1f dB, want sound", v)
	}
	if v := loudness(t, out, 2.05, 0.4); v > -60 {
		t.Errorf("silent b-roll before the voiceover is %.1f dB", v)
	}
	if v := loudness(t, out, 2.6, 0.7); v < -40 {
		t.Errorf("voiceover is %.1f dB, want sound", v)
	}
	// The blurred top half (first second) carries far less detail than
	// the same region once the blur ends.
	blurred, sharp := detail(t, out, 0.5, "360:160:0:140"), detail(t, out, 1.6, "360:160:0:140")
	if blurred*2 > sharp {
		t.Errorf("blurred region %d bytes vs sharp %d: blur not applied", blurred, sharp)
	}
}

var yavg = regexp.MustCompile(`YAVG=([0-9.]+)`)

// brightness is the mean luma (0-255) of a region of path's frame at at.
func brightness(t *testing.T, path string, at float64, crop string) float64 {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-v", "error", "-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", path, "-frames:v", "1",
		"-vf", "crop="+crop+",signalstats,metadata=mode=print:key=lavfi.signalstats.YAVG:file=-", "-f", "null", "-").CombinedOutput()
	m := yavg.FindSubmatch(out)
	if err != nil || m == nil {
		t.Fatalf("brightness: %v %s", err, out)
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return v
}

func TestRealSplitScreen(t *testing.T) {
	r := New()
	if err := r.Available(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"talk.mp4":  {"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"},
		"broll.mp4": {"-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30", "-t", "6", "-c:v", "libx264", "-pix_fmt", "yuv420p"},
	} {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, append(args, filepath.Join(dir, name))...)...).CombinedOutput(); err != nil {
			t.Fatalf("generate %s: %v %s", name, err, out)
		}
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	out := filepath.Join(dir, "split.mp4")
	ph := PanelHeight(640, 3) // 212: three 360x212 panels
	m := Montage{Width: 360, Height: 640, FPS: 30, Parts: []MontagePart{
		{Input: srv.URL + "/talk.mp4", Length: 2 * time.Second, CropX: 0, CropY: 77, CropW: 640, CropH: 206, Audio: true,
			Stack: []Tile{
				{Input: srv.URL + "/broll.mp4", Start: time.Second, CropX: 0, CropY: 222, CropW: 1280, CropH: 754 - 478},
				{Input: srv.URL + "/broll.mp4", Start: 3 * time.Second, CropX: 0, CropY: 222, CropW: 1280, CropH: 276},
			}},
		{Input: srv.URL + "/talk.mp4", Start: 2 * time.Second, Length: time.Second, CropX: 219, CropW: 202, CropH: 360, Audio: true},
	}}
	if err := r.Montage(context.Background(), out, m, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	p, err := r.ProbeFile(context.Background(), out)
	if err != nil || p.Width != 360 || p.Height != 640 || !p.HasAudio || p.Duration < 2900*time.Millisecond || p.Duration > 3200*time.Millisecond {
		t.Fatalf("split screen = %+v, %v", p, err)
	}
	top := (640 - 3*ph) / 2
	for k := 1; k < 3; k++ {
		y := top + k*ph - 2
		if b := brightness(t, out, 1, "360:4:0:"+strconv.Itoa(y)); b < 220 {
			t.Errorf("divider %d at y=%d has brightness %.0f, want white", k, y, b)
		}
	}
	if v := loudness(t, out, 0.2, 1.5); v < -40 {
		t.Errorf("split screen is %.1f dB: the main scene's sound should play", v)
	}
}
