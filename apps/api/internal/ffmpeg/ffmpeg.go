// Package ffmpeg runs ffprobe and ffmpeg for media processing. It is the
// only place that builds FFmpeg command lines; callers pass structured
// requests, never raw arguments.
//
// Inputs are read over HTTP(S) from short-lived signed URLs, so workers
// don't download whole files to inspect them. Only HTTP(S) protocols are
// allowed, so a crafted file can't make FFmpeg read local files or other
// protocols. Error messages never contain the input URL (it is signed).
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrUnreadable means the input is not a media file FFmpeg can decode.
// Retrying won't help.
var ErrUnreadable = errors.New("ffmpeg: file could not be read as media")

const protocols = "http,https,tcp,tls"

// Runner runs the FFmpeg binaries.
type Runner struct {
	FFprobe string
	FFmpeg  string
	// Timeout bounds each command (default 2 minutes).
	Timeout time.Duration
}

// New returns a Runner using ffprobe and ffmpeg from PATH.
func New() *Runner { return &Runner{FFprobe: "ffprobe", FFmpeg: "ffmpeg", Timeout: 2 * time.Minute} }

// Available reports whether both binaries can be found.
func (r *Runner) Available() error {
	for _, bin := range []string{r.FFprobe, r.FFmpeg} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found: %w", bin, err)
		}
	}
	return nil
}

// Probe describes a media file.
type Probe struct {
	Container  string
	Duration   time.Duration
	BitRate    int64
	HasVideo   bool
	VideoCodec string
	// Width and Height are display dimensions, after rotation metadata.
	Width, Height int
	FrameRate     float64
	// VideoDuration is the video stream's own length, when the file
	// records it: a video cut short shows here while Duration, the longest
	// stream, doesn't.
	VideoDuration time.Duration
	HasAudio      bool
	AudioCodec    string
	AudioChannels int
	SampleRate    int
}

// Probe inspects input with ffprobe.
func (r *Runner) Probe(ctx context.Context, input string) (Probe, error) {
	out, err := r.run(ctx, input, r.FFprobe,
		"-v", "error", "-protocol_whitelist", protocols,
		"-print_format", "json", "-show_format", "-show_streams",
		"-i", input)
	if err != nil {
		return Probe{}, err
	}
	return parseProbe(out)
}

// Frame grabs one frame at offset as a JPEG no wider than maxWidth.
func (r *Runner) Frame(ctx context.Context, input string, offset time.Duration, maxWidth int) ([]byte, error) {
	out, err := r.run(ctx, input, r.FFmpeg,
		"-v", "error", "-nostdin", "-protocol_whitelist", protocols,
		"-ss", strconv.FormatFloat(offset.Seconds(), 'f', 3, 64),
		"-i", input,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", maxWidth),
		"-f", "image2", "-c:v", "mjpeg", "-q:v", "4",
		"pipe:1")
	if err != nil {
		return nil, err
	}
	// A zero exit doesn't guarantee an image: check what came out.
	if len(out) < 100 || !bytes.HasPrefix(out, []byte{0xFF, 0xD8, 0xFF}) {
		return nil, fmt.Errorf("%w: no frame could be extracted", ErrUnreadable)
	}
	if len(out) > 5<<20 {
		return nil, errors.New("ffmpeg: thumbnail unexpectedly large")
	}
	return out, nil
}

// Audio extracts length of audio starting at offset as mono 16 kHz MP3
// (32 kbps), the compact form sent for transcription.
func (r *Runner) Audio(ctx context.Context, input string, offset, length time.Duration) ([]byte, error) {
	out, err := r.run(ctx, input, r.FFmpeg,
		"-v", "error", "-nostdin", "-protocol_whitelist", protocols,
		"-ss", strconv.FormatFloat(offset.Seconds(), 'f', 3, 64),
		"-t", strconv.FormatFloat(length.Seconds(), 'f', 3, 64),
		"-i", input,
		"-vn", "-ac", "1", "-ar", "16000", "-c:a", "libmp3lame", "-b:a", "32k",
		"-f", "mp3", "pipe:1")
	if err != nil {
		return nil, err
	}
	if len(out) < 100 {
		return nil, fmt.Errorf("%w: no audio could be extracted", ErrUnreadable)
	}
	return out, nil
}

// Clip describes one rendered clip: a stretch of the input, cropped and
// scaled to an exact output size, encoded as H.264/AAC MP4. It carries no
// product meaning; callers decide the numbers.
type Clip struct {
	Start, Length time.Duration
	// Crop is the source region, in display pixels (after rotation).
	CropX, CropY, CropW, CropH int
	// Width and Height are the output size; both even.
	Width, Height int
	// Audio keeps the first audio stream; otherwise the clip is silent.
	Audio bool
	// Pan, if set, moves the crop over time (CropX, or CropY when
	// PanVertical), overriding it.
	Pan         []PanKey
	PanVertical bool
	// Subtitles, if set, is a local ASS file burned into the output after
	// scaling (so its coordinates are output pixels), with fonts from
	// FontsDir. Both are paths CAVI made: plain characters only.
	Subtitles, FontsDir string
}

// safePath matches the paths allowed inside a filter graph, where ':' ','
// '[' and quotes have meaning.
var safePath = regexp.MustCompile(`^[A-Za-z0-9/_.-]+$`)

// Clip renders c from input to the local file output, replacing it.
// Seeking before the input and re-encoding makes the cut frame-accurate.
func (r *Runner) Clip(ctx context.Context, input, output string, c Clip, timeout time.Duration) error {
	if c.Length <= 0 || c.CropW <= 0 || c.CropH <= 0 || c.Width <= 0 || c.Height <= 0 || c.Width%2 != 0 || c.Height%2 != 0 {
		return fmt.Errorf("ffmpeg: invalid clip %+v", c)
	}
	filter := cropFilter(c.CropW, c.CropH, c.CropX, c.CropY, c.Pan, c.PanVertical) + fmt.Sprintf(",scale=%d:%d:flags=lanczos,setsar=1", c.Width, c.Height)
	if c.Subtitles != "" {
		if !safePath.MatchString(c.Subtitles) || (c.FontsDir != "" && !safePath.MatchString(c.FontsDir)) {
			return fmt.Errorf("ffmpeg: subtitle or font path has characters not allowed in a filter")
		}
		filter += ",subtitles=filename=" + c.Subtitles
		if c.FontsDir != "" {
			filter += ":fontsdir=" + c.FontsDir
		}
	}
	args := []string{
		"-v", "error", "-nostdin", "-y", "-protocol_whitelist", protocols,
		"-ss", strconv.FormatFloat(c.Start.Seconds(), 'f', 3, 64),
		"-i", input,
		"-t", strconv.FormatFloat(c.Length.Seconds(), 'f', 3, 64),
		"-map", "0:v:0",
	}
	if c.Audio {
		args = append(args, "-map", "0:a:0", "-c:a", "aac", "-b:a", "160k", "-ar", "48000")
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-vf", filter,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-profile:v", "high",
		"-movflags", "+faststart", "-f", "mp4", output)
	_, err := r.runFor(ctx, timeout, input, r.FFmpeg, args...)
	return err
}

// PanKey is a point on a moving crop: At after the clip (or montage part)
// starts, the crop's left (or top) edge is at Pos pixels. The crop moves
// in a straight line between keys and holds still before the first and
// after the last.
type PanKey struct {
	At  time.Duration
	Pos int
}

// cropFilter is a crop of w×h at x, y, or, with pan, moving along x (or y
// when vertical).
func cropFilter(w, h, x, y int, pan []PanKey, vertical bool) string {
	if len(pan) == 0 {
		return fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y)
	}
	if vertical {
		return fmt.Sprintf("crop=w=%d:h=%d:x=%d:y='%s'", w, h, x, panExpr(pan))
	}
	return fmt.Sprintf("crop=w=%d:h=%d:x='%s':y=%d", w, h, panExpr(pan), y)
}

// panExpr is an FFmpeg expression of t (seconds) for the position along
// keys: a balanced tree of comparisons, so its depth grows with the log
// of the number of keys.
func panExpr(keys []PanKey) string {
	first, last := keys[0], keys[len(keys)-1]
	if len(keys) == 1 {
		return strconv.Itoa(first.Pos)
	}
	var build func(lo, hi int) string
	build = func(lo, hi int) string {
		if hi-lo == 1 {
			a, b := keys[lo], keys[hi]
			if b.At <= a.At || a.Pos == b.Pos {
				return strconv.Itoa(b.Pos)
			}
			return fmt.Sprintf("%d+%d*(t-%s)/%s", a.Pos, b.Pos-a.Pos, secs(a.At), secs(b.At-a.At))
		}
		mid := (lo + hi) / 2
		return fmt.Sprintf("if(lt(t,%s),%s,%s)", secs(keys[mid].At), build(lo, mid), build(mid, hi))
	}
	// t is unknown (NaN) while the filter is set up.
	return fmt.Sprintf("if(isnan(t)+lt(t,%s),%d,if(gte(t,%s),%d,%s))", secs(first.At), first.Pos, secs(last.At), last.Pos, build(0, len(keys)-1))
}

// ProbeFile inspects a local file CAVI produced (never user input).
func (r *Runner) ProbeFile(ctx context.Context, path string) (Probe, error) {
	out, err := r.run(ctx, path, r.FFprobe,
		"-v", "error", "-protocol_whitelist", "file",
		"-print_format", "json", "-show_format", "-show_streams",
		"-i", "file:"+path)
	if err != nil {
		return Probe{}, err
	}
	return parseProbe(out)
}

var urlPattern = regexp.MustCompile(`https?://\S+`)

// scrub removes URLs (which carry signatures) from FFmpeg output.
func scrub(s, input string) string {
	s = strings.ReplaceAll(s, input, "<input>")
	s = urlPattern.ReplaceAllString(s, "<url>")
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

// unreadableSignals are FFmpeg messages that mean the data itself is bad.
var unreadableSignals = []string{
	"Invalid data found", "moov atom not found", "could not find codec parameters",
	"Invalid argument", "no streams", "does not contain any stream", "EBML header parsing failed",
	"Output file is empty", "Error opening input file",
}

func (r *Runner) run(ctx context.Context, input, bin string, args ...string) ([]byte, error) {
	return r.runFor(ctx, r.Timeout, input, bin, args...)
}

// runFor runs bin with its own timeout (default 2 minutes).
func (r *Runner) runFor(ctx context.Context, timeout time.Duration, input, bin string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("ffmpeg: %s timed out or was cancelled", bin)
	}
	if err != nil {
		msg := scrub(stderr.String(), input)
		// HTTP errors from storage are transient (or a signing problem);
		// decoding errors are permanent.
		if strings.Contains(msg, "Server returned") || strings.Contains(msg, "Connection refused") ||
			strings.Contains(msg, "timed out") {
			return nil, fmt.Errorf("ffmpeg: %s could not fetch the input: %s", bin, msg)
		}
		for _, sig := range unreadableSignals {
			if strings.Contains(msg, sig) {
				return nil, fmt.Errorf("%w: %s", ErrUnreadable, msg)
			}
		}
		return nil, fmt.Errorf("ffmpeg: %s failed: %s", bin, msg)
	}
	return stdout.Bytes(), nil
}

type probeJSON struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType    string            `json:"codec_type"`
		CodecName    string            `json:"codec_name"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		Channels     int               `json:"channels"`
		SampleRate   string            `json:"sample_rate"`
		Duration     string            `json:"duration"`
		Tags         map[string]string `json:"tags"`
		Disposition  map[string]int    `json:"disposition"`
		SideData     []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

func parseProbe(out []byte) (Probe, error) {
	var j probeJSON
	if err := json.Unmarshal(out, &j); err != nil {
		return Probe{}, fmt.Errorf("%w: unreadable probe output", ErrUnreadable)
	}
	p := Probe{Container: j.Format.FormatName}
	p.BitRate, _ = strconv.ParseInt(j.Format.BitRate, 10, 64)
	if d, err := strconv.ParseFloat(j.Format.Duration, 64); err == nil && d > 0 && !math.IsInf(d, 0) {
		p.Duration = time.Duration(d * float64(time.Second))
	}
	for _, s := range j.Streams {
		switch s.CodecType {
		case "video":
			// Cover art in audio files is a "video" stream; skip it.
			if p.HasVideo || s.Disposition["attached_pic"] == 1 || s.Width <= 0 || s.Height <= 0 {
				continue
			}
			p.HasVideo, p.VideoCodec = true, s.CodecName
			p.Width, p.Height = s.Width, s.Height
			if rotated(s.Tags["rotate"], s.SideData) {
				p.Width, p.Height = p.Height, p.Width
			}
			p.FrameRate = rate(s.AvgFrameRate)
			if p.FrameRate == 0 {
				p.FrameRate = rate(s.RFrameRate)
			}
			if d, err := strconv.ParseFloat(s.Duration, 64); err == nil && d > 0 && !math.IsInf(d, 0) {
				p.VideoDuration = time.Duration(d * float64(time.Second))
				if p.Duration == 0 {
					p.Duration = p.VideoDuration
				}
			}
		case "audio":
			if p.HasAudio {
				continue
			}
			p.HasAudio, p.AudioCodec, p.AudioChannels = true, s.CodecName, s.Channels
			p.SampleRate, _ = strconv.Atoi(s.SampleRate)
		}
	}
	if !p.HasVideo && !p.HasAudio {
		return Probe{}, fmt.Errorf("%w: no audio or video streams", ErrUnreadable)
	}
	return p, nil
}

func rotated(tag string, side []struct {
	Rotation float64 `json:"rotation"`
}) bool {
	deg := 0.0
	if tag != "" {
		deg, _ = strconv.ParseFloat(tag, 64)
	}
	for _, sd := range side {
		if sd.Rotation != 0 {
			deg = sd.Rotation
		}
	}
	return int(math.Abs(deg))%180 == 90
}

// rate parses "30000/1001" style frame rates.
func rate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return math.Round(n/d*1000) / 1000
}

// Sampling is how Frames reads a video for analysis: FPS frames a second,
// scaled to exactly Width×Height (display orientation), as packed 8-bit
// BGR (3 bytes a pixel).
type Sampling struct {
	FPS           int
	Width, Height int
}

// Frames decodes input (a signed URL or a local file CAVI made) and calls
// fn with each sampled frame in order: i counts from 0, so the frame is at
// i/FPS seconds. bgr is Width×Height×3 bytes, reused between calls. fn
// returning an error stops decoding.
func (r *Runner) Frames(ctx context.Context, in string, g Sampling, timeout time.Duration, fn func(i int, bgr []byte) error) error {
	if g.FPS <= 0 || g.Width < 2 || g.Height < 2 || g.Width*g.Height > 4<<20 {
		return fmt.Errorf("ffmpeg: invalid frame sampling %+v", g)
	}
	if timeout <= 0 {
		timeout = r.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append([]string{"-v", "error", "-nostdin"}, input(in)...)
	args = append(args, "-map", "0:v:0", "-an", "-sn",
		"-vf", fmt.Sprintf("fps=%d,scale=%d:%d:flags=area,format=bgr24", g.FPS, g.Width, g.Height),
		"-f", "rawvideo", "pipe:1")
	cmd := exec.CommandContext(ctx, r.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	pix := make([]byte, g.Width*g.Height*3)
	var fnErr error
	for i := 0; ; i++ {
		if _, err := io.ReadFull(out, pix); err != nil {
			break // end of stream (or a decode failure, reported by Wait)
		}
		if fnErr = fn(i, pix); fnErr != nil {
			cancel()
			break
		}
	}
	_, _ = io.Copy(io.Discard, out)
	waitErr := cmd.Wait()
	switch {
	case fnErr != nil:
		return fnErr
	case ctx.Err() != nil:
		return fmt.Errorf("ffmpeg: %s timed out or was cancelled", r.FFmpeg)
	case waitErr != nil:
		msg := scrub(stderr.String(), in)
		for _, sig := range unreadableSignals {
			if strings.Contains(msg, sig) {
				return fmt.Errorf("%w: %s", ErrUnreadable, msg)
			}
		}
		return fmt.Errorf("ffmpeg: frames failed: %s", msg)
	}
	return nil
}
