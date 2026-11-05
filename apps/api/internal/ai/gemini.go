package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/genai"
)

// Gemini implements AI tasks with the Gemini API.
type Gemini struct {
	client *genai.Client
}

// NewGemini returns a Gemini provider. The API key is a backend secret.
func NewGemini(ctx context.Context, apiKey string) (*Gemini, error) {
	if apiKey == "" {
		return nil, errors.New("ai: Gemini API key is empty")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &Gemini{client: c}, nil
}

const transcriptionPrompt = "Transcribe this audio verbatim in its original language. " +
	"Split it into segments of one sentence or short phrase spoken by a single speaker. " +
	"Give start_ms and end_ms in milliseconds from the start of this audio. " +
	"Label speakers 'Speaker 1', 'Speaker 2', ... in order of first appearance. " +
	"Give the main spoken language as a BCP-47 code in language. " +
	"Do not summarise, translate, correct or invent words. If there is no speech, return no segments."

var transcriptionSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"language": {Type: genai.TypeString},
		"segments": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"start_ms": {Type: genai.TypeInteger},
					"end_ms":   {Type: genai.TypeInteger},
					"speaker":  {Type: genai.TypeString},
					"text":     {Type: genai.TypeString},
				},
				Required: []string{"start_ms", "end_ms", "speaker", "text"},
			},
		},
	},
	Required: []string{"language", "segments"},
}

type transcriptionJSON struct {
	Language string `json:"language"`
	Segments []struct {
		StartMS int64  `json:"start_ms"`
		EndMS   int64  `json:"end_ms"`
		Speaker string `json:"speaker"`
		Text    string `json:"text"`
	} `json:"segments"`
}

// Transcribe implements Transcriber.
func (g *Gemini) Transcribe(ctx context.Context, model Model, audio []byte, mimeType string, durationMS int64) (Transcript, Usage, error) {
	if model.Provider != ProviderGemini {
		return Transcript{}, Usage{}, fmt.Errorf("%w: model %s is not a Gemini model", ErrPermanent, model.ID)
	}
	temp := float32(0)
	resp, err := g.client.Models.GenerateContent(ctx, model.ID,
		[]*genai.Content{genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromBytes(audio, mimeType),
			genai.NewPartFromText(transcriptionPrompt),
		}, genai.RoleUser)},
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   transcriptionSchema,
			Temperature:      &temp,
		})
	if err != nil {
		return Transcript{}, Usage{}, classify(err)
	}
	usage := usageOf(resp)
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return Transcript{}, usage, fmt.Errorf("%w: input blocked (%s)", ErrPermanent, resp.PromptFeedback.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return Transcript{}, usage, fmt.Errorf("%w: no candidates", ErrInvalidOutput)
	}
	switch fr := resp.Candidates[0].FinishReason; fr {
	case genai.FinishReasonStop, "":
	case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonProhibitedContent, genai.FinishReasonBlocklist:
		return Transcript{}, usage, fmt.Errorf("%w: output blocked (%s)", ErrPermanent, fr)
	default:
		return Transcript{}, usage, fmt.Errorf("%w: generation stopped (%s)", ErrInvalidOutput, fr)
	}

	var out transcriptionJSON
	if err := json.Unmarshal([]byte(resp.Text()), &out); err != nil {
		return Transcript{}, usage, fmt.Errorf("%w: not JSON: %v", ErrInvalidOutput, err)
	}
	t := Transcript{Language: out.Language, Segments: make([]Segment, len(out.Segments))}
	for i, s := range out.Segments {
		t.Segments[i] = Segment{StartMS: s.StartMS, EndMS: s.EndMS, Speaker: s.Speaker, Text: s.Text}
	}
	t, err = ValidateTranscript(t, durationMS)
	return t, usage, err
}

func usageOf(resp *genai.GenerateContentResponse) Usage {
	if resp == nil || resp.UsageMetadata == nil {
		return Usage{}
	}
	u := resp.UsageMetadata
	return Usage{InputTokens: int64(u.PromptTokenCount), OutputTokens: int64(u.CandidatesTokenCount) + int64(u.ThoughtsTokenCount)}
}

// classify maps API errors: rate limits and server errors may succeed on
// retry; other client errors won't.
func classify(err error) error {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code == http.StatusTooManyRequests || apiErr.Code >= 500:
			return fmt.Errorf("gemini: temporarily unavailable (%d %s)", apiErr.Code, apiErr.Status)
		case apiErr.Code >= 400:
			return fmt.Errorf("%w: gemini rejected the request (%d %s: %s)", ErrPermanent, apiErr.Code, apiErr.Status, apiErr.Message)
		}
	}
	return fmt.Errorf("gemini: %w", err)
}
