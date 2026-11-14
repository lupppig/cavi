package render

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/lupppig/cavi/apps/api/internal/ai"
)

// Caption styles. Both are black and white.
const (
	// CaptionsBold is heavy white text with a black outline.
	CaptionsBold = "bold"
	// CaptionsBoxed is white text on a solid black box.
	CaptionsBoxed = "boxed"
)

// Limits on captions.
const (
	maxCues      = 2000
	maxCueRunes  = 160
	minCueMS     = 400
	captionLines = 2
)

// Cue is one caption, timed from the start of the clip.
type Cue struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"` // lines separated by "\n"
}

// Captions are burned into the clip. The cues are copied from the
// transcript when the render is requested, so re-rendering gives the same
// result even if the transcript changes later.
type Captions struct {
	Style string `json:"style"`
	Cues  []Cue  `json:"cues"`
}

func validStyle(s string) bool { return s == CaptionsBold || s == CaptionsBoxed }

// captionLayout is the caption size and placement for an output shape,
// in output pixels. The preview in the web app uses the same rules.
type captionLayout struct {
	FontSize, MarginV, MaxChars int
}

func layoutFor(sh Shape) captionLayout {
	size := int(math.Round(float64(min(sh.Width, sh.Height)) * 0.075))
	margin := 0.08
	if sh.Height > sh.Width {
		// Keep clear of the buttons and captions vertical feeds overlay.
		margin = 0.22
	}
	return captionLayout{
		FontSize: size,
		MarginV:  int(math.Round(float64(sh.Height) * margin)),
		// Roughly the characters of bold text that fit in 84% of the width.
		MaxChars: int(float64(sh.Width) * 0.84 / (float64(size) * 0.62)),
	}
}

// wrap breaks text into lines of at most maxChars, then groups them into
// cues of up to captionLines lines. Words longer than a line are kept whole.
func wrap(text string, maxChars int) []string {
	var lines []string
	var cur string
	for _, w := range strings.Fields(text) {
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) <= maxChars:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	var cues []string
	for i := 0; i < len(lines); i += captionLines {
		cues = append(cues, strings.Join(lines[i:min(i+captionLines, len(lines))], "\n"))
	}
	return cues
}

// BuildCues turns the transcript segments overlapping [startMS, endMS)
// into caption cues for the given shape, timed from the clip's start. A
// long segment becomes several cues, its time shared out by length; the
// parts outside the clip are dropped.
func BuildCues(segments []ai.Segment, startMS, endMS int64, aspect string) []Cue {
	sh, _ := shape(aspect)
	lay := layoutFor(sh)
	var cues []Cue
	for _, seg := range segments {
		if seg.EndMS <= startMS || seg.StartMS >= endMS {
			continue
		}
		parts := wrap(seg.Text, lay.MaxChars)
		total := 0
		for _, p := range parts {
			total += utf8.RuneCountInString(p)
		}
		at := float64(seg.StartMS)
		span := float64(seg.EndMS - seg.StartMS)
		for _, p := range parts {
			d := span * float64(utf8.RuneCountInString(p)) / float64(max(total, 1))
			s, e := int64(math.Round(at)), int64(math.Round(at+d))
			at += d
			s, e = max(s, startMS), min(e, endMS)
			if e-s < minCueMS/2 {
				continue // mostly outside the clip
			}
			c := Cue{StartMS: s - startMS, EndMS: e - startMS, Text: p}
			if n := len(cues); n > 0 && c.StartMS < cues[n-1].EndMS {
				c.StartMS = cues[n-1].EndMS // overlapping speech: one caption at a time
			}
			if c.EndMS-c.StartMS < minCueMS {
				c.EndMS = min(c.StartMS+minCueMS, endMS-startMS)
			}
			if c.EndMS > c.StartMS {
				cues = append(cues, c)
			}
		}
	}
	return cues
}

// validate checks captions against a clip of lengthMS.
func (c *Captions) validate(lengthMS int64) error {
	if !validStyle(c.Style) {
		return invalid("captions.style", "Choose bold or boxed captions.")
	}
	if len(c.Cues) == 0 {
		return invalid("captions", "There’s no speech in this part of the transcript to caption.")
	}
	if len(c.Cues) > maxCues {
		return invalid("captions", "This clip has too many captions.")
	}
	var prev int64
	for i, q := range c.Cues {
		if q.StartMS < prev || q.EndMS <= q.StartMS || q.EndMS > lengthMS || strings.TrimSpace(q.Text) == "" ||
			utf8.RuneCountInString(q.Text) > maxCueRunes || !utf8.ValidString(q.Text) {
			return invalid("captions", fmt.Sprintf("Caption %d isn’t valid for this clip.", i+1))
		}
		prev = q.EndMS
	}
	return nil
}

// assText makes cue text safe for ASS: braces start override blocks and a
// backslash starts escapes, so they are replaced; lines become \N.
func assText(s string) string {
	s = strings.NewReplacer("{", "(", "}", ")", `\`, "/", "\r", "").Replace(s)
	return strings.ReplaceAll(s, "\n", `\N`)
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
	lay := layoutFor(sh)
	// Colours are &HAABBGGRR.
	border, outline, back := 1, int(math.Round(float64(lay.FontSize)*0.09)), "&H00000000"
	if c.Style == CaptionsBoxed {
		border, outline = 3, int(math.Round(float64(lay.FontSize)*0.2)) // the box's padding
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\nScaledBorderAndShadow: yes\n\n", sh.Width, sh.Height)
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Cap,Inter Black,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,%s,-1,0,0,0,100,100,0,0,%d,%d,0,2,%d,%d,%d,1\n\n",
		lay.FontSize, back, border, outline, sh.Width/12, sh.Width/12, lay.MarginV)
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, q := range c.Cues {
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Cap,,0,0,0,,%s\n", assTime(q.StartMS), assTime(q.EndMS), assText(q.Text))
	}
	return b.String()
}
