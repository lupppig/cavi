package ai

import (
	"errors"
	"testing"
)

func TestCatalogue(t *testing.T) {
	for _, task := range Tasks() {
		d := Default(task)
		if d.ID == "" {
			t.Fatalf("%s has no default model", task)
		}
		if _, ok := Lookup(task, d.Provider, d.ID); !ok {
			t.Errorf("%s default not in catalogue", task)
		}
		defaults := 0
		for _, m := range Models(task) {
			if m.Default {
				defaults++
			}
		}
		if defaults != 1 {
			t.Errorf("%s has %d defaults", task, defaults)
		}
	}
	if _, ok := Lookup(TaskTranscription, ProviderGemini, "gemini-made-up"); ok {
		t.Error("unknown model accepted")
	}
}

func TestValidateTranscript(t *testing.T) {
	w := func(at int64, text string) Word { return Word{StartMS: at, Text: text} }
	good := Transcript{Language: "en-GB", Segments: []Segment{
		{StartMS: 5000, EndMS: 9000, Speaker: "Speaker 2", Text: " second ", Words: []Word{w(5100, "second")}},
		{StartMS: 0, EndMS: 4000, Speaker: "", Text: "first", Words: []Word{w(0, "first")}},
		{StartMS: 9000, EndMS: 10500, Speaker: "Speaker 1", Text: "   "},
		{StartMS: 9500, EndMS: 11000, Speaker: "Speaker 1", Text: "runs a little past the end", Words: []Word{w(9500, "runs"), w(9900, "a")}},
	}}
	got, err := ValidateTranscript(good, 10000)
	if err != nil {
		t.Fatal(err)
	}
	// A zero-length segment gets a minimal length rather than failing.
	if z, err := ValidateTranscript(Transcript{Segments: []Segment{{StartMS: 500, EndMS: 500, Text: "hm", Words: []Word{w(500, "hm")}}}}, 10000); err != nil ||
		z.Segments[0].EndMS != 500+minSegmentMS {
		t.Errorf("zero-length segment = %+v, %v", z.Segments, err)
	}
	if len(got.Segments) != 3 || got.Segments[0].Text != "first" || got.Segments[0].Speaker != "Speaker 1" ||
		got.Segments[1].Text != "second" || got.Segments[2].EndMS != 10000 {
		t.Fatalf("normalised = %+v", got.Segments)
	}

	bad := []Transcript{
		{Language: "english please", Segments: nil},
		{Segments: []Segment{{StartMS: -1, EndMS: 100, Text: "x"}}},
		{Segments: []Segment{{StartMS: 500, EndMS: 400, Text: "x"}}},
		{Segments: []Segment{{StartMS: 10000, EndMS: 10100, Text: "after the end"}}},
		{Segments: []Segment{{StartMS: 0, EndMS: 30000, Text: "far past the end"}}},
	}
	for i, b := range bad {
		if _, err := ValidateTranscript(b, 10000); !errors.Is(err, ErrInvalidOutput) {
			t.Errorf("case %d: err = %v", i, err)
		}
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int64{
		"0:07.4": 7400, "0:7.4": 7400, "12:31.0": 751000, "2:26": 146000, "1:02:03.25": 3723250, " 0:00.0 ": 0, "90:00.5": 5400500,
	} {
		if got, ok := parseClock(in); !ok || got != want {
			t.Errorf("parseClock(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"226", "2.26", "7.4", "1:60.0", "1:7a", "abc", "", "-0:01.0"} {
		if _, ok := parseClock(bad); ok {
			t.Errorf("parseClock(%q) accepted", bad)
		}
	}
}

func TestValidateWordsAndSpeakers(t *testing.T) {
	w := func(at int64, text string) Word { return Word{StartMS: at, Text: text} }
	seg := func(start, end int64, speaker string, words ...Word) Segment {
		return Segment{StartMS: start, EndMS: end, Speaker: speaker, Text: "some words here", Words: words}
	}
	tr := Transcript{
		Speakers: []Speaker{{Label: "Speaker 1", Voice: "woman"}, {Label: "Speaker 2", Voice: "robot"}},
		Segments: []Segment{
			// Times slightly outside the segment are clamped; ends chain.
			seg(1000, 3000, "Speaker 1", w(900, "some"), w(1500, "words"), w(3200, "here")),
			// A word far outside its segment: the segment falls back to estimates.
			seg(4000, 6000, "Speaker 2", w(4000, "some"), w(9000, "words")),
			seg(6000, 8000, "Speaker 1", w(6000, "some"), w(6500, "words"), w(7000, "here")),
			seg(8000, 9000, "Speaker 3", w(8000, "some"), w(8400, "words"), w(8800, "here")),
			seg(9000, 9900, "Speaker 1", w(9000, "some"), w(9300, "words"), w(9600, "here")),
		},
	}
	got, err := ValidateTranscript(tr, 10000)
	if err != nil {
		t.Fatal(err)
	}
	first := got.Segments[0].Words
	if len(first) != 3 || first[0].StartMS != 1000 || first[0].EndMS != 1500 || first[2].StartMS != 3000 || first[2].EndMS != 3000 {
		t.Errorf("clamped words = %+v", first)
	}
	if got.Segments[1].Words != nil {
		t.Errorf("implausible words kept: %+v", got.Segments[1].Words)
	}
	// Every label appears once, in order; unknown or missing voices are unclear.
	want := []Speaker{{"Speaker 1", VoiceWoman}, {"Speaker 2", VoiceUnclear}, {"Speaker 3", VoiceUnclear}}
	if len(got.Speakers) != 3 || got.Speakers[0] != want[0] || got.Speakers[1] != want[1] || got.Speakers[2] != want[2] {
		t.Errorf("speakers = %+v", got.Speakers)
	}

	// Words running backwards are distrusted too.
	back := Transcript{Segments: []Segment{seg(0, 2000, "Speaker 1", w(1500, "some"), w(200, "words"))}}
	if _, err := ValidateTranscript(back, 10000); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("only segment without usable words: %v", err) // 1 of 1 > 20%
	}
	// Too many segments without word times: the output is retried.
	tr.Segments[2].Words, tr.Segments[3].Words = nil, nil
	if _, err := ValidateTranscript(tr, 10000); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("3 of 5 segments without words accepted: %v", err)
	}
}
