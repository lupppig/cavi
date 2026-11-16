package ai

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// PlansPromptVersion identifies the edit-plan prompt and schema.
//
// v2 centres plans on a person the brief says must be featured, and asks
// for a voiceover in every plan: about that person by name, in a curious,
// playful narrator's voice that sets up what the video is about.
//
// v3 plans for retention: a cold open on the most gripping line, setup
// cut to the minimum, ending on the payoff; hooks that open a curiosity
// gap; titles written to be clicked; a voiceover only when the footage
// needs one; and up to 10 plans, for making a batch.
const PlansPromptVersion = "plans-v3"

// Edit plan limits.
const (
	MaxPlans          = 10
	MaxPlanSegments   = 8
	MaxPlanFiles      = 5
	MaxPlanHookRunes  = 80
	MaxPlanVoiceRunes = 400
	planLengthSlackMS = 10_000
)

// PlanFile is one transcribed file offered to the planner.
type PlanFile struct {
	ID       string
	Name     string
	Segments []Segment
}

// PlanBrief is what the planner is asked for.
type PlanBrief struct {
	Objective    string
	MinMS, MaxMS int64
	Campaign     *Campaign
	// Rules on each file (by ID), to keep plans off excluded moments.
	Rules map[string][]BriefRule
}

// PlanSegment is one stretch of one file: whole transcript lines.
type PlanSegment struct {
	File         int // index into the files offered
	FirstSegment int
	LastSegment  int
	StartMS      int64
	EndMS        int64
}

// Plan is a proposed montage.
type Plan struct {
	Title           string
	Hook            string
	VoiceoverScript string
	Reason          string
	CampaignLink    string
	Segments        []PlanSegment
}

// LengthMS is the plan's total length.
func (p Plan) LengthMS() int64 {
	var n int64
	for _, s := range p.Segments {
		n += s.EndMS - s.StartMS
	}
	return n
}

// Planner proposes edit plans from transcripts and a brief.
type Planner interface {
	Plans(ctx context.Context, model Model, files []PlanFile, brief PlanBrief) ([]Plan, Usage, error)
}

// plansInput renders the files' transcripts as numbered lines.
func plansInput(files []PlanFile) (string, error) {
	var b strings.Builder
	for f, file := range files {
		fmt.Fprintf(&b, "File F%d (%s):\n", f+1, strings.NewReplacer("\n", " ", "<<<", "", ">>>", "").Replace(file.Name))
		for i, s := range file.Segments {
			fmt.Fprintf(&b, "[F%d:%d] %s-%s %s: %s\n", f+1, i, clock(s.StartMS), clock(s.EndMS), s.Speaker, s.Text)
		}
		b.WriteString("\n")
	}
	if n := utf8.RuneCountInString(b.String()); n > maxHighlightInputRunes {
		return "", fmt.Errorf("%w: transcripts are too long to plan from (%d characters)", ErrPermanent, n)
	}
	return b.String(), nil
}

// ValidatePlans checks model output against the files and the brief.
// References to files or lines that don't exist mean the model invented
// them (ErrInvalidOutput). Plans that are well-formed but unusable
// (excluded moments, wrong length, no campaign link when there's a brief)
// are dropped. Times always come from the transcripts.
func ValidatePlans(ps []Plan, files []PlanFile, brief PlanBrief) ([]Plan, error) {
	bad := func(format string, args ...any) ([]Plan, error) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
	}
	if len(ps) > 3*MaxPlans {
		return bad("%d plans", len(ps))
	}
	out := []Plan{}
plans:
	for i, p := range ps {
		p.Title = strings.TrimSpace(p.Title)
		p.Hook = strings.Join(strings.Fields(p.Hook), " ")
		p.VoiceoverScript = strings.Join(strings.Fields(p.VoiceoverScript), " ")
		p.Reason = strings.TrimSpace(p.Reason)
		p.CampaignLink = strings.TrimSpace(p.CampaignLink)
		switch {
		case p.Title == "" || p.Reason == "" || len(p.Segments) == 0:
			return bad("plan %d is missing a title, reason or segments", i)
		case utf8.RuneCountInString(p.Title) > maxTitleRunes || utf8.RuneCountInString(p.Reason) > maxReasonRunes ||
			utf8.RuneCountInString(p.CampaignLink) > maxCampaignLinkRunes:
			return bad("plan %d text is too long", i)
		case len(p.Segments) > MaxPlanSegments:
			return bad("plan %d has %d segments", i, len(p.Segments))
		}
		if utf8.RuneCountInString(p.Hook) > MaxPlanHookRunes {
			continue
		}
		if utf8.RuneCountInString(p.VoiceoverScript) > MaxPlanVoiceRunes {
			p.VoiceoverScript = ""
		}
		for j := range p.Segments {
			s := &p.Segments[j]
			if s.File < 0 || s.File >= len(files) {
				return bad("plan %d segment %d is in file %d of %d", i, j, s.File+1, len(files))
			}
			segs := files[s.File].Segments
			if s.FirstSegment < 0 || s.LastSegment >= len(segs) || s.FirstSegment > s.LastSegment {
				return bad("plan %d segment %d covers lines %d-%d of %d", i, j, s.FirstSegment, s.LastSegment, len(segs))
			}
			s.StartMS, s.EndMS = segs[s.FirstSegment].StartMS, segs[s.LastSegment].EndMS
			for _, l := range segs[s.FirstSegment : s.LastSegment+1] {
				s.EndMS = max(s.EndMS, l.EndMS)
			}
			if excludes(brief.Rules[files[s.File].ID], s.StartMS, s.EndMS) {
				continue plans
			}
		}
		if n := p.LengthMS(); n < brief.MinMS-planLengthSlackMS || n > brief.MaxMS+planLengthSlackMS {
			continue
		}
		if brief.Campaign != nil && p.CampaignLink == "" {
			continue
		}
		if brief.Campaign == nil {
			p.CampaignLink = ""
		}
		out = append(out, p)
		if len(out) == MaxPlans {
			break
		}
	}
	return out, nil
}
