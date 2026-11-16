package ai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeEleven is a stand-in for the ElevenLabs API.
type fakeEleven struct {
	status int
	got    map[string]any
	path   string
	query  string
}

func (f *fakeEleven) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("xi-api-key") != "test-key" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &f.got)
	f.path, f.query = r.URL.Path, r.URL.RawQuery
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"detail":"nope"}`))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music") {
		mp3 := append([]byte("ID3"), make([]byte, 20_000)...)
		_, _ = w.Write(mp3)
		return
	}
	// 2 s of tone for "Marlon went to Botswana", timed per character.
	text := f.got["text"].(string)
	n := 48_000
	pcm := make([]byte, 2*n)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(9000*math.Sin(float64(i)/9))))
	}
	var chars []string
	var starts, ends []float64
	for i, c := range text {
		chars = append(chars, string(c))
		starts = append(starts, float64(i)*0.06)
		ends = append(ends, float64(i)*0.06+0.05)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"audio_base64": base64.StdEncoding.EncodeToString(pcm),
		"alignment": map[string]any{"characters": chars, "character_start_times_seconds": starts, "character_end_times_seconds": ends}})
}

func TestElevenLabsSpeak(t *testing.T) {
	f := &fakeEleven{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, err := NewElevenLabs("test-key", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{Provider: ProviderElevenLabs, ID: "eleven_v3"}
	voice := VoicesFor(ProviderElevenLabs)[0].ID
	s, _, err := e.Speak(context.Background(), m, "Marlon went [loudly] to Botswana", voice, StyleCurious)
	if err != nil {
		t.Fatal(err)
	}
	if f.path != "/v1/text-to-speech/"+voice+"/with-timestamps" || f.query != "output_format=pcm_24000" || f.got["model_id"] != "eleven_v3" {
		t.Fatalf("request %s?%s %v", f.path, f.query, f.got)
	}
	// The style opens the text as a tag; the script's own brackets are gone.
	if f.got["text"] != "[curious] Marlon went loudly to Botswana" {
		t.Fatalf("text sent = %q", f.got["text"])
	}
	if s.SampleRate != 24000 || s.DurationMS() != 2000 || len(s.Words) != 5 || s.Words[0].Text != "Marlon" || s.Words[0].StartMS != 600 {
		t.Fatalf("speech = %d Hz, %d ms, words %+v", s.SampleRate, s.DurationMS(), s.Words)
	}
	if _, _, err := e.Speak(context.Background(), m, "Hi there", "Kore", StyleNatural); !errors.Is(err, ErrPermanent) {
		t.Fatalf("Gemini voice on ElevenLabs: %v", err)
	}
	if _, _, err := e.Speak(context.Background(), m, "Hi there", voice, "sarcastic"); !errors.Is(err, ErrPermanent) {
		t.Fatalf("unknown style: %v", err)
	}

	f.status = http.StatusTooManyRequests
	if _, _, err := e.Speak(context.Background(), m, "Marlon went to Botswana", voice, StyleNatural); err == nil || errors.Is(err, ErrPermanent) {
		t.Fatalf("rate limit should be retryable: %v", err)
	}
	f.status = http.StatusUnprocessableEntity
	if _, _, err := e.Speak(context.Background(), m, "Marlon went to Botswana", voice, StyleNatural); !errors.Is(err, ErrPermanent) {
		t.Fatalf("422 should be permanent: %v", err)
	}
	bad, _ := NewElevenLabs("wrong", srv.URL)
	f.status = 0
	if _, _, err := bad.Speak(context.Background(), m, "Marlon went to Botswana", voice, StyleNatural); !errors.Is(err, ErrPermanent) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("bad key: %v", err)
	}
}

func TestElevenLabsCompose(t *testing.T) {
	f := &fakeEleven{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := NewElevenLabs("test-key", srv.URL)
	m := Model{Provider: ProviderElevenLabs, ID: "music_v1"}
	tr, _, err := e.Compose(context.Background(), m, " Soulful, warm  guitar ", 30_000)
	if err != nil {
		t.Fatal(err)
	}
	if f.path != "/v1/music" || f.got["force_instrumental"] != true || f.got["music_length_ms"] != float64(30_000) || f.got["prompt"] != "Soulful, warm guitar" || tr.MIMEType != "audio/mpeg" {
		t.Fatalf("request %s %v -> %s", f.path, f.got, tr.MIMEType)
	}
	if _, _, err := e.Compose(context.Background(), m, "x", 1000); !errors.Is(err, ErrPermanent) {
		t.Fatalf("too short: %v", err)
	}
	if err := ValidateTrack(Track{Data: []byte("not music")}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("ValidateTrack: %v", err)
	}
}

func TestRouting(t *testing.T) {
	n := Narrators{ProviderGemini: nil}
	if _, _, err := n.Speak(context.Background(), Model{Provider: ProviderElevenLabs}, "x", "y", StyleNatural); !errors.Is(err, ErrPermanent) {
		t.Fatalf("unconfigured provider: %v", err)
	}
	if !ValidStyleFor(ProviderElevenLabs, StyleCurious) || ValidStyleFor(ProviderGemini, StyleCurious) {
		t.Fatal("styles per provider")
	}
	if alignWords(strings.Split("[x] a b", ""), make([]float64, 7), make([]float64, 7))[0].Text != "a" {
		t.Fatal("tag not skipped")
	}
}

// TestEvalElevenLabs speaks a line plainly and in the curious style with
// the real API: the style must be performed, not read aloud (no more than
// a little longer than plain), and timings must cover every word.
// Then it composes 10 s of music. Runs with CAVI_AI_EVAL=1 and
// ELEVENLABS_API_KEY.
func TestEvalElevenLabs(t *testing.T) {
	key := os.Getenv("ELEVENLABS_API_KEY")
	if os.Getenv("CAVI_AI_EVAL") != "1" || key == "" {
		t.Skip("set CAVI_AI_EVAL=1 and ELEVENLABS_API_KEY to run the ElevenLabs evaluation")
	}
	e, err := NewElevenLabs(key, "")
	if err != nil {
		t.Fatal(err)
	}
	script := "Marlon flew to Botswana with one question. What did diamonds actually build here?"
	voice := VoicesFor(ProviderElevenLabs)[3].ID // Liam
	m := Model{Provider: ProviderElevenLabs, ID: os.Getenv("ELEVEN_MODEL")}
	if m.ID == "" {
		m.ID = "eleven_v3"
	}
	ctx := context.Background()
	plain, _, err := e.Speak(ctx, m, script, voice, StyleNatural)
	if err != nil {
		t.Fatal(err)
	}
	curious, _, err := e.Speak(ctx, m, script, voice, StyleCurious)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("plain %d ms, curious %d ms, %d words timed", plain.DurationMS(), curious.DurationMS(), len(curious.Words))
	if float64(curious.DurationMS()) > 1.35*float64(plain.DurationMS()) {
		t.Errorf("curious is much longer than plain: the tag may have been read aloud")
	}
	if len(curious.Words) != len(strings.Fields(script)) || curious.Words[0].Text != "Marlon" {
		t.Errorf("word timings = %+v", curious.Words)
	}
	_ = os.WriteFile(os.Getenv("EVAL_OUT")+"plain.wav", plain.WAV(), 0o644)
	_ = os.WriteFile(os.Getenv("EVAL_OUT")+"curious.wav", curious.WAV(), 0o644)
	tr, _, err := e.Compose(ctx, Model{Provider: ProviderElevenLabs, ID: "music_v1"}, "Soulful, warm, gentle instrumental: soft guitar and Rhodes, light percussion, hopeful.", 10_000)
	if err != nil {
		t.Logf("music: %v", err)
		return
	}
	t.Logf("music: %d bytes", len(tr.Data))
	_ = os.WriteFile(os.Getenv("EVAL_OUT")+"music.mp3", tr.Data, 0o644)
}
