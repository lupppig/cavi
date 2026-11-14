package render

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/lupppig/cavi/apps/api/internal/ai"
)

// Caption options.
const (
	// CaptionsBold is heavy text with an outline.
	CaptionsBold = "bold"
	// CaptionsBoxed is text on a solid box.
	CaptionsBoxed = "boxed"

	// ModeLines shows whole lines; ModeWords shows a few words at a time
	// and highlights the one being said.
	ModeLines = "lines"
	ModeWords = "words"

	PositionBottom = "bottom"
	PositionMiddle = "middle"
	PositionTop    = "top"
)

// Limits on captions.
const (
	maxCues          = 2000
	maxCueRunes      = 160
	maxCueWords      = 60
	minCueMS         = 400
	captionLines     = 2
	wordsPerCue      = 3
	maxSpeakerColors = 20
)

// DefaultColors are white text, a yellow highlight and a black outline.
var DefaultColors = CaptionColors{Text: "#FFFFFF", Highlight: "#FFE600", Stroke: "#000000"}

// CaptionColors are #RRGGBB colours. Stroke is the outline (bold) or the
// box (boxed).
type CaptionColors struct {
	Text      string `json:"text"`
	Highlight string `json:"highlight"`
	Stroke    string `json:"stroke"`
}

// CaptionOptions are what a person chooses; cues are built from them.
type CaptionOptions struct {
	Style     string        `json:"style"`
	Mode      string        `json:"mode,omitempty"`
	Position  string        `json:"position,omitempty"`
	Uppercase bool          `json:"uppercase,omitempty"`
	Colors    CaptionColors `json:"colors"`
	// SpeakerColors gives a speaker label its own text colour.
	SpeakerColors map[string]string `json:"speaker_colors,omitempty"`
}

// CueWord is one word of a cue, timed from the start of the clip.
type CueWord struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// Cue is one caption, timed from the start of the clip.
type Cue struct {
	StartMS int64     `json:"start_ms"`
	EndMS   int64     `json:"end_ms"`
	Text    string    `json:"text"` // lines separated by "\n"; its words are Words
	Speaker string    `json:"speaker,omitempty"`
	Words   []CueWord `json:"words"`
}

// Captions are burned into the clip. The cues are copied from the
// transcript when the render is requested, so re-rendering gives the same
// result even if the transcript changes later.
type Captions struct {
	CaptionOptions
	Cues []Cue `json:"cues"`
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// Normalize fills defaults and checks the options.
func (o CaptionOptions) Normalize() (CaptionOptions, error) {
	if !oneOf(o.Style, CaptionsBold, CaptionsBoxed) {
		return o, invalid("captions.style", "Choose bold or boxed captions.")
	}
	if o.Mode == "" {
		o.Mode = ModeLines
	}
	if !oneOf(o.Mode, ModeLines, ModeWords) {
		return o, invalid("captions.mode", "Choose lines or word by word.")
	}
	if o.Position == "" {
		o.Position = PositionBottom
	}
	if !oneOf(o.Position, PositionBottom, PositionMiddle, PositionTop) {
		return o, invalid("captions.position", "Choose bottom, middle or top.")
	}
	if o.Colors == (CaptionColors{}) {
		o.Colors = DefaultColors
	}
	for _, c := range []string{o.Colors.Text, o.Colors.Highlight, o.Colors.Stroke} {
		if !hexColor.MatchString(c) {
			return o, invalid("captions.colors", "Colours must be #RRGGBB.")
		}
	}
	if len(o.SpeakerColors) > maxSpeakerColors {
		return o, invalid("captions.speaker_colors", "Too many speaker colours.")
	}
	for label, c := range o.SpeakerColors {
		if !hexColor.MatchString(c) || label == "" || utf8.RuneCountInString(label) > 40 {
			return o, invalid("captions.speaker_colors", "Speaker colours must be #RRGGBB for a speaker label.")
		}
	}
	return o, nil
}

// captionLayout is the caption size and placement for a shape, in output
// pixels. The preview in the web app uses the same rules.
type captionLayout struct {
	FontSize, MarginV, MaxChars, Alignment int
}

func layoutFor(sh Shape, mode, position string) captionLayout {
	scale := 0.075
	if mode == ModeWords {
		scale = 0.095 // fewer words, bigger
	}
	size := int(math.Round(float64(min(sh.Width, sh.Height)) * scale))
	vertical := sh.Height > sh.Width
	lay := captionLayout{
		FontSize: size,
		// Roughly the characters of bold text that fit in 84% of the width.
		MaxChars: int(float64(sh.Width) * 0.84 / (float64(size) * 0.62)),
	}
	switch position {
	case PositionMiddle:
		lay.Alignment = 5
	case PositionTop:
		lay.Alignment = 8
		lay.MarginV = int(math.Round(float64(sh.Height) * map[bool]float64{true: 0.14, false: 0.08}[vertical]))
	default:
		lay.Alignment = 2
		// Vertical feeds overlay buttons and text at the bottom.
		lay.MarginV = int(math.Round(float64(sh.Height) * map[bool]float64{true: 0.22, false: 0.08}[vertical]))
	}
	return lay
}

// timedWords returns the segment's words, using the transcript's word
// times when it has them and otherwise sharing the segment's time out by
// each word's length.
func timedWords(seg ai.Segment) []ai.Word {
	if len(seg.Words) > 0 {
		return seg.Words
	}
	fields := strings.Fields(seg.Text)
	total := 0
	for _, f := range fields {
		total += utf8.RuneCountInString(f) + 1
	}
	out := make([]ai.Word, len(fields))
	at, span := float64(seg.StartMS), float64(seg.EndMS-seg.StartMS)
	for i, f := range fields {
		d := span * float64(utf8.RuneCountInString(f)+1) / float64(max(total, 1))
		out[i] = ai.Word{StartMS: int64(math.Round(at)), EndMS: int64(math.Round(at + d)), Text: f}
		at += d
	}
	return out
}

// chunk groups words into cues: up to two lines of maxChars each, and in
// word mode at most wordsPerCue words. It returns, per cue, its lines of
// words.
func chunk(ws []ai.Word, maxChars int, mode string) [][][]ai.Word {
	var cues [][][]ai.Word
	var lines [][]ai.Word
	var line []ai.Word
	lineLen, cueWords := 0, 0
	flush := func() {
		if len(line) > 0 {
			lines = append(lines, line)
			line, lineLen = nil, 0
		}
	}
	flushCue := func() {
		flush()
		if len(lines) > 0 {
			cues = append(cues, lines)
			lines, cueWords = nil, 0
		}
	}
	for _, w := range ws {
		n := utf8.RuneCountInString(w.Text)
		if mode == ModeWords && cueWords == wordsPerCue {
			flushCue()
		}
		if len(line) > 0 && lineLen+1+n > maxChars {
			flush()
			if len(lines) == captionLines {
				flushCue()
			}
		}
		if len(line) > 0 {
			lineLen++
		}
		line = append(line, w)
		lineLen += n
		cueWords++
	}
	flushCue()
	return cues
}

// BuildCues turns the transcript segments overlapping [startMS, endMS)
// into caption cues for a shape and mode, timed from the clip's start.
// Cues start and end on word times (estimated where the transcript has
// none); words mostly outside the clip are dropped.
func BuildCues(segments []ai.Segment, startMS, endMS int64, aspect, mode string) []Cue {
	sh, _ := shape(aspect)
	lay := layoutFor(sh, mode, PositionBottom)
	var cues []Cue
	for _, seg := range segments {
		if seg.EndMS <= startMS || seg.StartMS >= endMS {
			continue
		}
		var in []ai.Word
		for _, w := range timedWords(seg) {
			if mid := (w.StartMS + w.EndMS) / 2; mid >= startMS && mid < endMS {
				in = append(in, w)
			}
		}
		for _, lines := range chunk(in, lay.MaxChars, mode) {
			c := Cue{Speaker: seg.Speaker}
			var text []string
			for _, line := range lines {
				var ts []string
				for _, w := range line {
					ts = append(ts, w.Text)
					c.Words = append(c.Words, CueWord{
						StartMS: max(w.StartMS, startMS) - startMS,
						EndMS:   min(w.EndMS, endMS) - startMS,
						Text:    w.Text,
					})
				}
				text = append(text, strings.Join(ts, " "))
			}
			c.Text = strings.Join(text, "\n")
			c.StartMS, c.EndMS = c.Words[0].StartMS, c.Words[len(c.Words)-1].EndMS
			if n := len(cues); n > 0 && c.StartMS < cues[n-1].EndMS {
				c.StartMS = cues[n-1].EndMS // overlapping speech: one caption at a time
			}
			if c.EndMS-c.StartMS < minCueMS {
				c.EndMS = min(c.StartMS+minCueMS, endMS-startMS)
			}
			if c.EndMS <= c.StartMS {
				continue
			}
			for i := range c.Words {
				w := &c.Words[i]
				w.StartMS = min(max(w.StartMS, c.StartMS), c.EndMS)
				w.EndMS = min(max(w.EndMS, w.StartMS), c.EndMS)
			}
			cues = append(cues, c)
		}
	}
	return cues
}

// validate checks captions against a clip of lengthMS.
func (c *Captions) validate(lengthMS int64) error {
	opts, err := c.CaptionOptions.Normalize()
	if err != nil {
		return err
	}
	c.CaptionOptions = opts
	if len(c.Cues) == 0 {
		return invalid("captions", "There’s no speech in this part of the transcript to caption.")
	}
	if len(c.Cues) > maxCues {
		return invalid("captions", "This clip has too many captions.")
	}
	var prev int64
	for i, q := range c.Cues {
		bad := invalid("captions", fmt.Sprintf("Caption %d isn’t valid for this clip.", i+1))
		if q.StartMS < prev || q.EndMS <= q.StartMS || q.EndMS > lengthMS || strings.TrimSpace(q.Text) == "" ||
			utf8.RuneCountInString(q.Text) > maxCueRunes || !utf8.ValidString(q.Text) || len(q.Words) > maxCueWords {
			return bad
		}
		if len(q.Words) != len(strings.Fields(q.Text)) {
			return bad
		}
		at := q.StartMS
		for _, w := range q.Words {
			if w.StartMS < at || w.EndMS < w.StartMS || w.EndMS > q.EndMS || strings.TrimSpace(w.Text) == "" {
				return bad
			}
			at = w.StartMS
		}
		prev = q.EndMS
	}
	return nil
}

// highlightFor returns the colour for the spoken word over text of colour
// base: the chosen highlight, or, when that matches the text (so the
// highlight would be invisible), the first of the caption's text colour,
// yellow and green that doesn't.
func highlightFor(base string, o CaptionOptions) string {
	for _, c := range []string{o.Colors.Highlight, o.Colors.Text, "#FFE600", "#39FF14"} {
		if !strings.EqualFold(c, base) {
			return c
		}
	}
	return o.Colors.Highlight
}

// assColor turns #RRGGBB into ASS &H00BBGGRR (opaque).
func assColor(hex string) string {
	h := strings.TrimPrefix(hex, "#")
	return "&H00" + strings.ToUpper(h[4:6]+h[2:4]+h[0:2])
}

// assText makes text safe for ASS: braces start override blocks and a
// backslash starts escapes, so they are replaced.
func assText(s string, upper bool) string {
	s = strings.NewReplacer("{", "(", "}", ")", `\`, "/", "\r", "").Replace(s)
	if upper {
		s = strings.ToUpper(s)
	}
	return s
}

func assTime(ms int64) string {
	cs := ms / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

// ASS renders captions as an Advanced SubStation Alpha script for an
// output of the given shape. Sizes are in output pixels (PlayRes is the
// output size). Fonts come from Inter, installed in the render worker.
func ASS(c Captions, aspect string) string {
	sh, _ := shape(aspect)
	opts, _ := c.CaptionOptions.Normalize()
	lay := layoutFor(sh, opts.Mode, opts.Position)
	border, outline := 1, int(math.Round(float64(lay.FontSize)*0.09))
	if opts.Style == CaptionsBoxed {
		border, outline = 3, int(math.Round(float64(lay.FontSize)*0.2)) // the box's padding
	}
	text, stroke := assColor(opts.Colors.Text), assColor(opts.Colors.Stroke)
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\nScaledBorderAndShadow: yes\n\n", sh.Width, sh.Height)
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Cap,Inter Black,%d,%s,%s,%s,%s,-1,0,0,0,100,100,0,0,%d,%d,0,%d,%d,%d,%d,1\n\n",
		lay.FontSize, text, text, stroke, stroke, border, outline, lay.Alignment, sh.Width/12, sh.Width/12, lay.MarginV)
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, q := range c.Cues {
		baseHex := opts.Colors.Text
		if col, ok := opts.SpeakerColors[q.Speaker]; ok {
			baseHex = col
		}
		base, hl := assColor(baseHex), assColor(highlightFor(baseHex, opts))
		lineSizes := []int{}
		for _, l := range strings.Split(q.Text, "\n") {
			lineSizes = append(lineSizes, len(strings.Fields(l)))
		}
		// render draws the cue with word k highlighted (-1 for none).
		render := func(k int) string {
			var out strings.Builder
			fmt.Fprintf(&out, `{\1c%s&}`, base)
			i := 0
			for li, n := range lineSizes {
				if li > 0 {
					out.WriteString(`\N`)
				}
				for j := 0; j < n && i < len(q.Words); j++ {
					if j > 0 {
						out.WriteString(" ")
					}
					w := assText(q.Words[i].Text, opts.Uppercase)
					if i == k {
						fmt.Fprintf(&out, `{\1c%s&\fscx110\fscy110}%s{\1c%s&\fscx100\fscy100}`, hl, w, base)
					} else {
						out.WriteString(w)
					}
					i++
				}
			}
			return out.String()
		}
		if opts.Mode != ModeWords || len(q.Words) == 0 {
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Cap,,0,0,0,,%s\n", assTime(q.StartMS), assTime(q.EndMS), render(-1))
			continue
		}
		// One event per word, each showing the whole cue with that word lit.
		for k, w := range q.Words {
			from, to := w.StartMS, q.EndMS
			if k == 0 {
				from = q.StartMS
			}
			if k+1 < len(q.Words) {
				to = q.Words[k+1].StartMS
			}
			if to <= from {
				continue
			}
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Cap,,0,0,0,,%s\n", assTime(from), assTime(to), render(k))
		}
	}
	return b.String()
}
