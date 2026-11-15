package ai

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

const testBrief = `Key: 01:30 – how it began; 0:47 - 0:53 must not be used.
Plates: 03:13, 1:02:15 – blur.`

func TestBriefTimes(t *testing.T) {
	got := BriefTimes(testBrief)
	want := []int64{90_000, 47_000, 53_000, 193_000, 3_735_000}
	if len(got) != len(want) {
		t.Fatalf("BriefTimes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BriefTimes = %v, want %v", got, want)
		}
	}
}

func TestValidateBrief(t *testing.T) {
	const hour = int64(time.Hour / time.Millisecond)
	ok := Brief{
		CampaignMessage: " Diamonds fund schools. ",
		Rules: []BriefRule{
			{Kind: RuleBlur, StartMS: 193_000, EndMS: 193_000, Note: "Licence plate"},
			{Kind: RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"},
			{Kind: RulePriority, StartMS: 90_400, EndMS: 90_400, Note: "How it began"},
			{Kind: RuleBlur, StartMS: 193_000, EndMS: 193_000, Note: "duplicate"},
		},
	}
	b, err := ValidateBrief(ok, testBrief, 2*hour)
	if err != nil {
		t.Fatal(err)
	}
	if b.CampaignMessage != "Diamonds fund schools." || len(b.Rules) != 3 || b.Rules[0].Kind != RuleExclude || b.Rules[2].Note != "Licence plate" {
		t.Fatalf("ValidateBrief = %+v", b)
	}

	for name, r := range map[string]BriefRule{
		"invented time":   {Kind: RulePriority, StartMS: 600_000, EndMS: 600_000, Note: "x"},
		"invented end":    {Kind: RuleExclude, StartMS: 47_000, EndMS: 70_000, Note: "x"},
		"unknown kind":    {Kind: "delete", StartMS: 90_000, EndMS: 90_000, Note: "x"},
		"no note":         {Kind: RulePriority, StartMS: 90_000, EndMS: 90_000, Note: " "},
		"backwards":       {Kind: RuleExclude, StartMS: 53_000, EndMS: 47_000, Note: "x"},
		"after the video": {Kind: RuleBlur, StartMS: 3_735_000, EndMS: 3_735_000, Note: "x"},
	} {
		_, err := ValidateBrief(Brief{Rules: []BriefRule{r}}, testBrief, hour)
		if !errors.Is(err, ErrInvalidOutput) {
			t.Errorf("%s: err = %v, want ErrInvalidOutput", name, err)
		}
	}
}

func TestBriefRuleOverlaps(t *testing.T) {
	point := BriefRule{StartMS: 10_000, EndMS: 10_000}
	span := BriefRule{StartMS: 10_000, EndMS: 20_000}
	cases := []struct {
		r          BriefRule
		start, end int64
		want       bool
	}{
		{point, 0, 10_000, false}, // clip ends as the moment starts
		{point, 0, 10_001, true},
		{point, 10_000, 15_000, true},
		{span, 0, 10_000, false},
		{span, 19_999, 30_000, true},
		{span, 20_000, 30_000, false},
		{span, 12_000, 13_000, true},
	}
	for _, c := range cases {
		if got := c.r.Overlaps(c.start, c.end); got != c.want {
			t.Errorf("%+v.Overlaps(%d, %d) = %v, want %v", c.r, c.start, c.end, got, c.want)
		}
	}
}

// TestEvalBriefs reads a synthetic brief shaped like real creator briefs
// (ranges, several timestamps per line, every rule kind) with each
// catalogued model.
func TestEvalBriefs(t *testing.T) {
	g := evalProvider(t)
	text, err := os.ReadFile("testdata/brief-festival.txt")
	if err != nil {
		t.Fatal(err)
	}
	const duration = 65 * 60 * 1000
	type want struct {
		kind       string
		start, end int64
	}
	s := func(m, sec int64) int64 { return (m*60 + sec) * 1000 }
	wants := []want{
		{RuleExclude, s(0, 31), s(0, 40)},
		{RulePriority, s(2, 10), s(2, 10)},
		{RulePriority, s(5, 45), s(5, 45)},
		{RuleBlur, s(7, 12), s(7, 12)},
		{RuleCaution, s(9, 50), s(9, 50)},
		{RuleMute, s(14, 2), s(14, 2)},
		{RuleBlur, s(18, 5), s(18, 5)},
		{RulePriority, s(21, 30), s(21, 30)},
		{RuleCaution, s(26, 40), s(26, 40)},
		{RuleCaution, s(33, 20), s(33, 20)},
		{RuleReview, s(40, 0), s(40, 0)},
		{RuleExclude, s(44, 44), s(44, 44)},
		{RulePriority, s(62, 15), s(62, 15)},
	}
	for _, m := range Models(TaskBriefs) {
		t.Run(m.ID, func(t *testing.T) {
			b, usage := readBriefWithBackoff(t, g, m, string(text), duration)
			t.Logf("usage %+v; campaign %q; guidance %q", usage, b.CampaignMessage, b.Guidance)
			got := map[want]bool{}
			for _, r := range b.Rules {
				t.Logf("%-8s %s-%s %s", r.Kind, clock(r.StartMS), clock(r.EndMS), r.Note)
				got[want{r.Kind, r.StartMS, r.EndMS}] = true
			}
			for _, w := range wants {
				if !got[w] {
					t.Errorf("missing %s %s-%s", w.kind, clock(w.start), clock(w.end))
				}
			}
			if len(b.Rules) != len(wants) {
				t.Errorf("%d rules, want %d", len(b.Rules), len(wants))
			}
			if b.CampaignMessage == "" || b.Guidance == "" {
				t.Error("campaign message or guidance is empty")
			}
		})
	}
}

func readBriefWithBackoff(t *testing.T, g *Gemini, m Model, text string, duration int64) (Brief, Usage) {
	t.Helper()
	var err error
	for _, wait := range []time.Duration{0, 20 * time.Second, 60 * time.Second, 120 * time.Second} {
		time.Sleep(wait)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		var b Brief
		var u Usage
		b, u, err = g.ReadBrief(ctx, m, text, duration)
		cancel()
		if err == nil {
			return b, u
		}
		if errors.Is(err, ErrPermanent) {
			break
		}
		t.Logf("retrying after: %v", err)
	}
	t.Fatalf("ReadBrief: %v", err)
	return Brief{}, Usage{}
}
