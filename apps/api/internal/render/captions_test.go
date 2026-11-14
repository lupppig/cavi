package render

import (
	"strings"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/ai"
)

func TestLayout(t *testing.T) {
	v, _ := shape("9:16")
	w, _ := shape("16:9")
	lv, lw := layoutFor(v), layoutFor(w)
	if lv.FontSize != 81 || lw.FontSize != 81 {
		t.Fatalf("font sizes %d, %d", lv.FontSize, lw.FontSize)
	}
	if lv.MarginV != 422 || lw.MarginV != 86 {
		t.Errorf("margins %d, %d", lv.MarginV, lw.MarginV)
	}
	if lv.MaxChars != 18 || lw.MaxChars != 32 {
		t.Errorf("line lengths %d, %d", lv.MaxChars, lw.MaxChars)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("So I stood on a chair and played the whole set unplugged", 18)
	want := []string{"So I stood on a\nchair and played", "the whole set\nunplugged"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap = %q", got)
	}
	if got := wrap("Supercalifragilisticexpialidocious!", 10); len(got) != 1 {
		t.Errorf("long word split: %q", got)
	}
}

func TestBuildCues(t *testing.T) {
	segs := []ai.Segment{
		{StartMS: 0, EndMS: 4000, Text: "Way before the clip starts."},
		{StartMS: 9000, EndMS: 12000, Text: "Your first ever gig."},                                                   // straddles the in point
		{StartMS: 12000, EndMS: 20000, Text: "So I stood on a chair and played the whole set unplugged in the dark."}, // three cues
		{StartMS: 19500, EndMS: 22000, Text: "No way."},                                                               // overlaps the last
		{StartMS: 40000, EndMS: 42000, Text: "After the out point."},
	}
	cues := BuildCues(segs, 10000, 30000, "9:16")
	if len(cues) != 5 {
		t.Fatalf("%d cues: %+v", len(cues), cues)
	}
	if cues[0].StartMS != 0 || cues[0].EndMS != 2000 || cues[0].Text != "Your first ever\ngig." {
		t.Errorf("clipped first cue = %+v", cues[0])
	}
	// The long segment's 8 s are shared out by length, in order, with no gaps.
	if cues[1].StartMS != 2000 || cues[3].EndMS != 10000 || cues[2].StartMS != cues[1].EndMS {
		t.Errorf("long segment cues = %+v", cues[1:4])
	}
	// Overlapping speech waits for the previous caption.
	if cues[4].StartMS != 10000 || cues[4].EndMS != 12000 || cues[4].Text != "No way." {
		t.Errorf("overlap cue = %+v", cues[4])
	}
	c := Captions{Style: CaptionsBold, Cues: cues}
	if err := c.validate(20000); err != nil {
		t.Fatalf("built cues invalid: %v", err)
	}
	if cues := BuildCues(segs, 30000, 35000, "9:16"); len(cues) != 0 {
		t.Errorf("cues in a silent stretch: %+v", cues)
	}
}

func TestValidateCaptions(t *testing.T) {
	ok := []Cue{{StartMS: 0, EndMS: 1000, Text: "a"}, {StartMS: 1000, EndMS: 2000, Text: "b"}}
	for name, c := range map[string]Captions{
		"style":     {Style: "rainbow", Cues: ok},
		"empty":     {Style: CaptionsBold},
		"overlap":   {Style: CaptionsBold, Cues: []Cue{{0, 1500, "a"}, {1000, 2000, "b"}}},
		"past end":  {Style: CaptionsBold, Cues: []Cue{{0, 6000, "a"}}},
		"blank":     {Style: CaptionsBold, Cues: []Cue{{0, 1000, "  "}}},
		"too long":  {Style: CaptionsBold, Cues: []Cue{{0, 1000, strings.Repeat("x", maxCueRunes+1)}}},
		"backwards": {Style: CaptionsBold, Cues: []Cue{{900, 800, "a"}}},
	} {
		if err := c.validate(5000); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := Captions{Style: CaptionsBoxed, Cues: ok}
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
	out := ASS(Captions{Style: CaptionsBoxed, Cues: []Cue{
		{StartMS: 1234, EndMS: 61005, Text: "Line {one}\nback\\slash"},
	}}, "9:16")
	for _, want := range []string{
		"PlayResX: 1080\nPlayResY: 1920",
		"Style: Cap,Inter Black,81,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,3,16,0,2,90,90,422,1",
		`Dialogue: 0,0:00:01.23,0:01:01.00,Cap,,0,0,0,,Line (one)\Nback/slash`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ASS lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(ASS(Captions{Style: CaptionsBold}, "16:9"), ",1,7,0,2,160,160,86,1") {
		t.Error("bold style should be outlined text")
	}
}
