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
	minSegmentMS     = 300
	// wordSlackMS is how far outside its segment a word may start before
	// the segment's word times are distrusted (within it, words are
	// clamped to the segment).
	wordSlackMS = 500
	// maxEstimatedShare is the share of segments that may lack usable word
	// times before the whole output is treated as invalid.
	maxEstimatedShare  = 0.2
	maxWordsPerSegment = 400
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
		// Equal times are a rounding artefact on very short speech; give
		// the segment a minimal length. Ends before starts are invalid.
		if s.EndMS == s.StartMS {
			s.EndMS = s.StartMS + minSegmentMS
		}
		if s.StartMS < 0 || s.EndMS < s.StartMS {
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
		s.Words = validWords(s)
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartMS < out[j].StartMS })
	estimated := 0
	for _, s := range out {
		if len(s.Words) == 0 {
			estimated++
		}
	}
	if len(out) > 0 && float64(estimated) > maxEstimatedShare*float64(len(out)) {
		return bad("%d of %d segments have no usable word times", estimated, len(out))
	}
	t.Segments = out
	t.Speakers = speakers(out, t.Speakers)
	return t, nil
}

// validWords returns the segment's words with checked times (each ends
// where the next starts; the last at the segment's end), or nil if the
// model's word times don't hold up.
func validWords(s Segment) []Word {
	if len(s.Words) == 0 || len(s.Words) > maxWordsPerSegment {
		return nil
	}
	out := make([]Word, 0, len(s.Words))
	prev := s.StartMS
	for _, w := range s.Words {
		w.Text = strings.TrimSpace(w.Text)
		if w.Text == "" {
			continue
		}
		if w.StartMS < s.StartMS-wordSlackMS || w.StartMS > s.EndMS+wordSlackMS || w.StartMS < prev-wordSlackMS/5 {
			return nil
		}
		w.StartMS = min(max(w.StartMS, s.StartMS, prev), s.EndMS)
		prev = w.StartMS
		out = append(out, w)
	}
	for i := range out {
		if i+1 < len(out) {
			out[i].EndMS = out[i+1].StartMS
		} else {
			out[i].EndMS = s.EndMS
		}
	}
	return out
}

// speakers lists each label used in segments once, in order of first
// appearance, with the voice the model gave it (unclear if none or not a
// known value).
func speakers(segs []Segment, given []Speaker) []Speaker {
	voice := map[string]string{}
	for _, sp := range given {
		if v := strings.TrimSpace(sp.Voice); ValidVoice(v) {
			voice[strings.TrimSpace(sp.Label)] = v
		}
	}
	var out []Speaker
	seen := map[string]bool{}
	for _, s := range segs {
		if seen[s.Speaker] {
			continue
		}
		seen[s.Speaker] = true
		v := voice[s.Speaker]
		if v == "" {
			v = VoiceUnclear
		}
		out = append(out, Speaker{Label: s.Speaker, Voice: v})
	}
	return out
}
