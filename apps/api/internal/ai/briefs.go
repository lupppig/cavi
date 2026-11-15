package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// BriefsPromptVersion identifies the brief-reading prompt and schema.
const BriefsPromptVersion = "briefs-v1"

// Limits on creator briefs.
const (
	// MaxBriefTextRunes bounds a pasted brief.
	MaxBriefTextRunes = 20_000
	// MaxBriefRules is the most timed rules one brief may hold.
	MaxBriefRules = 200
	// MaxRuleNoteRunes bounds a rule's note.
	MaxRuleNoteRunes = 300
	// MaxCampaignRunes bounds the campaign message and the guidance.
	MaxCampaignRunes = 2_000
	// briefTimeSlackMS is how far a rule's time may sit from a time
	// written in the brief: briefs give whole seconds.
	briefTimeSlackMS = 1000
)

// Kinds of brief rule. Exclude is a hard constraint enforced in code; the
// others are flags people see when a clip touches them.
const (
	RulePriority = "priority" // a moment the creator wants clipped
	RuleExclude  = "exclude"  // must never appear in any clip
	RuleBlur     = "blur"     // something on screen must be blurred or cropped out
	RuleMute     = "mute"     // audio that must not be used (e.g. uncleared music)
	RuleCaution  = "caution"  // sensitive topic or third-party branding: avoid featuring it
	RuleReview   = "review"   // a person must check before it is used
)

// RuleKinds lists every rule kind, in display order.
var RuleKinds = []string{RulePriority, RuleExclude, RuleBlur, RuleMute, RuleCaution, RuleReview}

// ValidRuleKind reports whether k is a rule kind.
func ValidRuleKind(k string) bool {
	for _, x := range RuleKinds {
		if x == k {
			return true
		}
	}
	return false
}

// BriefRule is one timed instruction from a brief. A single timestamp in
// the brief has StartMS == EndMS.
type BriefRule struct {
	Kind    string
	StartMS int64
	EndMS   int64
	Note    string
}

// Overlaps reports whether the rule touches the clip [startMS, endMS).
func (r BriefRule) Overlaps(startMS, endMS int64) bool {
	if r.StartMS == r.EndMS {
		return r.StartMS >= startMS && r.StartMS < endMS
	}
	return r.StartMS < endMS && startMS < r.EndMS
}

// Brief is a creator brief read into structure.
type Brief struct {
	// CampaignMessage is what every clip should connect back to.
	CampaignMessage string
	// Guidance is the creative direction that has no timestamp: tone,
	// freedom to combine footage, what to avoid in general.
	Guidance string
	Rules    []BriefRule
}

// BriefReader reads a pasted creator brief.
type BriefReader interface {
	// ReadBrief structures text, whose timestamps refer to a video of
	// durationMS.
	ReadBrief(ctx context.Context, model Model, text string, durationMS int64) (Brief, Usage, error)
}

// briefTimePattern finds times written in a brief: [H:]M:SS or [H:]MM:SS.
var briefTimePattern = regexp.MustCompile(`\b(?:\d{1,2}:)?\d{1,3}:[0-5]\d\b`)

// BriefTimes returns every time written in text, in milliseconds.
func BriefTimes(text string) []int64 {
	var out []int64
	for _, m := range briefTimePattern.FindAllString(text, -1) {
		if ms, ok := parseClock(m); ok {
			out = append(out, ms)
		}
	}
	return out
}

func near(times []int64, ms int64) bool {
	for _, t := range times {
		if d := t - ms; d >= -briefTimeSlackMS && d <= briefTimeSlackMS {
			return true
		}
	}
	return false
}

// ValidateBrief checks model output against the brief's text and the
// video. Every time must be one written in the brief, so the model can't
// invent moments, and must lie within the video. Output is sorted by time.
func ValidateBrief(b Brief, text string, durationMS int64) (Brief, error) {
	bad := func(format string, args ...any) (Brief, error) {
		return Brief{}, fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
	}
	b.CampaignMessage = strings.TrimSpace(b.CampaignMessage)
	b.Guidance = strings.TrimSpace(b.Guidance)
	if utf8.RuneCountInString(b.CampaignMessage) > MaxCampaignRunes || utf8.RuneCountInString(b.Guidance) > MaxCampaignRunes {
		return bad("campaign message or guidance is too long")
	}
	if len(b.Rules) > MaxBriefRules {
		return bad("%d rules", len(b.Rules))
	}
	written := BriefTimes(text)
	seen := map[BriefRule]bool{}
	rules := make([]BriefRule, 0, len(b.Rules))
	for i, r := range b.Rules {
		r.Note = strings.TrimSpace(r.Note)
		if !ValidRuleKind(r.Kind) {
			return bad("rule %d has kind %q", i, r.Kind)
		}
		if r.Note == "" || utf8.RuneCountInString(r.Note) > MaxRuleNoteRunes {
			return bad("rule %d note is empty or too long", i)
		}
		if r.EndMS < r.StartMS {
			return bad("rule %d ends before it starts", i)
		}
		if !near(written, r.StartMS) || !near(written, r.EndMS) {
			return bad("rule %d time %s-%s is not in the brief", i, clock(r.StartMS), clock(r.EndMS))
		}
		if r.StartMS >= durationMS {
			return bad("rule %d at %s is after the video ends (%s)", i, clock(r.StartMS), clock(durationMS))
		}
		r.EndMS = min(r.EndMS, durationMS)
		key := r
		key.Note = ""
		if seen[key] {
			continue // same kind and time twice: keep the first note
		}
		seen[key] = true
		rules = append(rules, r)
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].StartMS < rules[j].StartMS })
	b.Rules = rules
	return b, nil
}
