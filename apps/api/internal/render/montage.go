package render

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
)

// MontageVersion is the composition version for montages: one or more
// segments, from one or more sources, played one after another, with
// optional blur boxes, muted ranges and hook text.
const MontageVersion = 3

// Montage limits.
const (
	MaxSegments   = 20
	MaxTiles      = 2 // extra scenes in a split-screen segment
	MinSegment    = 500 * time.Millisecond
	MaxTreatments = 20 // blur boxes or muted ranges per segment
	MaxHookRunes  = 120
	// MaxGaps is how many pauses can be cut from one segment.
	MaxGaps         = 60
	MaxSignOffRunes = 80
	// MaxBadgeRunes is the longest badge ("PART 2/3").
	MaxBadgeRunes = 24
	// SignOffMS is how long a silent sign-off card shows; a spoken one
	// shows while it's said, plus signOffTailMS.
	SignOffMS     = 2500
	signOffTailMS = 500
	// signOffDuck is the clip's own sound under a spoken sign-off.
	signOffDuck       = 0.2
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
	// Tiles turn the segment into a split screen: these scenes play at the
	// same time as the segment's own, stacked top to bottom (the
	// segment's own on top), each for the segment's length. Only the
	// segment's own sound plays.
	Tiles []Tile `json:"tiles,omitempty"`
	// Gaps are pauses cut out of the segment (source time, ascending),
	// worked out by the server when the composition asks to tighten.
	Gaps []Range `json:"gaps,omitempty"`
}

// Pieces are the stretches of source the segment plays: its range less
// its gaps.
func (s Segment) Pieces() []Range {
	out := []Range{}
	from := s.StartMS
	for _, g := range s.Gaps {
		if g.StartMS > from {
			out = append(out, Range{from, g.StartMS})
		}
		from = max(from, g.EndMS)
	}
	if s.EndMS > from {
		out = append(out, Range{from, s.EndMS})
	}
	return out
}

// PlayMS is how long the segment plays, gaps cut.
func (s Segment) PlayMS() int64 {
	var n int64
	for _, p := range s.Pieces() {
		n += p.EndMS - p.StartMS
	}
	return n
}

// Tile is another scene in a split-screen segment.
type Tile struct {
	SourceAssetID string  `json:"source_asset_id"`
	StartMS       int64   `json:"start_ms"`
	Focus         float64 `json:"focus"`
}

// Ranges returns every stretch of source a segment shows: its own and its
// tiles', for checking against briefs.
func (s Segment) Ranges() []Segment {
	out := []Segment{{SourceAssetID: s.SourceAssetID, StartMS: s.StartMS, EndMS: s.EndMS}}
	for _, t := range s.Tiles {
		out = append(out, Segment{SourceAssetID: t.SourceAssetID, StartMS: t.StartMS, EndMS: t.StartMS + s.EndMS - s.StartMS})
	}
	return out
}

// Hook is text shown at the top of the clip from its first frame until
// EndMS (clip time), to make the opening land.
type Hook struct {
	Text  string `json:"text"`
	EndMS int64  `json:"end_ms"`
}

// VoiceoverMix places a ready voiceover in a montage.
type VoiceoverMix struct {
	VoiceoverID string `json:"voiceover_id"`
	// StartMS is where it starts, in clip time.
	StartMS int64 `json:"start_ms"`
	// Duck is the clip's own audio level (0 to 1) while it plays.
	Duck float64 `json:"duck"`
	// DurationMS is copied from the voiceover when the render is requested.
	DurationMS int64 `json:"duration_ms"`
}

// SignOff is the clip's ending: a card with Text over the last moments,
// optionally said by the narrator (a ready voiceover), e.g. "Follow for
// more". Captions give way to the card.
type SignOff struct {
	Text        string `json:"text"`
	VoiceoverID string `json:"voiceover_id,omitempty"`
	// VoiceDurationMS is copied from the voiceover when the render is
	// requested.
	VoiceDurationMS int64 `json:"voice_duration_ms,omitempty"`
}

// LengthMS is how long the card shows.
func (s SignOff) LengthMS() int64 {
	if s.VoiceoverID == "" {
		return SignOffMS
	}
	return max(s.VoiceDurationMS+signOffTailMS, 1500)
}

// SignOffStartMS is where a montage's sign-off begins, in clip time; the
// clip's length when it has none.
func (c Composition) SignOffStartMS() int64 {
	if c.SignOff == nil {
		return c.LengthMS()
	}
	return max(c.LengthMS()-c.SignOff.LengthMS(), 0)
}

// LengthMS is the montage's total length.
func (c Composition) LengthMS() int64 {
	if c.Version != MontageVersion {
		return c.EndMS - c.StartMS
	}
	var n int64
	for _, s := range c.Segments {
		n += s.PlayMS()
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
		for _, r := range s.Ranges() {
			if !seen[r.SourceAssetID] {
				seen[r.SourceAssetID] = true
				out = append(out, r.SourceAssetID)
			}
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
	if err := validFraming(*c); err != nil {
		return err
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
		if len(s.Tiles) > MaxTiles {
			return invalid(field+".tiles", fmt.Sprintf("A split screen can show at most %d scenes.", MaxTiles+1))
		}
		for j, t := range s.Tiles {
			f := fmt.Sprintf("%s.tiles[%d]", field, j)
			ts, ok := srcs[t.SourceAssetID]
			if !ok || ts.Width < 2 || ts.Height < 2 {
				return invalid(f+".source_asset_id", "Choose a processed video from this project.")
			}
			if t.StartMS < 0 || t.StartMS+(s.EndMS-s.StartMS) > ts.DurationMS {
				return invalid(f+".start_ms", "Each scene must run for the whole segment, within its video.")
			}
			if math.IsNaN(t.Focus) || t.Focus < 0 || t.Focus > 1 {
				return invalid(f+".focus", "Framing must be between 0 and 1.")
			}
		}
		if len(s.Gaps) > MaxGaps {
			return invalid(field+".gaps", fmt.Sprintf("A segment can have at most %d pauses cut.", MaxGaps))
		}
		if len(s.Gaps) > 0 && len(s.Tiles) > 0 {
			return invalid(field+".gaps", "Pauses can’t be cut from a split screen.")
		}
		prev := s.StartMS
		for j, g := range s.Gaps {
			if g.StartMS < prev || g.EndMS <= g.StartMS || g.EndMS > s.EndMS {
				return invalid(fmt.Sprintf("%s.gaps[%d]", field, j), "Cut pauses must be inside their segment, in order.")
			}
			prev = g.EndMS
		}
		if s.PlayMS() < MinSegment.Milliseconds() {
			return invalid(field+".gaps", "Each segment must still play for at least half a second with its pauses cut.")
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
	c.Badge = strings.Join(strings.Fields(c.Badge), " ")
	if !utf8.ValidString(c.Badge) || utf8.RuneCountInString(c.Badge) > MaxBadgeRunes {
		return invalid("composition.badge", fmt.Sprintf("Keep the badge to %d characters, like PART 2/3.", MaxBadgeRunes))
	}
	if v := c.Voiceover; v != nil {
		if math.IsNaN(v.Duck) || v.Duck < 0 || v.Duck > 1 {
			return invalid("composition.voiceover.duck", "The clip’s own sound under the voiceover must be between 0 and 1.")
		}
		if v.StartMS < 0 || v.DurationMS <= 0 || v.StartMS+v.DurationMS > total {
			return invalid("composition.voiceover.start_ms", fmt.Sprintf(
				"The voiceover (%.1f s) must fit inside the clip (%.1f s). Start it earlier or make the clip longer.",
				float64(v.DurationMS)/1000, float64(total)/1000))
		}
	}
	if so := c.SignOff; so != nil {
		so.Text = strings.Join(strings.Fields(so.Text), " ")
		if so.Text == "" || !utf8.ValidString(so.Text) || utf8.RuneCountInString(so.Text) > MaxSignOffRunes {
			return invalid("composition.sign_off.text", fmt.Sprintf("Write a sign-off of up to %d characters.", MaxSignOffRunes))
		}
		if so.VoiceoverID != "" && so.VoiceDurationMS <= 0 {
			return invalid("composition.sign_off.voiceover_id", "Choose a finished voiceover from this project.")
		}
		from := total - so.LengthMS()
		if from < 0 {
			return invalid("composition.sign_off", fmt.Sprintf(
				"The sign-off (%.1f s) is longer than the clip (%.1f s).", float64(so.LengthMS())/1000, float64(total)/1000))
		}
		if v := c.Voiceover; v != nil && v.StartMS+v.DurationMS > from {
			return invalid("composition.voiceover.start_ms", fmt.Sprintf(
				"The voiceover runs into the sign-off, which starts at %.1f s. Start it earlier or make the clip longer.", float64(from)/1000))
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
// composition. inputs maps each source to where FFmpeg reads it; tracks
// holds the framing analysis of each segment's source when c is auto
// framed.
func PlanMontage(c Composition, srcs map[string]Source, inputs map[string]string, tracks map[string]framing.Track) ffmpeg.Montage {
	sh, _ := shape(c.Aspect)
	m := ffmpeg.Montage{Width: sh.Width, Height: sh.Height, FPS: montageFPS}
	ms := func(v int64) time.Duration { return time.Duration(v) * time.Millisecond }
	for _, s := range c.Segments {
		src := srcs[s.SourceAssetID]
		panel := sh
		if len(s.Tiles) > 0 {
			panel = Shape{Aspect: sh.Aspect, Width: sh.Width, Height: ffmpeg.PanelHeight(sh.Height, 1+len(s.Tiles))}
		}
		x, y, w, h := region(panel, src, s.Focus)
		p := ffmpeg.MontagePart{
			Input: inputs[s.SourceAssetID], Start: ms(s.StartMS), Length: ms(s.EndMS - s.StartMS),
			CropX: x, CropY: y, CropW: w, CropH: h, Audio: src.HasAudio,
		}
		if t, ok := tracks[s.SourceAssetID]; ok && c.Auto() {
			p.Pan, p.PanVertical = pan(t.Path(s.StartMS, s.EndMS, window(panel, src), s.Focus), panel, src)
		}
		for _, t := range s.Tiles {
			tx, ty, tw, th := region(panel, srcs[t.SourceAssetID], t.Focus)
			p.Stack = append(p.Stack, ffmpeg.Tile{Input: inputs[t.SourceAssetID], Start: ms(t.StartMS), CropX: tx, CropY: ty, CropW: tw, CropH: th})
		}
		if len(s.Gaps) > 0 {
			for _, k := range s.Pieces() {
				p.Keep = append(p.Keep, ffmpeg.Span{From: ms(k.StartMS - s.StartMS), To: ms(k.EndMS - s.StartMS)})
			}
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
		for _, p := range s.Pieces() {
			for _, q := range BuildCues(transcripts[s.SourceAssetID], p.StartMS, p.EndMS, aspect, mode) {
				q.StartMS += offset
				q.EndMS += offset
				for i := range q.Words {
					q.Words[i].StartMS += offset
					q.Words[i].EndMS += offset
				}
				out = append(out, q)
			}
			offset += p.EndMS - p.StartMS
		}
	}
	return out
}

// OverlayASS is the subtitle file for a montage: its captions, hook and
// sign-off card (each if any; the card from signOffAt, clip time, to
// lengthMS). Empty when there is nothing to burn in.
func OverlayASS(c *Captions, h *Hook, so *SignOff, signOffAt, lengthMS int64, aspect, badge string) string {
	if c == nil && h == nil && so == nil && badge == "" {
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
	if h != nil {
		// White text on a black box, top centre, clear of platform UI.
		size := sh.Width / 15
		style := fmt.Sprintf("Style: Hook,Inter Black,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,3,%d,0,8,%d,%d,%d,1\n",
			size, size/4, sh.Width/10, sh.Width/10, sh.Height/9)
		s = strings.Replace(s, "\n[Events]", style+"\n[Events]", 1)
		// The script turns wrapping off for captions (they are split
		// already); \q0 turns smart wrapping back on for the hook.
		s += fmt.Sprintf("Dialogue: 1,%s,%s,Hook,,0,0,0,,{\\q0}%s\n", assTime(0), assTime(h.EndMS), assText(h.Text, false))
	}
	if badge != "" {
		// A small yellow label at the top left, above the hook, until the
		// sign-off card.
		size := sh.Width / 24
		style := fmt.Sprintf("Style: Badge,Inter Black,%d,&H00000000,&H00000000,&H0000E6FF,&H0000E6FF,-1,0,0,0,100,100,0,0,3,%d,0,7,%d,%d,%d,1\n",
			size, size/3, sh.Width/14, sh.Width/14, sh.Height/24)
		s = strings.Replace(s, "\n[Events]", style+"\n[Events]", 1)
		s += fmt.Sprintf("Dialogue: 3,%s,%s,Badge,,0,0,0,,%s\n", assTime(0), assTime(signOffAt), assText(badge, false))
	}
	if so != nil {
		// Black text on a yellow box in the middle, popping in.
		size := sh.Width / 12
		style := fmt.Sprintf("Style: SignOff,Inter Black,%d,&H00000000,&H00000000,&H0000E6FF,&H0000E6FF,-1,0,0,0,100,100,0,0,3,%d,0,5,%d,%d,0,1\n",
			size, size/3, sh.Width/10, sh.Width/10)
		s = strings.Replace(s, "\n[Events]", style+"\n[Events]", 1)
		s += fmt.Sprintf("Dialogue: 2,%s,%s,SignOff,,0,0,0,,{\\q0\\fscx40\\fscy40\\t(0,180,\\fscx110\\fscy110)\\t(180,280,\\fscx100\\fscy100)}%s\n",
			assTime(signOffAt), assTime(lengthMS), assText(so.Text, false))
	}
	return s
}

// BeforeSignOff ends captions where the sign-off card starts: cues that
// start later are dropped, and one running into it is cut short.
func BeforeSignOff(cues []Cue, fromMS int64) []Cue {
	out := make([]Cue, 0, len(cues))
	for _, q := range cues {
		if q.StartMS >= fromMS {
			continue
		}
		if q.EndMS > fromMS {
			q.EndMS = fromMS
			ws := q.Words[:0:0]
			for _, w := range q.Words {
				w.StartMS, w.EndMS = min(w.StartMS, fromMS), min(w.EndMS, fromMS)
				ws = append(ws, w)
			}
			q.Words = ws
		}
		out = append(out, q)
	}
	return out
}

// VoiceoverCues are caption cues for a voiceover's script, timed by its
// words, placed at startMS on the clip's timeline.
func VoiceoverCues(script string, words []ai.Word, durationMS, startMS int64, aspect, mode string) []Cue {
	seg := ai.Segment{StartMS: 0, EndMS: durationMS, Speaker: "Voiceover", Text: script, Words: words}
	cues := BuildCues([]ai.Segment{seg}, 0, durationMS, aspect, mode)
	for i := range cues {
		cues[i].StartMS += startMS
		cues[i].EndMS += startMS
		for j := range cues[i].Words {
			cues[i].Words[j].StartMS += startMS
			cues[i].Words[j].EndMS += startMS
		}
	}
	return cues
}

// WithVoiceover captions the voiceover while it plays: dialogue cues that
// overlap it are dropped (the dialogue is lowered under it anyway).
func WithVoiceover(cues, vo []Cue) []Cue {
	if len(vo) == 0 {
		return cues
	}
	from, to := vo[0].StartMS, vo[len(vo)-1].EndMS
	out := make([]Cue, 0, len(cues)+len(vo))
	for _, q := range cues {
		if q.EndMS <= from || q.StartMS >= to {
			out = append(out, q)
		}
	}
	out = append(out, vo...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartMS < out[j].StartMS })
	return out
}
