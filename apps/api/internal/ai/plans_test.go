package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func planFiles() []PlanFile {
	mk := func(n int) []Segment {
		s := make([]Segment, n)
		for i := range s {
			s[i] = Segment{StartMS: int64(i) * 10_000, EndMS: int64(i+1) * 10_000, Speaker: "Speaker 1", Text: "x"}
		}
		return s
	}
	return []PlanFile{{ID: "a", Name: "talk.mp4", Segments: mk(20)}, {ID: "b", Name: "safari.mp4", Segments: mk(5)}}
}

func TestValidatePlans(t *testing.T) {
	files := planFiles()
	brief := PlanBrief{MinMS: 30_000, MaxMS: 60_000, Campaign: &Campaign{Message: "m"},
		Rules: map[string][]BriefRule{"a": {{Kind: RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "no"}}}}
	seg := func(f, a, b int) PlanSegment { return PlanSegment{File: f, FirstSegment: a, LastSegment: b} }
	ps, err := ValidatePlans([]Plan{
		{Title: "good", Reason: "r", CampaignLink: "l", Hook: " Diamonds  built this ", VoiceoverScript: "Narration.",
			Segments: []PlanSegment{seg(0, 10, 12), seg(1, 0, 1)}}, // 30 s + 20 s
		{Title: "excluded", Reason: "r", CampaignLink: "l", Segments: []PlanSegment{seg(0, 3, 7)}},
		{Title: "too short", Reason: "r", CampaignLink: "l", Segments: []PlanSegment{seg(1, 0, 0)}},
		{Title: "no link", Reason: "r", Segments: []PlanSegment{seg(0, 10, 13)}},
		{Title: "long hook", Reason: "r", CampaignLink: "l", Hook: strings.Repeat("x", 81), Segments: []PlanSegment{seg(0, 10, 13)}},
	}, files, brief)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Title != "good" || ps[0].Hook != "Diamonds built this" || ps[0].LengthMS() != 50_000 ||
		ps[0].Segments[1].StartMS != 0 || ps[0].Segments[1].EndMS != 20_000 {
		t.Fatalf("plans = %+v", ps)
	}
	for name, p := range map[string]Plan{
		"unknown file": {Title: "t", Reason: "r", Segments: []PlanSegment{seg(2, 0, 1)}},
		"unknown line": {Title: "t", Reason: "r", Segments: []PlanSegment{seg(1, 3, 9)}},
		"no segments":  {Title: "t", Reason: "r"},
	} {
		if _, err := ValidatePlans([]Plan{p}, files, brief); !errors.Is(err, ErrInvalidOutput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// TestEvalPlans asks each model for montages of the podcast fixture under
// a campaign brief: plans must be valid, stay off the excluded story, and
// feature the prioritised advice.
func TestEvalPlans(t *testing.T) {
	g := evalProvider(t)
	segs := podcast(t)
	files := []PlanFile{{ID: "pod", Name: "podcast.mp3", Segments: segs}}
	brief := PlanBrief{MinMS: 20_000, MaxMS: 75_000,
		Campaign: &Campaign{Message: "Help independent musicians plan their releases.", Guidance: "Practical, encouraging advice."},
		Rules: map[string][]BriefRule{"pod": {
			{Kind: RulePriority, StartMS: segs[20].StartMS, EndMS: segs[20].StartMS, Note: "Release planning advice"},
			{Kind: RuleExclude, StartMS: segs[7].StartMS, EndMS: segs[12].EndMS, Note: "Power-cut story: not for this campaign"},
		}}}
	for _, m := range Models(TaskPlans) {
		t.Run(m.ID, func(t *testing.T) {
			var ps []Plan
			var err error
			for _, wait := range []time.Duration{0, 20 * time.Second, 60 * time.Second, 120 * time.Second} {
				time.Sleep(wait)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				ps, _, err = g.Plans(ctx, m, files, brief)
				cancel()
				if err == nil || errors.Is(err, ErrPermanent) {
					break
				}
				t.Logf("retrying after: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(ps) == 0 {
				t.Fatal("no usable plans")
			}
			featured := false
			for _, p := range ps {
				t.Logf("%q hook %q (%d s, %d segments) vo %q: %s", p.Title, p.Hook, p.LengthMS()/1000, len(p.Segments), p.VoiceoverScript, p.CampaignLink)
				for _, s := range p.Segments {
					if s.FirstSegment <= 20 && s.LastSegment >= 20 {
						featured = true
					}
				}
				if p.Hook == "" {
					t.Errorf("plan %q has no hook", p.Title)
				}
			}
			if !featured {
				t.Error("no plan features the prioritised advice (line 20)")
			}
		})
	}
}

// TestEvalPlansFeaturedPerson checks a brief that requires someone to be
// the main focus: plans centre on them and the narration names them.
func TestEvalPlansFeaturedPerson(t *testing.T) {
	g := evalProvider(t)
	segs := podcast(t)
	files := []PlanFile{{ID: "pod", Name: "podcast.mp3", Segments: segs}}
	brief := PlanBrief{MinMS: 20_000, MaxMS: 75_000,
		Campaign: &Campaign{Message: "Promote Maya Okafor's first headline tour.", Guidance: "Submitted videos must feature Maya Okafor as the primary focus."}}
	for _, m := range Models(TaskPlans) {
		t.Run(m.ID, func(t *testing.T) {
			var ps []Plan
			var err error
			for _, wait := range []time.Duration{0, 20 * time.Second, 60 * time.Second} {
				time.Sleep(wait)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				ps, _, err = g.Plans(ctx, m, files, brief)
				cancel()
				if err == nil || errors.Is(err, ErrPermanent) {
					break
				}
				t.Logf("retrying after: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(ps) == 0 {
				t.Fatal("no usable plans")
			}
			for _, p := range ps {
				t.Logf("%q hook %q vo %q", p.Title, p.Hook, p.VoiceoverScript)
				if !strings.Contains(p.VoiceoverScript, "Maya") {
					t.Errorf("narration doesn't name Maya: %q", p.VoiceoverScript)
				}
				if n := len(strings.Fields(p.VoiceoverScript)); n < 8 || n > 40 {
					t.Errorf("narration is %d words", n)
				}
			}
		})
	}
}
