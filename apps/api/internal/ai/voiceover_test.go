package ai

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func tone(seconds float64, amp float64) Speech {
	n := int(seconds * 24000)
	pcm := make([]byte, 2*n)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(amp*math.Sin(float64(i)/10))))
	}
	return Speech{PCM: pcm, SampleRate: 24000}
}

func TestValidateSpeech(t *testing.T) {
	script := "Natural diamonds helped build schools across Botswana." // 7 words
	if err := ValidateSpeech(tone(3, 8000), script); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]Speech{
		"empty":     {SampleRate: 24000},
		"silent":    tone(3, 10),
		"truncated": tone(0.5, 8000),
		"runaway":   tone(30, 8000),
	} {
		if err := ValidateSpeech(s, script); !errors.Is(err, ErrInvalidOutput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	w := tone(1, 8000).WAV()
	if string(w[:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " || binary.LittleEndian.Uint32(w[24:]) != 24000 || len(w) != 44+48000 {
		t.Fatalf("bad WAV header: % x", w[:44])
	}
	// A whole WAV file in the reply: only its samples are speech.
	inner := tone(1, 8000)
	inner.SampleRate = 16000
	pcm, rate, err := pcmOf(inner.WAV(), 24000)
	if err != nil || rate != 16000 || len(pcm) != len(inner.PCM) || string(pcm[:4]) == "RIFF" {
		t.Fatalf("pcmOf(WAV) = %d bytes at %d Hz, %v", len(pcm), rate, err)
	}
	if raw, rate, _ := pcmOf(inner.PCM, 24000); len(raw) != len(inner.PCM) || rate != 24000 {
		t.Fatal("raw PCM changed")
	}
	stereo := inner.WAV()
	stereo[22] = 2
	if _, _, err := pcmOf(stereo, 24000); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("stereo WAV: %v", err)
	}
	// Speech padded by extra words (an instruction read aloud) is too slow.
	if err := ValidateSpeech(tone(7, 8000), script); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("7 s for 7 words: %v", err)
	}
	if _, err := CleanScript("   "); err == nil {
		t.Error("empty script accepted")
	}
}

// TestEvalVoiceover speaks a short campaign line with each catalogued
// model, in two voices.
func TestEvalVoiceover(t *testing.T) {
	g := evalProvider(t)
	script := "In Botswana, natural diamonds helped pay for schools, roads and clinics. Here is what that looks like today."
	for _, m := range Models(TaskVoiceover) {
		for _, voice := range []string{"Kore", "Puck"} {
			t.Run(m.ID+"/"+voice, func(t *testing.T) {
				var s Speech
				var err error
				for _, wait := range []time.Duration{0, 20 * time.Second, 60 * time.Second} {
					time.Sleep(wait)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					s, _, err = g.Speak(ctx, m, script, voice, StyleNatural)
					cancel()
					if err == nil || errors.Is(err, ErrPermanent) {
						break
					}
					t.Logf("retrying after: %v", err)
				}
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%d ms at %d Hz", s.DurationMS(), s.SampleRate)
			})
		}
	}
}
