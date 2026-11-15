package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// ProviderElevenLabs is ElevenLabs: expressive voices and music.
const ProviderElevenLabs = "elevenlabs"

// ElevenLabs speaks voiceovers and composes music with the ElevenLabs
// API. The API key is a backend secret.
type ElevenLabs struct {
	key  string
	base string
	http *http.Client
}

// NewElevenLabs returns an ElevenLabs provider. base is the API's base
// URL; empty means the public API.
func NewElevenLabs(apiKey, base string) (*ElevenLabs, error) {
	if apiKey == "" {
		return nil, errors.New("ai: ElevenLabs API key is empty")
	}
	if base == "" {
		base = "https://api.elevenlabs.io"
	}
	return &ElevenLabs{key: apiKey, base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 4 * time.Minute}}, nil
}

// styleTags are the audio tags each delivery style opens with. Expressive
// models read tags as direction, not words.
var styleTags = map[string]string{
	StyleNatural:     "",
	StyleCurious:     "[curious]",
	StyleExcited:     "[excited]",
	StylePlayful:     "[playful]",
	StyleWarm:        "[warmly]",
	StyleStoryteller: "[storytelling]",
}

// post sends a JSON request and returns the response body. Rate limits
// and server errors may pass on retry; other client errors won't.
func (e *ElevenLabs) post(ctx context.Context, path string, query url.Values, body any) ([]byte, http.Header, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	u := e.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("xi-api-key", e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: read reply: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, nil, fmt.Errorf("elevenlabs: temporarily unavailable (%d)", resp.StatusCode)
	case resp.StatusCode >= 400:
		// The body explains the refusal; it never holds the key.
		msg := string(out)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, nil, fmt.Errorf("%w: elevenlabs rejected the request (%d: %s)", ErrPermanent, resp.StatusCode, msg)
	}
	return out, resp.Header, nil
}

type elevenTTSReply struct {
	AudioBase64 string `json:"audio_base64"`
	Alignment   *struct {
		Characters []string  `json:"characters"`
		Starts     []float64 `json:"character_start_times_seconds"`
		Ends       []float64 `json:"character_end_times_seconds"`
	} `json:"alignment"`
}

// Speak implements Narrator. The style opens the text as an audio tag;
// the returned words are the script's (without the tag), timed from the
// provider's character alignment.
func (e *ElevenLabs) Speak(ctx context.Context, model Model, script, voice, style string) (Speech, Usage, error) {
	if model.Provider != ProviderElevenLabs {
		return Speech{}, Usage{}, fmt.Errorf("%w: model %s is not an ElevenLabs model", ErrPermanent, model.ID)
	}
	if !ValidVoiceFor(ProviderElevenLabs, voice) {
		return Speech{}, Usage{}, fmt.Errorf("%w: voice %q is not offered", ErrPermanent, voice)
	}
	tag, ok := styleTags[style]
	if !ok {
		return Speech{}, Usage{}, fmt.Errorf("%w: style %q is not offered", ErrPermanent, style)
	}
	script, err := CleanScript(script)
	if err != nil {
		return Speech{}, Usage{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	text := script
	if tag != "" {
		text = tag + " " + script
	}
	body, _, err := e.post(ctx, "/v1/text-to-speech/"+url.PathEscape(voice)+"/with-timestamps",
		url.Values{"output_format": {"pcm_24000"}},
		map[string]any{"text": text, "model_id": model.ID,
			"voice_settings": map[string]any{"stability": 0.4, "similarity_boost": 0.75, "style": 0.5, "use_speaker_boost": true}})
	usage := Usage{InputTokens: int64(len([]rune(text)))} // ElevenLabs bills by character
	if err != nil {
		return Speech{}, usage, err
	}
	var r elevenTTSReply
	if err := json.Unmarshal(body, &r); err != nil {
		return Speech{}, usage, fmt.Errorf("%w: not JSON: %v", ErrInvalidOutput, err)
	}
	pcm, err := base64.StdEncoding.DecodeString(r.AudioBase64)
	if err != nil {
		return Speech{}, usage, fmt.Errorf("%w: audio is not base64: %v", ErrInvalidOutput, err)
	}
	s := Speech{PCM: pcm, SampleRate: 24000}
	if r.Alignment != nil && len(r.Alignment.Characters) == len(r.Alignment.Starts) && len(r.Alignment.Starts) == len(r.Alignment.Ends) {
		s.Words = alignWords(r.Alignment.Characters, r.Alignment.Starts, r.Alignment.Ends)
		if len(s.Words) != len(strings.Fields(script)) {
			s.Words = nil // doesn't line up: callers estimate instead
		}
	}
	return s, usage, ValidateSpeech(s, script)
}

// alignWords turns character timings into word timings, skipping audio
// tags ([curious]) which are direction, not words.
func alignWords(chars []string, starts, ends []float64) []Word {
	var words []Word
	var cur strings.Builder
	var from, to float64
	inTag := false
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, Word{StartMS: int64(from * 1000), EndMS: int64(to * 1000), Text: cur.String()})
			cur.Reset()
		}
	}
	for i, c := range chars {
		switch {
		case c == "[":
			flush()
			inTag = true
		case c == "]":
			inTag = false
		case inTag:
		case strings.TrimFunc(c, unicode.IsSpace) == "":
			flush()
		default:
			if cur.Len() == 0 {
				from = starts[i]
			}
			cur.WriteString(c)
			to = ends[i]
		}
	}
	flush()
	return words
}

// Compose implements Composer: an instrumental of about lengthMS.
func (e *ElevenLabs) Compose(ctx context.Context, model Model, prompt string, lengthMS int64) (Track, Usage, error) {
	if model.Provider != ProviderElevenLabs {
		return Track{}, Usage{}, fmt.Errorf("%w: model %s is not an ElevenLabs model", ErrPermanent, model.ID)
	}
	prompt, err := CleanMusicPrompt(prompt)
	if err != nil {
		return Track{}, Usage{}, fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	if lengthMS < MinMusicMS || lengthMS > MaxMusicMS {
		return Track{}, Usage{}, fmt.Errorf("%w: music must be %d to %d seconds", ErrPermanent, MinMusicMS/1000, MaxMusicMS/1000)
	}
	body, _, err := e.post(ctx, "/v1/music", url.Values{"output_format": {"mp3_44100_128"}},
		map[string]any{"prompt": prompt, "music_length_ms": lengthMS, "model_id": model.ID, "force_instrumental": true})
	usage := Usage{OutputTokens: lengthMS / 1000} // billed by generated length
	if err != nil {
		return Track{}, usage, err
	}
	t := Track{Data: body, MIMEType: "audio/mpeg"}
	return t, usage, ValidateTrack(t)
}
