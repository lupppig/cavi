package ai

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Evaluation against the real Gemini API. It costs money, so it only runs
// when CAVI_AI_EVAL=1 and GEMINI_API_KEY are set (make ai-eval). Every
// model in the catalogue must pass before it is offered to organisations.
//
// Fixtures (synthetic speech made with espeak-ng):
//   - two-speakers.mp3: voice A 0.0–7.0s, silence, voice B 8.5–15.9s.
//   - tone-no-speech.mp3: a 4s tone with no speech.

const (
	speakerA = "Welcome back to the studio. Today we are talking about how independent artists plan a release campaign."
	speakerB = "Thanks for having me. The first thing I tell every artist is to start three months before the single comes out."
)

func evalProvider(t *testing.T) *Gemini {
	t.Helper()
	key := os.Getenv("GEMINI_API_KEY")
	if os.Getenv("CAVI_AI_EVAL") != "1" || key == "" {
		t.Skip("set CAVI_AI_EVAL=1 and GEMINI_API_KEY to run the AI evaluation")
	}
	g, err := NewGemini(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

var nonWord = regexp.MustCompile(`[^a-z0-9 ]+`)

func words(s string) []string {
	return strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), ""))
}

// wer is the word error rate of got against want.
func wer(want, got string) float64 {
	w, g := words(want), words(got)
	d := make([][]int, len(w)+1)
	for i := range d {
		d[i] = make([]int, len(g)+1)
		d[i][0] = i
	}
	for j := range g {
		d[0][j+1] = j + 1
	}
	for i := 1; i <= len(w); i++ {
		for j := 1; j <= len(g); j++ {
			cost := 1
			if w[i-1] == g[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return float64(d[len(w)][len(g)]) / float64(len(w))
}

// transcribe retries briefly on transient errors (busy models), as the
// job system would.
func transcribe(t *testing.T, g *Gemini, m Model, file string, durationMS int64) Transcript {
	t.Helper()
	audio, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		tr, usage, err := g.Transcribe(ctx, m, audio, "audio/mpeg", durationMS)
		cancel()
		if err == nil {
			t.Logf("%s on %s: %d segments, %d input / %d output tokens", m.ID, file, len(tr.Segments), usage.InputTokens, usage.OutputTokens)
			return tr
		}
		last = err
		if strings.Contains(err.Error(), "temporarily unavailable") {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
			continue
		}
		break
	}
	t.Fatalf("%s: %v", m.ID, last)
	return Transcript{}
}

func TestEvalTranscription(t *testing.T) {
	g := evalProvider(t)
	for _, m := range Models(TaskTranscription) {
		t.Run(m.ID, func(t *testing.T) {
			tr := transcribe(t, g, m, "two-speakers.mp3", 15890)
			if !strings.HasPrefix(strings.ToLower(tr.Language), "en") {
				t.Errorf("language = %q", tr.Language)
			}
			var all []string
			speakers := map[string]string{}
			for _, s := range tr.Segments {
				all = append(all, s.Text)
				if _, ok := speakers[s.Speaker]; !ok {
					speakers[s.Speaker] = s.Text
				}
				t.Logf("  %6d-%6d %s: %s", s.StartMS, s.EndMS, s.Speaker, s.Text)
			}
			if e := wer(speakerA+" "+speakerB, strings.Join(all, " ")); e > 0.1 {
				t.Errorf("word error rate %.2f", e)
			}
			if len(speakers) != 2 {
				t.Errorf("%d speakers, want 2", len(speakers))
			}
			// Speaker B starts at 8.5s; allow the model a second either way.
			for _, s := range tr.Segments {
				if strings.Contains(strings.ToLower(s.Text), "thanks for having me") {
					if s.StartMS < 7000 || s.StartMS > 9500 {
						t.Errorf("speaker B starts at %dms, want about 8500", s.StartMS)
					}
				}
			}
		})
		t.Run(m.ID+"/no-speech", func(t *testing.T) {
			tr := transcribe(t, g, m, "tone-no-speech.mp3", 4000)
			if len(tr.Segments) != 0 {
				t.Errorf("invented %d segments from a tone: %+v", len(tr.Segments), tr.Segments)
			}
		})
	}
}
