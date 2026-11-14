package ai

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// Timing evaluation: transcripts are cut on, so their timestamps must be
// right, not merely inside the audio. podcast-two-voices.mp3 is a 151 s
// synthetic two-voice podcast (espeak-ng); podcast-two-voices.json has
// each line's true start and end. A model passes when, on every run,
// the transcript is valid, nearly every line is found, the median start
// error is at most 1 s and 90% of lines start within 2 s.

type truthLine struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

func loadTruth(t *testing.T) (int64, []truthLine) {
	t.Helper()
	b, err := os.ReadFile("testdata/podcast-two-voices.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		DurationMS int64       `json:"duration_ms"`
		Lines      []truthLine `json:"lines"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.DurationMS, f.Lines
}

type timedWord struct {
	w    string
	ms   float64
	line int
}

// timedWords spreads each span's words evenly across the span.
func timedWords[T any](spans []T, get func(T) (int64, int64, string)) []timedWord {
	var out []timedWord
	for i, sp := range spans {
		start, end, text := get(sp)
		ws := words(text)
		for j, w := range ws {
			out = append(out, timedWord{w: w, ms: float64(start) + float64(end-start)*float64(j)/float64(max(len(ws), 1)), line: i})
		}
	}
	return out
}

// startErrors aligns the transcript's words to the true words (longest
// common subsequence, so split or merged segments don't matter) and
// returns, for each true line with a matched word, the absolute error in
// seconds at its first matched word, sorted.
func startErrors(truth []truthLine, segs []Segment) []float64 {
	tw := timedWords(truth, func(l truthLine) (int64, int64, string) { return l.StartMS, l.EndMS, l.Text })
	sw := timedWords(segs, func(s Segment) (int64, int64, string) { return s.StartMS, s.EndMS, s.Text })
	n, m := len(tw), len(sw)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if tw[i].w == sw[j].w {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	first := map[int]float64{}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case tw[i].w == sw[j].w:
			if _, ok := first[tw[i].line]; !ok {
				first[tw[i].line] = math.Abs(sw[j].ms-tw[i].ms) / 1000
			}
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	errs := make([]float64, 0, len(first))
	for _, e := range first {
		errs = append(errs, e)
	}
	sort.Float64s(errs)
	return errs
}

func TestStartErrors(t *testing.T) {
	truth := []truthLine{{StartMS: 0, Text: "hello there friend"}, {StartMS: 5000, Text: "how are you today"}, {StartMS: 9000, Text: "hello there friend"}}
	segs := []Segment{{StartMS: 300, Text: "Hello there, friend."}, {StartMS: 4000, Text: "How are you today?"}, {StartMS: 9100, Text: "Hello there friend"}}
	got := startErrors(truth, segs)
	if len(got) != 3 || math.Abs(got[0]-0.1) > 1e-9 || math.Abs(got[1]-0.3) > 1e-9 || math.Abs(got[2]-1) > 1e-9 {
		t.Fatalf("errors = %v", got)
	}
	// Two true lines merged into one segment: the second line's first word
	// is placed by its position in the segment (4.5 s against 5 s).
	merged := startErrors(
		[]truthLine{{StartMS: 0, EndMS: 4000, Text: "one two three four"}, {StartMS: 5000, EndMS: 9000, Text: "five six seven eight"}},
		[]Segment{{StartMS: 0, EndMS: 9000, Text: "One two three four. Five six seven eight."}})
	if len(merged) != 2 || merged[0] != 0 || math.Abs(merged[1]-0.5) > 1e-9 {
		t.Fatalf("merged = %v", merged)
	}
}

func TestEvalTranscriptionTiming(t *testing.T) {
	g := evalProvider(t)
	dur, truth := loadTruth(t)
	for _, m := range Models(TaskTranscription) {
		t.Run(m.ID, func(t *testing.T) {
			for run := 0; run < 3; run++ {
				tr := transcribe(t, g, m, "podcast-two-voices.mp3", dur)
				errs := startErrors(truth, tr.Segments)
				if len(errs) < len(truth)-3 {
					t.Fatalf("run %d: found %d of %d lines", run, len(errs), len(truth))
				}
				med, p90 := errs[len(errs)/2], errs[len(errs)*9/10]
				t.Logf("run %d: %d segments, start error median %.2fs, p90 %.2fs", run, len(tr.Segments), med, p90)
				if med > 1 || p90 > 2 {
					t.Errorf("run %d: timestamps drift (median %.1fs, p90 %.1fs)", run, med, p90)
				}
			}
		})
	}
}
