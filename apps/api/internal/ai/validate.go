package ai

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Limits on transcript output.
const (
	maxSegments      = 5000
	maxSegmentRunes  = 4000
	maxSpeakerRunes  = 40
	timestampSlackMS = 2000
)

var languagePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// ValidateTranscript checks model output against what's possible for audio
// of durationMS and normalises it: segments sorted, text trimmed, empty
// segments dropped, ends clamped to the audio. Anything implausible is
// ErrInvalidOutput rather than being stored.
func ValidateTranscript(t Transcript, durationMS int64) (Transcript, error) {
	bad := func(format string, args ...any) (Transcript, error) {
		return Transcript{}, fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
	}
	t.Language = strings.TrimSpace(t.Language)
	if t.Language != "" && !languagePattern.MatchString(t.Language) {
		return bad("language %q is not a language code", t.Language)
	}
	if len(t.Segments) > maxSegments {
		return bad("%d segments", len(t.Segments))
	}
	out := make([]Segment, 0, len(t.Segments))
	for i, s := range t.Segments {
		s.Text = strings.TrimSpace(s.Text)
		s.Speaker = strings.TrimSpace(s.Speaker)
		if s.Text == "" {
			continue
		}
		if len([]rune(s.Text)) > maxSegmentRunes {
			return bad("segment %d is too long", i)
		}
		if s.Speaker == "" {
			s.Speaker = "Speaker 1"
		}
		if len([]rune(s.Speaker)) > maxSpeakerRunes {
			return bad("segment %d speaker label is too long", i)
		}
		if s.StartMS < 0 || s.EndMS <= s.StartMS {
			return bad("segment %d has times %d-%d", i, s.StartMS, s.EndMS)
		}
		if durationMS > 0 {
			if s.StartMS >= durationMS {
				return bad("segment %d starts at %dms, after the audio ends (%dms)", i, s.StartMS, durationMS)
			}
			if s.EndMS > durationMS+timestampSlackMS {
				return bad("segment %d ends at %dms, after the audio ends (%dms)", i, s.EndMS, durationMS)
			}
			s.EndMS = min(s.EndMS, durationMS)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartMS < out[j].StartMS })
	t.Segments = out
	return t, nil
}
