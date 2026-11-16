package media

import (
	"encoding/json"
	"math"
	"testing"
)

// speech makes loudness levels at pauseFPS: talking at -20 dB over a
// -50 dB background, except in the given quiet stretches (ms).
func speech(totalMS int64, quiet ...[2]int64) []float64 {
	n := int(totalMS * pauseFPS / 1000)
	out := make([]float64, n)
	for i := range out {
		ms := int64(i) * 1000 / pauseFPS
		db := -20.0 + 3*math.Sin(float64(i)) // syllables
		for _, q := range quiet {
			if ms >= q[0] && ms < q[1] {
				db = -50 + 2*math.Sin(float64(i))
			}
		}
		out[i] = math.Pow(10, db/20)
	}
	return out
}

func TestFindPauses(t *testing.T) {
	// A long pause, a short breath (too short to keep) and a trailing pause.
	got := FindPauses(speech(20_000, [2]int64{4000, 5200}, [2]int64{9000, 9200}, [2]int64{18_000, 20_000}), pauseFPS)
	want := []Pause{{4000, 5200}, {18_000, 20_000}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pauses = %+v, want %+v", got, want)
	}
	// Quieter speech (a far microphone) is judged against its own level.
	soft := speech(10_000, [2]int64{3000, 4000})
	for i := range soft {
		soft[i] *= 0.05 // 26 dB down: speech at -46 dB, pauses at -76 dB
	}
	if got := FindPauses(soft, pauseFPS); len(got) != 1 || got[0].StartMS != 3000 || got[0].EndMS != 4000 {
		t.Fatalf("pauses in quiet speech = %+v", got)
	}
	// A music bed 10 dB under the voice: its gaps aren't pauses, so
	// nothing is cut.
	bed := speech(10_000, [2]int64{3000, 4000})
	for i := range bed {
		bed[i] = math.Max(bed[i], math.Pow(10, -30.0/20))
	}
	if got := FindPauses(bed, pauseFPS); len(got) != 0 {
		t.Fatalf("pauses over a music bed = %+v", got)
	}
	// Constant sound has none; nothing gives nothing.
	flat := make([]float64, 250)
	for i := range flat {
		flat[i] = 0.1
	}
	if got := FindPauses(flat, pauseFPS); len(got) != 0 {
		t.Fatalf("constant sound pauses = %+v", got)
	}
	if got := FindPauses(nil, pauseFPS); got == nil || len(got) != 0 {
		t.Fatalf("no sound = %#v", got)
	}
	// Stored compactly.
	b, _ := json.Marshal([]Pause{{1, 2}})
	var back []Pause
	if string(b) != "[[1,2]]" || json.Unmarshal(b, &back) != nil || back[0] != (Pause{1, 2}) {
		t.Fatalf("json = %s, %+v", b, back)
	}
}
