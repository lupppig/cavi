// Package ai is CAVI's boundary with AI providers. Product code asks for a
// task (transcription, highlight detection) with a resolved model and gets back validated,
// structured results; nothing outside this package imports a provider SDK.
//
// Models are chosen per organisation from a catalogue of models CAVI has
// validated against its structured output schemas. Arbitrary model IDs are
// never accepted. The resolved provider and model are stored on the work
// they produce, so retries use the same model even if settings change.
package ai

import (
	"context"
	"errors"
)

// Tasks AI is used for.
const (
	TaskTranscription = "transcription"
	TaskHighlights    = "highlights"
)

// Providers.
const (
	ProviderGemini = "gemini"
)

// Model is a provider model CAVI supports for a task.
type Model struct {
	Provider string
	ID       string
	Label    string
	Default  bool
}

// catalogue lists the models validated for each task: each was run against
// the evaluation fixtures (see gemini_test.go) and produced valid
// structured output. Add a model only after it passes them.
var catalogue = map[string][]Model{
	TaskTranscription: {
		{Provider: ProviderGemini, ID: "gemini-3.5-flash", Label: "Gemini 3.5 Flash", Default: true},
		{Provider: ProviderGemini, ID: "gemini-3.5-flash-lite", Label: "Gemini 3.5 Flash-Lite (lower cost)"},
	},
	TaskHighlights: {
		{Provider: ProviderGemini, ID: "gemini-3.5-flash", Label: "Gemini 3.5 Flash", Default: true},
		{Provider: ProviderGemini, ID: "gemini-3.5-flash-lite", Label: "Gemini 3.5 Flash-Lite (lower cost)"},
	},
}

// Tasks returns every task, in a stable order.
func Tasks() []string { return []string{TaskTranscription, TaskHighlights} }

// Models returns the supported models for task.
func Models(task string) []Model { return catalogue[task] }

// Default returns the platform default model for task.
func Default(task string) Model {
	for _, m := range catalogue[task] {
		if m.Default {
			return m
		}
	}
	return Model{}
}

// Lookup returns the supported model with this provider and ID.
func Lookup(task, provider, id string) (Model, bool) {
	for _, m := range catalogue[task] {
		if m.Provider == provider && m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Errors from providers.
var (
	// ErrInvalidOutput means the model's output failed validation. It may
	// succeed on retry.
	ErrInvalidOutput = errors.New("ai: model output failed validation")
	// ErrPermanent means the request can't succeed as made (rejected input,
	// blocked content, unsupported model). Don't retry.
	ErrPermanent = errors.New("ai: request cannot succeed")
)

// Usage is what a call consumed, for cost accounting.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Add accumulates usage across calls.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
}

// Segment is one stretch of speech.
type Segment struct {
	StartMS int64
	EndMS   int64
	Speaker string
	Text    string
}

// Transcript is a validated transcription of one piece of audio. Times are
// relative to the start of that audio.
type Transcript struct {
	Language string
	Segments []Segment
}

// Transcriber turns audio into a transcript.
type Transcriber interface {
	// Transcribe transcribes audio (at most a few minutes; callers chunk
	// longer media) of the given duration with model.
	Transcribe(ctx context.Context, model Model, audio []byte, mimeType string, durationMS int64) (Transcript, Usage, error)
}

// TranscriptionPromptVersion identifies the prompt and schema used, so
// results can be traced and compared when the prompt changes.
const TranscriptionPromptVersion = "transcription-v1"
