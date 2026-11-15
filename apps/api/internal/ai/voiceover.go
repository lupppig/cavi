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
	Provider string
	ID       string // the provider's voice name or ID
	Label    string // how it sounds, for people
}

// Voices are the voices offered, per provider, each provider's first its
// default. They are the providers' prebuilt voices; arbitrary voices are
// never accepted.
var Voices = []Voice{
	{Provider: ProviderGemini, ID: "Kore", Label: "Firm"},
	{Provider: ProviderGemini, ID: "Puck", Label: "Upbeat"},
	{Provider: ProviderGemini, ID: "Charon", Label: "Informative"},
	{Provider: ProviderGemini, ID: "Aoede", Label: "Breezy"},
	{Provider: ProviderGemini, ID: "Fenrir", Label: "Excitable"},
	{Provider: ProviderGemini, ID: "Leda", Label: "Youthful"},
	{Provider: ProviderGemini, ID: "Zephyr", Label: "Bright"},
	{Provider: ProviderElevenLabs, ID: "JBFqnCBsd6RMkjVDRZzb", Label: "George · warm, captivating storyteller"},
	{Provider: ProviderElevenLabs, ID: "nPczCjzI2devNBz1zQrb", Label: "Brian · deep, resonant"},
	{Provider: ProviderElevenLabs, ID: "IKne3meq5aSn9XLyUdCD", Label: "Charlie · confident, energetic"},
	{Provider: ProviderElevenLabs, ID: "TX3LPaxmHKxFdv7VOQHJ", Label: "Liam · energetic social-media creator"},
	{Provider: ProviderElevenLabs, ID: "EXAVITQu4vr4xnSDxMaL", Label: "Sarah · mature, reassuring"},
}

// VoicesFor returns the voices a provider offers, its default first.
func VoicesFor(provider string) []Voice {
	var out []Voice
	for _, v := range Voices {
		if v.Provider == provider {
			out = append(out, v)
		}
	}
	return out
}

// ValidVoiceFor reports whether id is a voice the provider offers.
func ValidVoiceFor(provider, id string) bool {
	for _, v := range VoicesFor(provider) {
		if v.ID == id {
			return true
		}
	}
	return false
}

// Delivery styles for voiceovers. Only expressive models take a style
// other than natural (see StylesFor).
const (
	StyleNatural     = "natural"
	StyleCurious     = "curious"
	StyleExcited     = "excited"
	StylePlayful     = "playful"
	StyleWarm        = "warm"
	StyleStoryteller = "storyteller"
)

// StylesFor returns the delivery styles a provider can perform. Gemini's
// voices read any direction aloud, so they only speak naturally.
func StylesFor(provider string) []string {
	if provider == ProviderElevenLabs {
		return []string{StyleNatural, StyleCurious, StyleExcited, StylePlayful, StyleWarm, StyleStoryteller}
	}
	return []string{StyleNatural}
}

// ValidStyleFor reports whether a provider can perform style.
func ValidStyleFor(provider, style string) bool {
	for _, s := range StylesFor(provider) {
		if s == style {
			return true
		}
	}
	return false
}

// Speech is synthesised audio: signed 16-bit little-endian mono PCM.
type Speech struct {
	PCM        []byte
	SampleRate int
	// Words are the script's words timed by the provider, when it gives
	// timings; otherwise empty and callers time them.
	Words []Word
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

// Narrator turns a script into speech in a voice and delivery style.
type Narrator interface {
	Speak(ctx context.Context, model Model, script, voice, style string) (Speech, Usage, error)
}

// Narrators routes speech to the provider of the model asked for.
type Narrators map[string]Narrator

// Speak implements Narrator.
func (n Narrators) Speak(ctx context.Context, model Model, script, voice, style string) (Speech, Usage, error) {
	p, ok := n[model.Provider]
	if !ok {
		return Speech{}, Usage{}, fmt.Errorf("%w: provider %s is not configured", ErrPermanent, model.Provider)
	}
	return p.Speak(ctx, model, script, voice, style)
}

// CleanScript trims a script and checks its length. Square brackets are
// removed: expressive models read them as direction, and direction comes
// only from the chosen style.
func CleanScript(script string) (string, error) {
	script = strings.NewReplacer("[", "", "]", "").Replace(script)
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

// Music limits.
const (
	MusicPromptVersion  = "music-v1"
	MaxMusicPromptRunes = 500
	MinMusicMS          = 5_000
	MaxMusicMS          = 180_000
)

// Track is composed music: an encoded audio file.
type Track struct {
	Data     []byte
	MIMEType string
}

// Composer writes instrumental music to a description.
type Composer interface {
	Compose(ctx context.Context, model Model, prompt string, lengthMS int64) (Track, Usage, error)
}

// Composers routes music to the provider of the model asked for.
type Composers map[string]Composer

// Compose implements Composer.
func (c Composers) Compose(ctx context.Context, model Model, prompt string, lengthMS int64) (Track, Usage, error) {
	p, ok := c[model.Provider]
	if !ok {
		return Track{}, Usage{}, fmt.Errorf("%w: provider %s is not configured", ErrPermanent, model.Provider)
	}
	return p.Compose(ctx, model, prompt, lengthMS)
}

// CleanMusicPrompt trims a music description and checks its length.
func CleanMusicPrompt(prompt string) (string, error) {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if prompt == "" || !utf8.ValidString(prompt) || utf8.RuneCountInString(prompt) > MaxMusicPromptRunes {
		return "", fmt.Errorf("describe the music in 1 to %d characters", MaxMusicPromptRunes)
	}
	return prompt, nil
}

// ValidateTrack checks composed music is an MP3 of some substance.
func ValidateTrack(t Track) error {
	d := t.Data
	if len(d) < 8_000 {
		return fmt.Errorf("%w: %d bytes of music", ErrInvalidOutput, len(d))
	}
	if !(string(d[:3]) == "ID3" || (d[0] == 0xFF && d[1]&0xE0 == 0xE0)) {
		return fmt.Errorf("%w: music is not MP3", ErrInvalidOutput)
	}
	return nil
}
