package render

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
)

// MontageVersion is the composition version for montages: one or more
// segments, from one or more sources, played one after another, with
// optional blur boxes, muted ranges and hook text.
const MontageVersion = 3

// Montage limits.
const (
	MaxSegments       = 20
	MinSegment        = 500 * time.Millisecond
	MaxTreatments     = 20 // blur boxes or muted ranges per segment
	MaxHookRunes      = 120
	minBlurFraction   = 0.02
	minHookMS         = 500
	montageFPS        = 30
	maxMontageSources = 10
)

// Range is a stretch of a segment's source, in source milliseconds.
type Range struct {
	StartMS int64 `json:"start_ms"`
	EndMS   int64 `json:"end_ms"`
}

// BlurBox hides a region of a segment's source frame for a stretch of it.
// The region is given as fractions of the source frame (0 to 1), so it
// doesn't depend on the output shape.
type BlurBox struct {
	Range
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Segment is one stretch of one source in a montage.
type Segment struct {
	SourceAssetID string  `json:"source_asset_id"`
	StartMS       int64   `json:"start_ms"`
	EndMS         int64   `json:"end_ms"`
	Focus         float64 `json:"focus"`
	// Mute silences the source's audio, e.g. uncleared music.
	Mute []Range `json:"mute,omitempty"`
	// Blur hides parts of the frame, e.g. licence plates.
	Blur []BlurBox `json:"blur,omitempty"`
}

// Hook is text shown at the top of the clip from its first frame until
// EndMS (clip time), to make the opening land.
type Hook struct {
	Text  string `json:"text"`
	EndMS int64  `json:"end_ms"`
}

// LengthMS is the montage's total length.
func (c Composition) LengthMS() int64 {
	if c.Version != MontageVersion {
		return c.EndMS - c.StartMS
	}
	var n int64
	for _, s := range c.Segments {
		n += s.EndMS - s.StartMS
	}
	return n
}

// SourceIDs returns the distinct sources a composition uses, in order.
func (c Composition) SourceIDs() []string {
	if c.Version != MontageVersion {
		return []string{c.SourceAssetID}
	}
	var out []string
	seen := map[string]bool{}
	for _, s := range c.Segments {
		if !seen[s.SourceAssetID] {
			seen[s.SourceAssetID] = true
			out = append(out, s.SourceAssetID)
		}
	}
	return out
}

// within reports whether r lies inside the segment.
func (s Segment) within(r Range) bool {
	return r.StartMS >= s.StartMS && r.EndMS <= s.EndMS && r.EndMS > r.StartMS
}

// ValidateMontage checks a version-3 composition against its sources.
// The top-level source and times mirror the first segment.
func (c *Composition) ValidateMontage(srcs map[string]Source) error {
	if len(c.Segments) == 0 {
		return invalid("composition.segments", "Add at least one segment.")
	}
	if len(c.Segments) > MaxSegments {
		return invalid("composition.segments", fmt.Sprintf("A clip can have at most %d segments.", MaxSegments))
	}
	if len(c.SourceIDs()) > maxMontageSources {
		return invalid("composition.segments", fmt.Sprintf("A clip can use at most %d files.", maxMontageSources))
	}
	if _, ok := shape(c.Aspect); !ok {
		return invalid("composition.aspect", "Choose 9:16, 1:1, 4:5 or 16:9.")
	}
	for i, s := range c.Segments {
		field := fmt.Sprintf("composition.segments[%d]", i)
		src, ok := srcs[s.SourceAssetID]
		if !ok || src.Width < 2 || src.Height < 2 || src.DurationMS <= 0 {
			return invalid(field+".source_asset_id", "Choose a processed video from this project.")
		}
		if s.StartMS < 0 || s.EndMS > src.DurationMS || s.EndMS-s.StartMS < MinSegment.Milliseconds() {
			return invalid(field+".end_ms", "Each segment must be at least half a second, within its video.")
		}
		if math.IsNaN(s.Focus) || s.Focus < 0 || s.Focus > 1 {
			return invalid(field+".focus", "Framing must be between 0 and 1.")
		}
		if len(s.Mute) > MaxTreatments || len(s.Blur) > MaxTreatments {
			return invalid(field, fmt.Sprintf("A segment can have at most %d muted ranges and %d blur boxes.", MaxTreatments, MaxTreatments))
		}
		for j, m := range s.Mute {
			if !s.within(m) {
				return invalid(fmt.Sprintf("%s.mute[%d]", field, j), "Muted ranges must be inside their segment.")
			}
		}
		for j, b := range s.Blur {
			f := fmt.Sprintf("%s.blur[%d]", field, j)
			if !s.within(b.Range) {
				return invalid(f, "Blur boxes must be inside their segment’s time.")
			}
			for _, v := range []float64{b.X, b.Y, b.W, b.H} {
				if math.IsNaN(v) || v < 0 || v > 1 {
					return invalid(f, "Blur boxes must be inside the frame.")
				}
			}
			if b.W < minBlurFraction || b.H < minBlurFraction || b.X+b.W > 1.0001 || b.Y+b.H > 1.0001 {
				return invalid(f, "Blur boxes must be inside the frame and not tiny.")
			}
		}
	}
	total := c.LengthMS()
	if total < MinClip.Milliseconds() {
		return invalid("composition.segments", "Clips must be at least a second long.")
	}
	if total > MaxClip.Milliseconds() {
		return invalid("composition.segments", fmt.Sprintf("Clips can be at most %d minutes long.", int(MaxClip.Minutes())))
	}
	if c.Hook != nil {
		c.Hook.Text = strings.Join(strings.Fields(c.Hook.Text), " ")
		if c.Hook.Text == "" || !utf8.ValidString(c.Hook.Text) || utf8.RuneCountInString(c.Hook.Text) > MaxHookRunes {
			return invalid("composition.hook.text", fmt.Sprintf("Write a hook of up to %d characters.", MaxHookRunes))
		}
		if c.Hook.EndMS < minHookMS || c.Hook.EndMS > total {
			return invalid("composition.hook.end_ms", "Show the hook for at least half a second, within the clip.")
		}
	}
	if c.Captions != nil {
		if err := c.Captions.validate(total); err != nil {
			return err
		}
	}
	first := c.Segments[0]
	c.SourceAssetID, c.StartMS, c.EndMS, c.Focus = first.SourceAssetID, first.StartMS, first.EndMS, first.Focus
	return nil
}

// region is the part of a source frame shown in an output of shape sh,
// placed by focus: the same framing rule single clips use.
func region(sh Shape, src Source, focus float64) (x, y, w, h int) {
	sw, shh := src.Width, src.Height
	if sw*sh.Height > shh*sh.Width {
		h = shh - shh%2
		w = min(even(float64(shh)*float64(sh.Width)/float64(sh.Height)), sw-sw%2)
		x = int(math.Round(float64(sw-w) * focus))
	} else {
		w = sw - sw%2
		h = min(even(float64(sw)*float64(sh.Height)/float64(sh.Width)), shh-shh%2)
		y = int(math.Round(float64(shh-h) * focus))
	}
	return
}

// PlanMontage works out the FFmpeg montage for a valid version-3
// composition. inputs maps each source to where FFmpeg reads it.
func PlanMontage(c Composition, srcs map[string]Source, inputs map[string]string) ffmpeg.Montage {
	sh, _ := shape(c.Aspect)
	m := ffmpeg.Montage{Width: sh.Width, Height: sh.Height, FPS: montageFPS}
	ms := func(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
	for _, s := range c.Segments {
		src := srcs[s.SourceAssetID]
		x, y, w, h := region(sh, src, s.Focus)
		p := ffmpeg.MontagePart{
			Input: inputs[s.SourceAssetID], Start: ms(s.StartMS), Length: ms(s.EndMS - s.StartMS),
			CropX: x, CropY: y, CropW: w, CropH: h, Audio: src.HasAudio,
		}
		for _, r := range s.Mute {
			p.Mute = append(p.Mute, ffmpeg.Span{From: ms(r.StartMS - s.StartMS), To: ms(r.EndMS - s.StartMS)})
		}
		for _, b := range s.Blur {
			bx, by := int(math.Floor(b.X*float64(src.Width))), int(math.Floor(b.Y*float64(src.Height)))
			bw := min(int(math.Ceil(b.W*float64(src.Width))), src.Width-bx)
			bh := min(int(math.Ceil(b.H*float64(src.Height))), src.Height-by)
			p.Blur = append(p.Blur, ffmpeg.Box{X: bx, Y: by, W: max(bw, 2), H: max(bh, 2),
				Span: ffmpeg.Span{From: ms(b.StartMS - s.StartMS), To: ms(b.EndMS - s.StartMS)}})
		}
		m.Parts = append(m.Parts, p)
	}
	return m
}

// MontageCaptions builds caption cues for each segment from its source's
// transcript and places them on the montage's timeline.
func MontageCaptions(segs []Segment, transcripts map[string][]ai.Segment, aspect, mode string) []Cue {
	var out []Cue
	var offset int64
	for _, s := range segs {
		for _, q := range BuildCues(transcripts[s.SourceAssetID], s.StartMS, s.EndMS, aspect, mode) {
			q.StartMS += offset
			q.EndMS += offset
			for i := range q.Words {
				q.Words[i].StartMS += offset
				q.Words[i].EndMS += offset
			}
			out = append(out, q)
		}
		offset += s.EndMS - s.StartMS
	}
	return out
}

// OverlayASS is the subtitle file for a montage: its captions (if any) and
// its hook (if any). Empty when there is nothing to burn in.
func OverlayASS(c *Captions, h *Hook, aspect string) string {
	if c == nil && h == nil {
		return ""
	}
	sh, _ := shape(aspect)
	var s string
	if c != nil {
		s = ASS(*c, aspect)
	} else {
		s = fmt.Sprintf("[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\nScaledBorderAndShadow: yes\n\n", sh.Width, sh.Height) +
			"[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n\n" +
			"[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
	}
	if h == nil {
		return s
	}
	// White text on a black box, top centre, clear of platform UI.
	size := sh.Width / 15
	style := fmt.Sprintf("Style: Hook,Inter Black,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,3,%d,0,8,%d,%d,%d,1\n",
		size, size/4, sh.Width/10, sh.Width/10, sh.Height/9)
	s = strings.Replace(s, "\n[Events]", style+"\n[Events]", 1)
	return s + fmt.Sprintf("Dialogue: 1,%s,%s,Hook,,0,0,0,,%s\n", assTime(0), assTime(h.EndMS), assText(h.Text, false))
}
