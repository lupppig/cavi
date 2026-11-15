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
	// Stack shows more scenes at the same time: the part's own scene and
	// these are stacked top to bottom in equal panels (a split screen).
	// They are silent; the part's own audio plays.
	Stack []Tile
}

// Tile is another scene played alongside a part, from Start for the
// part's length, cropped like a part.
type Tile struct {
	Input                      string
	Start                      time.Duration
	CropX, CropY, CropW, CropH int
}

// PanelHeight is each panel's height when n scenes share an output of
// height h: even, so encoders accept it.
func PanelHeight(h, n int) int { return (h / n) &^ 1 }

// Montage joins parts, one after another, into one clip.
type Montage struct {
	Parts []MontagePart
	// Width and Height are the output size; both even. FPS is the output
	// frame rate.
	Width, Height, FPS int
	// Subtitles, if set, is a local ASS file (captions, hook text) burned
	// in after joining, with fonts from FontsDir.
	Subtitles, FontsDir string
	// Voices are narration tracks (a voiceover, a spoken sign-off) mixed
	// in over the parts' own audio.
	Voices []Voice
	// Music, if set, is a background track looped to fill the montage at
	// MusicVolume (0 to 1), faded in and out, and lowered further while
	// anyone speaks (dialogue or voiceover) so the words stay clear.
	Music       string
	MusicVolume float64
}

// Voice is a narration track played from At for Length, with the parts'
// own audio at Duck (0 to 1) while it plays.
type Voice struct {
	Input      string
	At, Length time.Duration
	Duck       float64
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
	next := len(m.Parts) // tiles are inputs after every part's own
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
		if len(p.Stack) == 0 {
			fmt.Fprintf(&g, "%scrop=%d:%d:%d:%d,scale=%d:%d:flags=lanczos,setsar=1,fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[v%d];",
				cur, p.CropW, p.CropH, p.CropX, p.CropY, m.Width, m.Height, m.FPS, secs(p.Length), i)
		} else {
			n := 1 + len(p.Stack)
			ph := PanelHeight(m.Height, n)
			panels := fmt.Sprintf("[p%dt0]", i)
			fmt.Fprintf(&g, "%scrop=%d:%d:%d:%d,scale=%d:%d:flags=lanczos,setsar=1[p%dt0];", cur, p.CropW, p.CropH, p.CropX, p.CropY, m.Width, ph, i)
			for k, t := range p.Stack {
				if t.CropW <= 0 || t.CropH <= 0 {
					return "", fmt.Errorf("ffmpeg: invalid tile %d in part %d", k, i)
				}
				fmt.Fprintf(&g, "[%d:v]crop=%d:%d:%d:%d,scale=%d:%d:flags=lanczos,setsar=1,setpts=PTS-STARTPTS[p%dt%d];",
					next, t.CropW, t.CropH, t.CropX, t.CropY, m.Width, ph, i, k+1)
				panels += fmt.Sprintf("[p%dt%d]", i, k+1)
				next++
			}
			fmt.Fprintf(&g, "%svstack=inputs=%d,pad=%d:%d:0:%d:black", panels, n, m.Width, m.Height, (m.Height-n*ph)/2)
			for k := 1; k < n; k++ {
				fmt.Fprintf(&g, ",drawbox=x=0:y=%d:w=%d:h=6:color=white:t=fill", (m.Height-n*ph)/2+k*ph-3, m.Width)
			}
			fmt.Fprintf(&g, ",fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[v%d];", m.FPS, secs(p.Length), i)
		}
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
	if len(m.Voices) == 0 {
		g.WriteString("[ac]anull[speech];")
	} else {
		// Synthesised voices arrive at full scale: level them to spoken-word
		// loudness with headroom, and limit the mix so nothing clips. The
		// parts' audio is lowered under each.
		mix := "[ad]"
		g.WriteString("[ac]")
		for k, v := range m.Voices {
			if v.Duck < 0 || v.Duck > 1 || v.Length <= 0 || v.At < 0 {
				return "", fmt.Errorf("ffmpeg: invalid voice %d", k)
			}
			if k > 0 {
				g.WriteString(",")
			}
			fmt.Fprintf(&g, "volume=%s:enable='between(t,%s,%s)'", strconv.FormatFloat(v.Duck, 'f', 2, 64), secs(v.At), secs(v.At+v.Length))
		}
		g.WriteString("[ad];")
		for k, v := range m.Voices {
			at := v.At.Milliseconds()
			fmt.Fprintf(&g, "[%d:a]highpass=f=70,loudnorm=I=-16:TP=-2:LRA=7,aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,"+
				"atrim=duration=%s,adelay=%d|%d,apad[vo%d];", next, secs(v.Length), at, at, k)
			mix += fmt.Sprintf("[vo%d]", k)
			next++
		}
		fmt.Fprintf(&g, "%samix=inputs=%d:duration=first:normalize=0[speech];", mix, 1+len(m.Voices))
	}
	if m.Music == "" {
		g.WriteString("[speech]alimiter=limit=0.95:level=disabled[aout]")
		return g.String(), nil
	}
	if m.MusicVolume <= 0 || m.MusicVolume > 1 {
		return "", fmt.Errorf("ffmpeg: invalid music volume")
	}
	total := m.Length()
	fade := min(1500*time.Millisecond, total/4)
	// The music sits under the speech: its level is set, it fades in and
	// out, and a compressor keyed by the speech pulls it down further
	// whenever anyone talks.
	fmt.Fprintf(&g, "[%d:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,atrim=duration=%s,asetpts=PTS-STARTPTS,"+
		"volume=%s,afade=t=in:d=%s,afade=t=out:st=%s:d=%s[bed];",
		next, secs(total), strconv.FormatFloat(m.MusicVolume, 'f', 2, 64), secs(fade/2), secs(total-fade), secs(fade))
	g.WriteString("[speech]asplit=2[sp][key];[bed][key]sidechaincompress=threshold=0.02:ratio=6:attack=30:release=600:makeup=1[ducked];" +
		"[sp][ducked]amix=inputs=2:duration=first:normalize=0,alimiter=limit=0.95:level=disabled[aout]")
	return g.String(), nil
}

// localFormats are the containers a local input may be read as: the video
// formats uploads are checked to be.
const localFormats = "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm,avi"

// input is the options and -i for one input. Input options apply only to
// the -i they precede, so each input gets its own whitelist: URLs may only
// use network protocols; a local file (an absolute path CAVI made) may only
// be read as a file, in a video container.
func input(in string) []string {
	if strings.HasPrefix(in, "/") {
		return []string{"-protocol_whitelist", "file", "-format_whitelist", localFormats, "-i", "file:" + in}
	}
	return []string{"-protocol_whitelist", protocols, "-i", in}
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
	args := []string{"-v", "error", "-nostdin", "-y"}
	for _, p := range m.Parts {
		args = append(args, "-ss", secs(p.Start), "-t", secs(p.Length))
		args = append(args, input(p.Input)...)
	}
	for _, p := range m.Parts {
		for _, t := range p.Stack {
			args = append(args, "-ss", secs(t.Start), "-t", secs(p.Length))
			args = append(args, input(t.Input)...)
		}
	}
	for _, v := range m.Voices {
		args = append(args, input(v.Input)...)
	}
	if m.Music != "" {
		args = append(args, "-stream_loop", "-1")
		args = append(args, input(m.Music)...)
	}
	args = append(args,
		"-filter_complex", graph, "-map", "[vout]", "-map", "[aout]",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-profile:v", "high",
		"-c:a", "aac", "-b:a", "160k", "-ar", "48000",
		"-t", secs(m.Length()), "-movflags", "+faststart", "-f", "mp4", output)
	_, err = r.runFor(ctx, timeout, m.Parts[0].Input, r.FFmpeg, args...)
	return err
}
