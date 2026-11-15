package ai

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"
)

// VoiceoverPromptVersion identifies how scripts are spoken.
const VoiceoverPromptVersion = "voiceover-v1"

// Voiceover limits.
const (
	MaxScriptRunes = 1200
	// Spoken pace bounds, in words per second, for checking output.
	// Natural narration runs at about 2.5 words a second; much slower
	// means extra speech (an instruction read aloud) or long silences.
	minWordsPerSecond = 1.4
	maxWordsPerSecond = 5.5
)

// Voice is a prebuilt voice CAVI offers.
type Voice struct {
	ID    string // the provider's voice name
	Label string // how it sounds, for people
}

// Voices are the voices offered, the first the default. They are the
// provider's prebuilt voices; arbitrary names are never accepted.
var Voices = []Voice{
	{ID: "Kore", Label: "Firm"},
	{ID: "Puck", Label: "Upbeat"},
	{ID: "Charon", Label: "Informative"},
	{ID: "Aoede", Label: "Breezy"},
	{ID: "Fenrir", Label: "Excitable"},
	{ID: "Leda", Label: "Youthful"},
}

// ValidVoiceID reports whether id is an offered voice.
func ValidVoiceID(id string) bool {
	for _, v := range Voices {
		if v.ID == id {
			return true
		}
	}
	return false
}

// Speech is synthesised audio: signed 16-bit little-endian mono PCM.
type Speech struct {
	PCM        []byte
	SampleRate int
}

// DurationMS is the speech's length.
func (s Speech) DurationMS() int64 {
	if s.SampleRate <= 0 {
		return 0
	}
	return int64(len(s.PCM)/2) * 1000 / int64(s.SampleRate)
}

// WAV wraps the speech in a WAV file.
func (s Speech) WAV() []byte {
	var b bytes.Buffer
	n := uint32(len(s.PCM))
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, 36+n)
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(s.SampleRate), uint32(s.SampleRate * 2), uint16(2), uint16(16)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, n)
	b.Write(s.PCM)
	return b.Bytes()
}

// Narrator turns a script into speech.
type Narrator interface {
	Speak(ctx context.Context, model Model, script, voice string) (Speech, Usage, error)
}

// CleanScript trims a script and checks its length.
func CleanScript(script string) (string, error) {
	script = strings.Join(strings.Fields(script), " ")
	if script == "" || !utf8.ValidString(script) || utf8.RuneCountInString(script) > MaxScriptRunes {
		return "", fmt.Errorf("script must be 1 to %d characters", MaxScriptRunes)
	}
	return script, nil
}

// ValidateSpeech checks synthesised audio is speech of a plausible length
// for the script: not empty, silent, truncated or runaway.
func ValidateSpeech(s Speech, script string) error {
	if s.SampleRate < 8000 || len(s.PCM) < 2 || len(s.PCM)%2 != 0 {
		return fmt.Errorf("%w: no usable audio", ErrInvalidOutput)
	}
	words := float64(len(strings.Fields(script)))
	secs := float64(s.DurationMS()) / 1000
	if secs <= 0 || words/secs < minWordsPerSecond || words/secs > maxWordsPerSecond {
		return fmt.Errorf("%w: %.1f s of audio for %.0f words", ErrInvalidOutput, secs, words)
	}
	var peak int16
	for i := 0; i+1 < len(s.PCM); i += 2 {
		v := int16(binary.LittleEndian.Uint16(s.PCM[i:]))
		if v < 0 {
			v = -v
		}
		peak = max(peak, v)
	}
	if peak < 500 {
		return fmt.Errorf("%w: audio is silent", ErrInvalidOutput)
	}
	return nil
}

// pcmOf returns the 16-bit mono PCM in a reply's audio data. Providers
// send either raw PCM (at rate) or a whole WAV file; the WAV header must
// not be played as sound, so its format is read and only its samples kept.
func pcmOf(data []byte, rate int) ([]byte, int, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return data, rate, nil
	}
	bad := func(msg string) ([]byte, int, error) {
		return nil, 0, fmt.Errorf("%w: WAV audio %s", ErrInvalidOutput, msg)
	}
	var channels, bits uint16
	for i := 12; i+8 <= len(data); {
		id, size := string(data[i:i+4]), int(binary.LittleEndian.Uint32(data[i+4:]))
		body := data[i+8:]
		if size > len(body) {
			size = len(body) // streams often leave the data size unset
		}
		switch id {
		case "fmt ":
			if size < 16 || binary.LittleEndian.Uint16(body) != 1 {
				return bad("is not PCM")
			}
			channels, rate, bits = binary.LittleEndian.Uint16(body[2:]), int(binary.LittleEndian.Uint32(body[4:])), binary.LittleEndian.Uint16(body[14:])
		case "data":
			if channels != 1 || bits != 16 {
				return bad(fmt.Sprintf("has %d channels of %d bits", channels, bits))
			}
			return body[:size-size%2], rate, nil
		}
		i += 8 + size + size%2
	}
	return bad("has no data")
}
