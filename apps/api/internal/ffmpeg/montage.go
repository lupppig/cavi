package ffmpeg

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Span is a stretch of time within a montage part, from its start.
type Span struct{ From, To time.Duration }

// Box is a region of a part's source frame, in display pixels, blurred
// for a Span.
type Box struct {
	X, Y, W, H int
	Span
}

// MontagePart is one stretch of one source in a montage.
type MontagePart struct {
	// Input is the source (a signed URL or a file CAVI made).
	Input         string
	Start, Length time.Duration
	// Crop is the source region, in display pixels, scaled to the output.
	CropX, CropY, CropW, CropH int
	// Audio keeps the source's first audio stream; otherwise the part is
	// silent.
	Audio bool
	// Blur hides regions of the source (licence plates, faces) before it
	// is cropped. Mute silences the source's audio.
	Blur []Box
	Mute []Span
}

// Montage joins parts, one after another, into one clip.
type Montage struct {
	Parts []MontagePart
	// Width and Height are the output size; both even. FPS is the output
	// frame rate.
	Width, Height, FPS int
	// Subtitles, if set, is a local ASS file (captions, hook text) burned
	// in after joining, with fonts from FontsDir.
	Subtitles, FontsDir string
	// Voiceover, if set, is mixed in from VoiceoverAt for VoiceoverLength,
	// with the parts' own audio at Duck (0 to 1) while it plays.
	Voiceover                    string
	VoiceoverAt, VoiceoverLength time.Duration
	Duck                         float64
}

// Length is the montage's total length.
func (m Montage) Length() time.Duration {
	var d time.Duration
	for _, p := range m.Parts {
		d += p.Length
	}
	return d
}

func secs(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }

// filterGraph builds the filter_complex for m. Every value in it is a
// number CAVI computed or a path checked against safePath.
func (m Montage) filterGraph() (string, error) {
	var g strings.Builder
	join := ""
	for i, p := range m.Parts {
		if p.Length <= 0 || p.CropW <= 0 || p.CropH <= 0 {
			return "", fmt.Errorf("ffmpeg: invalid montage part %d", i)
		}
		cur := fmt.Sprintf("[%d:v]", i)
		for k, b := range p.Blur {
			if b.W <= 0 || b.H <= 0 || b.To <= b.From {
				return "", fmt.Errorf("ffmpeg: invalid blur box %d in part %d", k, i)
			}
			base, region, blurred, next := fmt.Sprintf("[p%db%da]", i, k), fmt.Sprintf("[p%db%db]", i, k), fmt.Sprintf("[p%db%dc]", i, k), fmt.Sprintf("[p%db%d]", i, k)
			fmt.Fprintf(&g, "%ssplit=2%s%s;%scrop=%d:%d:%d:%d,gblur=sigma=%d%s;%s%soverlay=%d:%d:enable='between(t,%s,%s)'%s;",
				cur, base, region, region, b.W, b.H, b.X, b.Y, max(12, min(b.W, b.H)/4), blurred,
				base, blurred, b.X, b.Y, secs(b.From), secs(b.To), next)
			cur = next
		}
		fmt.Fprintf(&g, "%scrop=%d:%d:%d:%d,scale=%d:%d:flags=lanczos,setsar=1,fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[v%d];",
			cur, p.CropW, p.CropH, p.CropX, p.CropY, m.Width, m.Height, m.FPS, secs(p.Length), i)
		if p.Audio {
			fmt.Fprintf(&g, "[%d:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo", i)
			for _, s := range p.Mute {
				fmt.Fprintf(&g, ",volume=0:enable='between(t,%s,%s)'", secs(s.From), secs(s.To))
			}
			fmt.Fprintf(&g, ",apad,atrim=duration=%s,asetpts=PTS-STARTPTS[a%d];", secs(p.Length), i)
		} else {
			fmt.Fprintf(&g, "anullsrc=r=48000:cl=stereo,atrim=duration=%s[a%d];", secs(p.Length), i)
		}
		join += fmt.Sprintf("[v%d][a%d]", i, i)
	}
	fmt.Fprintf(&g, "%sconcat=n=%d:v=1:a=1[vc][ac];", join, len(m.Parts))
	if m.Subtitles != "" {
		if !safePath.MatchString(m.Subtitles) || (m.FontsDir != "" && !safePath.MatchString(m.FontsDir)) {
			return "", fmt.Errorf("ffmpeg: subtitle or font path has characters not allowed in a filter")
		}
		g.WriteString("[vc]subtitles=filename=" + m.Subtitles)
		if m.FontsDir != "" {
			g.WriteString(":fontsdir=" + m.FontsDir)
		}
		g.WriteString("[vout];")
	} else {
		g.WriteString("[vc]null[vout];")
	}
	if m.Voiceover != "" {
		if m.Duck < 0 || m.Duck > 1 || m.VoiceoverLength <= 0 {
			return "", fmt.Errorf("ffmpeg: invalid voiceover")
		}
		at := m.VoiceoverAt.Milliseconds()
		// Synthesised voices arrive at full scale: level them to spoken-word
		// loudness with headroom, and limit the mix so nothing clips.
		fmt.Fprintf(&g, "[%d:a]highpass=f=70,loudnorm=I=-16:TP=-2:LRA=7,aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,adelay=%d|%d,apad[vo];", len(m.Parts), at, at)
		fmt.Fprintf(&g, "[ac]volume=%s:enable='between(t,%s,%s)'[ad];[ad][vo]amix=inputs=2:duration=first:normalize=0,alimiter=limit=0.95:level=disabled[aout]",
			strconv.FormatFloat(m.Duck, 'f', 2, 64), secs(m.VoiceoverAt), secs(m.VoiceoverAt+m.VoiceoverLength))
	} else {
		g.WriteString("[ac]anull[aout]")
	}
	return g.String(), nil
}

// Montage renders m to the local file output, replacing it. Each part is
// seeked before its input and re-encoded, so cuts are frame-accurate.
func (r *Runner) Montage(ctx context.Context, output string, m Montage, timeout time.Duration) error {
	if len(m.Parts) == 0 || m.Width <= 0 || m.Height <= 0 || m.Width%2 != 0 || m.Height%2 != 0 || m.FPS <= 0 {
		return fmt.Errorf("ffmpeg: invalid montage")
	}
	graph, err := m.filterGraph()
	if err != nil {
		return err
	}
	args := []string{"-v", "error", "-nostdin", "-y", "-protocol_whitelist", protocols}
	for _, p := range m.Parts {
		args = append(args, "-ss", secs(p.Start), "-t", secs(p.Length), "-i", p.Input)
	}
	if m.Voiceover != "" {
		args = append(args, "-i", m.Voiceover)
	}
	args = append(args,
		"-filter_complex", graph, "-map", "[vout]", "-map", "[aout]",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-profile:v", "high",
		"-c:a", "aac", "-b:a", "160k", "-ar", "48000",
		"-t", secs(m.Length()), "-movflags", "+faststart", "-f", "mp4", output)
	_, err = r.runFor(ctx, timeout, m.Parts[0].Input, r.FFmpeg, args...)
	return err
}
