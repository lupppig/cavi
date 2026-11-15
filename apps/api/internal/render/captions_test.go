package render

import (
	"strings"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/ai"
)

func TestLayout(t *testing.T) {
	v, _ := shape("9:16")
	w, _ := shape("16:9")
	for _, tc := range []struct {
		sh                            Shape
		mode, pos                     string
		size, margin, maxChars, align int
	}{
		{v, ModeLines, PositionBottom, 81, 422, 18, 2},
		{w, ModeLines, PositionBottom, 81, 86, 32, 2},
		{v, ModeWords, PositionBottom, 103, 422, 14, 2},
		{v, ModeWords, PositionMiddle, 103, 0, 14, 5},
		{v, ModeLines, PositionTop, 81, 269, 18, 8},
	} {
		got := layoutFor(tc.sh, "", tc.mode, tc.pos)
		if got.FontSize != tc.size || got.MarginV != tc.margin || got.MaxChars != tc.maxChars || got.Alignment != tc.align {
			t.Errorf("%s %s %s = %+v", tc.sh.Aspect, tc.mode, tc.pos, got)
		}
	}
}

func words(start int64, step int64, text string) []ai.Word {
	var out []ai.Word
	for i, f := range strings.Fields(text) {
		s := start + int64(i)*step
		out = append(out, ai.Word{StartMS: s, EndMS: s + step, Text: f})
	}
	return out
}

func TestChunk(t *testing.T) {
	ws := words(0, 100, "So I stood on a chair and played the whole set unplugged")
	lines := chunk(ws, 18, ModeLines)
	got := []string{}
	for _, cue := range lines {
		var ls []string
		for _, l := range cue {
			var ts []string
			for _, w := range l {
				ts = append(ts, w.Text)
			}
			ls = append(ls, strings.Join(ts, " "))
		}
		got = append(got, strings.Join(ls, "/"))
	}
	if strings.Join(got, "|") != "So I stood on a/chair and played|the whole set/unplugged" {
		t.Errorf("lines = %q", got)
	}
	if n := len(chunk(ws, 14, ModeWords)); n != 4 { // 12 words, 3 a cue
		t.Errorf("word cues = %d", n)
	}
}

func TestBuildCues(t *testing.T) {
	segs := []ai.Segment{
		{StartMS: 0, EndMS: 4000, Speaker: "Speaker 1", Text: "Way before the clip.", Words: words(0, 1000, "Way before the clip.")},
		// Straddles the in point: only words mostly inside are kept.
		{StartMS: 9000, EndMS: 12000, Speaker: "Speaker 2", Text: "Your first ever gig.", Words: words(9000, 750, "Your first ever gig.")},
		// No word times: estimated by length.
		{StartMS: 12000, EndMS: 20000, Speaker: "Speaker 2", Text: "So I stood on a chair and played the whole set unplugged."},
		{StartMS: 19500, EndMS: 22000, Speaker: "Speaker 1", Text: "No way.", Words: words(19500, 1000, "No way.")},
	}
	cues := BuildCues(segs, 10000, 30000, "9:16", ModeLines)
	if len(cues) != 4 {
		t.Fatalf("%d cues: %+v", len(cues), cues)
	}
	// "Your" (9000-9750) is mostly before the clip; "first" starts at 9750.
	if c := cues[0]; c.Text != "first ever gig." || c.StartMS != 0 || c.EndMS != 2000 || c.Speaker != "Speaker 2" || len(c.Words) != 3 {
		t.Errorf("first cue = %+v", c)
	}
	// Estimated words chain without gaps from the segment's start.
	if cues[1].StartMS != 2000 || cues[2].EndMS != 10000 || cues[2].StartMS != cues[1].EndMS {
		t.Errorf("estimated cues = %+v", cues[1:3])
	}
	// Overlapping speech waits for the previous caption.
	if c := cues[3]; c.Text != "No way." || c.StartMS != 10000 || c.Words[0].StartMS != 10000 {
		t.Errorf("overlap cue = %+v", c)
	}
	caps := Captions{CaptionOptions: CaptionOptions{Style: CaptionsBold}, Cues: cues}
	if err := caps.validate(20000); err != nil {
		t.Fatalf("built cues invalid: %v", err)
	}
	if got := BuildCues(segs, 10000, 30000, "9:16", ModeWords); len(got) != 6 || len(got[0].Words) != 3 {
		t.Errorf("word mode cues = %+v", got)
	}
	if got := BuildCues(segs, 30000, 35000, "9:16", ModeLines); len(got) != 0 {
		t.Errorf("cues in a silent stretch: %+v", got)
	}
}

func TestCaptionOptions(t *testing.T) {
	o, err := CaptionOptions{Style: CaptionsBold}.Normalize()
	if err != nil || o.Mode != ModeLines || o.Position != PositionBottom || o.Colors != DefaultColors {
		t.Fatalf("defaults = %+v, %v", o, err)
	}
	for field, bad := range map[string]CaptionOptions{
		"captions.style":          {Style: "neon"},
		"captions.mode":           {Style: CaptionsBold, Mode: "karaoke"},
		"captions.position":       {Style: CaptionsBold, Position: "left"},
		"captions.colors":         {Style: CaptionsBold, Colors: CaptionColors{Text: "green", Highlight: "#FFFFFF", Stroke: "#000000"}},
		"captions.speaker_colors": {Style: CaptionsBold, SpeakerColors: map[string]string{"Speaker 1": "#12345"}},
	} {
		_, err := bad.Normalize()
		wantField(t, err, field)
	}
}

func TestValidateCaptions(t *testing.T) {
	cue := func(s, e int64, text string) Cue {
		var ws []CueWord
		for _, f := range strings.Fields(text) {
			ws = append(ws, CueWord{StartMS: s, EndMS: e, Text: f})
		}
		return Cue{StartMS: s, EndMS: e, Text: text, Words: ws}
	}
	bold := CaptionOptions{Style: CaptionsBold}
	ok := []Cue{cue(0, 1000, "a"), cue(1000, 2000, "b")}
	mismatched := cue(0, 1000, "two words")
	mismatched.Words = mismatched.Words[:1]
	for name, c := range map[string]Captions{
		"style":       {CaptionOptions: CaptionOptions{Style: "rainbow"}, Cues: ok},
		"empty":       {CaptionOptions: bold},
		"overlap":     {CaptionOptions: bold, Cues: []Cue{cue(0, 1500, "a"), cue(1000, 2000, "b")}},
		"past end":    {CaptionOptions: bold, Cues: []Cue{cue(0, 6000, "a")}},
		"blank":       {CaptionOptions: bold, Cues: []Cue{{StartMS: 0, EndMS: 1000, Text: "  "}}},
		"too long":    {CaptionOptions: bold, Cues: []Cue{cue(0, 1000, strings.Repeat("x", maxCueRunes+1))}},
		"backwards":   {CaptionOptions: bold, Cues: []Cue{cue(900, 800, "a")}},
		"word count":  {CaptionOptions: bold, Cues: []Cue{mismatched}},
		"word timing": {CaptionOptions: bold, Cues: []Cue{{StartMS: 0, EndMS: 1000, Text: "a", Words: []CueWord{{StartMS: 500, EndMS: 1200, Text: "a"}}}}},
	} {
		if err := c.validate(5000); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := Captions{CaptionOptions: CaptionOptions{Style: CaptionsBoxed}, Cues: ok}
	if err := c.validate(5000); err != nil {
		t.Fatal(err)
	}
	// Captions need version 2.
	comp := Composition{Version: 1, SourceAssetID: "x", EndMS: 5000, Aspect: "9:16", Focus: 0.5, Captions: &c}
	wantField(t, comp.Validate(landscape), "composition.version")
	comp.Version = 2
	if err := comp.Validate(landscape); err != nil {
		t.Fatal(err)
	}
}

func TestASS(t *testing.T) {
	q := Cue{StartMS: 1230, EndMS: 3000, Text: "Line {one}\nback\\slash", Speaker: "Speaker 2", Words: []CueWord{
		{StartMS: 1230, EndMS: 1700, Text: "Line"}, {StartMS: 1700, EndMS: 2200, Text: "{one}"}, {StartMS: 2200, EndMS: 3000, Text: "back\\slash"},
	}}
	lines := ASS(Captions{CaptionOptions: CaptionOptions{Style: CaptionsBoxed, Colors: CaptionColors{Text: "#FFFFFF", Highlight: "#39FF14", Stroke: "#8B4513"}}, Cues: []Cue{q}}, "9:16")
	for _, want := range []string{
		"PlayResX: 1080\nPlayResY: 1920",
		// Brown box (#8B4513 is &H0013458B in ASS byte order), padding 16.
		"Style: Cap,Inter Black,81,&H00FFFFFF,&H00FFFFFF,&H0013458B,&H0013458B,-1,0,0,0,100,100,0,0,3,16,0,2,90,90,422,1",
		`Dialogue: 0,0:00:01.23,0:00:03.00,Cap,,0,0,0,,{\1c&H00FFFFFF&}Line (one)\Nback/slash`,
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("lines ASS lacks %q:\n%s", want, lines)
		}
	}

	words := ASS(Captions{CaptionOptions: CaptionOptions{Style: CaptionsBold, Mode: ModeWords, Uppercase: true, Position: PositionMiddle,
		SpeakerColors: map[string]string{"Speaker 2": "#FF7A00"}}, Cues: []Cue{q}}, "9:16")
	// One event per word; the spoken word is highlighted (default yellow),
	// the rest take the speaker's orange; upper case.
	for _, want := range []string{
		",5,90,90,0,1",
		`Dialogue: 0,0:00:01.23,0:00:01.70,Cap,,0,0,0,,{\1c&H00007AFF&}{\1c&H0000E6FF&\fscx110\fscy110}LINE{\1c&H00007AFF&\fscx100\fscy100} (ONE)\NBACK/SLASH`,
		`Dialogue: 0,0:00:02.20,0:00:03.00,Cap,,0,0,0,,{\1c&H00007AFF&}LINE (ONE)\N{\1c&H0000E6FF&\fscx110\fscy110}BACK/SLASH{\1c&H00007AFF&\fscx100\fscy100}`,
	} {
		if !strings.Contains(words, want) {
			t.Errorf("words ASS lacks %q:\n%s", want, words)
		}
	}
	if n := strings.Count(words, "Dialogue:"); n != 3 {
		t.Errorf("%d word events", n)
	}
}

func TestHighlightStaysVisible(t *testing.T) {
	green := CaptionOptions{Colors: CaptionColors{Text: "#39FF14", Highlight: "#FFFFFF", Stroke: "#000000"}}
	if got := highlightFor("#39FF14", green); got != "#FFFFFF" {
		t.Errorf("normal highlight = %s", got)
	}
	// A white-voiced speaker with a white highlight: use the text colour.
	if got := highlightFor("#ffffff", green); got != "#39FF14" {
		t.Errorf("clashing highlight = %s", got)
	}
	white := CaptionOptions{Colors: CaptionColors{Text: "#FFFFFF", Highlight: "#FFFFFF", Stroke: "#000000"}}
	if got := highlightFor("#FFFFFF", white); got != "#FFE600" {
		t.Errorf("all-white highlight = %s", got)
	}
}

func TestCuesExcludeWordsBeforeTheCut(t *testing.T) {
	// A long line whose last word is stretched to the line's end, past
	// the clip's start: "everyone." was said before the clip.
	seg := ai.Segment{StartMS: 193_000, EndMS: 212_000, Speaker: "Speaker 3", Text: "It belongs to everyone.",
		Words: []ai.Word{{StartMS: 204_000, EndMS: 204_200, Text: "It"}, {StartMS: 204_200, EndMS: 204_500, Text: "belongs"},
			{StartMS: 204_500, EndMS: 204_800, Text: "to"}, {StartMS: 205_000, EndMS: 212_000, Text: "everyone."}}}
	next := ai.Segment{StartMS: 206_500, EndMS: 208_000, Speaker: "Speaker 3", Text: "And that's why",
		Words: []ai.Word{{StartMS: 206_500, EndMS: 206_800, Text: "And"}, {StartMS: 206_800, EndMS: 207_000, Text: "that's"}, {StartMS: 207_000, EndMS: 208_000, Text: "why"}}}
	cues := BuildCues([]ai.Segment{seg, next}, 206_400, 210_000, "9:16", ModeWords)
	for _, q := range cues {
		if strings.Contains(q.Text, "everyone") {
			t.Fatalf("captioned a word said before the clip: %+v", cues)
		}
	}
	if len(cues) == 0 || !strings.HasPrefix(cues[0].Text, "And") {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestPopCaptions(t *testing.T) {
	o, err := CaptionOptions{Style: CaptionsPop, Mode: ModeLines}.Normalize()
	if err != nil || o.Mode != ModeWords || !o.Uppercase {
		t.Fatalf("pop options = %+v, %v", o, err)
	}
	c := Captions{CaptionOptions: o, Cues: []Cue{{StartMS: 0, EndMS: 1000, Text: "no way bro",
		Words: []CueWord{{0, 300, "no"}, {300, 600, "way"}, {600, 1000, "bro"}}}}}
	ass := ASS(c, "9:16")
	// Outline and a shadow, capitals, and the spoken word bouncing in.
	if !strings.Contains(ass, ",&H80000000,-1,") || !strings.Contains(ass, `\t(0,90,\fscx125\fscy125)`) || !strings.Contains(ass, "WAY") {
		t.Fatalf("pop ASS = %s", ass)
	}
	if n := strings.Count(ass, "Dialogue:"); n != 3 {
		t.Fatalf("want one event per word, got %d", n)
	}
}
