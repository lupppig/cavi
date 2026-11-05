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
	timeout := r.Timeout
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
			if p.Duration == 0 {
				if d, err := strconv.ParseFloat(s.Duration, 64); err == nil && d > 0 {
					p.Duration = time.Duration(d * float64(time.Second))
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
