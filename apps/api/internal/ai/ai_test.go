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
	good := Transcript{Language: "en-GB", Segments: []Segment{
		{StartMS: 5000, EndMS: 9000, Speaker: "Speaker 2", Text: " second "},
		{StartMS: 0, EndMS: 4000, Speaker: "", Text: "first"},
		{StartMS: 9000, EndMS: 10500, Speaker: "Speaker 1", Text: "   "},
		{StartMS: 9500, EndMS: 11000, Speaker: "Speaker 1", Text: "runs a little past the end"},
	}}
	got, err := ValidateTranscript(good, 10000)
	if err != nil {
		t.Fatal(err)
	}
	// A zero-length segment gets a minimal length rather than failing.
	if z, err := ValidateTranscript(Transcript{Segments: []Segment{{StartMS: 500, EndMS: 500, Text: "hm"}}}, 10000); err != nil ||
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
