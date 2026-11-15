package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// HighlightsPromptVersion identifies the highlights prompt and schema.
//
// v2 adds the creator's brief: campaign message, guidance and timed rules,
// and asks how each clip connects to the campaign.
const HighlightsPromptVersion = "highlights-v2"

// Limits on highlight detection.
const (
	// MaxHighlights is the most candidate clips kept from one run.
	MaxHighlights = 10
	// MaxBriefRunes bounds the user's creative objective.
	MaxBriefRunes = 500
	// maxHighlightInputRunes bounds the transcript text sent per call.
	maxHighlightInputRunes = 1_500_000
	maxTitleRunes          = 120
	maxReasonRunes         = 600
	maxCampaignLinkRunes   = 400
	// lengthSlackMS lets a clip run a little outside the requested length,
	// since clips are cut on whole transcript lines.
	lengthSlackMS = 5000
)

// HighlightBrief is what the user asked for.
type HighlightBrief struct {
	// Objective is the user's creative goal in their words; may be empty.
	Objective string
	// MinMS and MaxMS bound each clip's length.
	MinMS, MaxMS int64
	// Campaign is the creator's brief for this video, if it has one.
	Campaign *Campaign
}

// Campaign is what highlight detection needs from a creator brief.
type Campaign struct {
	Message  string      `json:"message"`
	Guidance string      `json:"guidance"`
	Rules    []BriefRule `json:"rules"`
}

// Highlight is a candidate clip: a contiguous run of transcript segments.
// Times come from the segments, never from the model, so clips start and
// end on whole lines of speech.
type Highlight struct {
	FirstSegment int
	LastSegment  int
	StartMS      int64
	EndMS        int64
	Title        string
	Reason       string
	// CampaignLink says how the clip connects to the campaign message;
	// empty when there is no brief.
	CampaignLink string
}

// Highlighter finds candidate clips in a transcript.
type Highlighter interface {
	Highlights(ctx context.Context, model Model, segments []Segment, brief HighlightBrief) ([]Highlight, Usage, error)
}

// highlightsInput renders the transcript as numbered lines for the model.
func highlightsInput(segments []Segment) (string, error) {
	var b strings.Builder
	for i, s := range segments {
		fmt.Fprintf(&b, "[%d] %s-%s %s: %s\n", i, clock(s.StartMS), clock(s.EndMS), s.Speaker, s.Text)
	}
	if n := len([]rune(b.String())); n > maxHighlightInputRunes {
		return "", fmt.Errorf("%w: transcript is too long for highlight detection (%d characters)", ErrPermanent, n)
	}
	return b.String(), nil
}

func clock(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

// ValidateHighlights checks model output against the transcript and the
// brief. Line numbers that don't exist mean the model invented them
// (ErrInvalidOutput). Clips that are well-formed but unsuitable (outside
// the requested length, mostly overlapping a better-ranked clip) are
// dropped. Output keeps the model's ranking, best first.
func ValidateHighlights(hs []Highlight, segments []Segment, brief HighlightBrief) ([]Highlight, error) {
	bad := func(format string, args ...any) ([]Highlight, error) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
	}
	if len(hs) > 3*MaxHighlights {
		return bad("%d highlights", len(hs))
	}
	out := make([]Highlight, 0, min(len(hs), MaxHighlights))
	for i, h := range hs {
		if h.FirstSegment < 0 || h.LastSegment >= len(segments) || h.FirstSegment > h.LastSegment {
			return bad("highlight %d covers lines %d-%d of %d", i, h.FirstSegment, h.LastSegment, len(segments))
		}
		h.Title = strings.TrimSpace(h.Title)
		h.Reason = strings.TrimSpace(h.Reason)
		h.CampaignLink = strings.TrimSpace(h.CampaignLink)
		if len([]rune(h.CampaignLink)) > maxCampaignLinkRunes {
			return bad("highlight %d campaign link is too long", i)
		}
		if h.Title == "" || h.Reason == "" {
			return bad("highlight %d has no title or reason", i)
		}
		if len([]rune(h.Title)) > maxTitleRunes || len([]rune(h.Reason)) > maxReasonRunes {
			return bad("highlight %d text is too long", i)
		}
		h.StartMS = segments[h.FirstSegment].StartMS
		h.EndMS = segments[h.LastSegment].EndMS
		for _, s := range segments[h.FirstSegment : h.LastSegment+1] {
			h.EndMS = max(h.EndMS, s.EndMS)
		}
		if d := h.EndMS - h.StartMS; d < brief.MinMS-lengthSlackMS || d > brief.MaxMS+lengthSlackMS {
			continue
		}
		if overlapsMost(h, out) {
			continue
		}
		if brief.Campaign != nil {
			// Every clip must connect back to the campaign, and none may
			// include a moment the brief excludes.
			if h.CampaignLink == "" || excludes(brief.Campaign.Rules, h.StartMS, h.EndMS) {
				continue
			}
		} else {
			h.CampaignLink = ""
		}
		out = append(out, h)
		if len(out) == MaxHighlights {
			break
		}
	}
	if brief.Campaign != nil {
		// Clips that feature a moment the creator prioritised come first,
		// keeping the model's order otherwise.
		sort.SliceStable(out, func(i, j int) bool {
			return featuresPriority(brief.Campaign.Rules, out[i]) && !featuresPriority(brief.Campaign.Rules, out[j])
		})
	}
	return out, nil
}

func excludes(rules []BriefRule, startMS, endMS int64) bool {
	for _, r := range rules {
		if r.Kind == RuleExclude && r.Overlaps(startMS, endMS) {
			return true
		}
	}
	return false
}

func featuresPriority(rules []BriefRule, h Highlight) bool {
	for _, r := range rules {
		if r.Kind == RulePriority && r.Overlaps(h.StartMS, h.EndMS+1) {
			return true
		}
	}
	return false
}

// overlapsMost reports whether more than half of h is covered by one clip
// already kept, or h covers more than half of one.
func overlapsMost(h Highlight, kept []Highlight) bool {
	for _, k := range kept {
		o := min(h.EndMS, k.EndMS) - max(h.StartMS, k.StartMS)
		if o <= 0 {
			continue
		}
		if 2*o > h.EndMS-h.StartMS || 2*o > k.EndMS-k.StartMS {
			return true
		}
	}
	return false
}
