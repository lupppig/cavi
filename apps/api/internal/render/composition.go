// Package render turns compositions into finished clips.
//
// A composition is the structured, versioned description of a clip: which
// source file, which stretch of it, what shape, and where the frame sits.
// The editor (and later the Creative Director) produce compositions; the
// render worker interprets them. Composition semantics live here; how
// FFmpeg carries them out lives in the ffmpeg package.
package render

import (
	"fmt"
	"math"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
)

// CompositionVersion is the composition schema this code writes. Version
// 1 (no captions) is still read.
const CompositionVersion = 2

// Clip length limits.
const (
	MinClip = time.Second
	MaxClip = 10 * time.Minute
)

// Shape is an output frame: its aspect ratio and the size it renders at.
type Shape struct {
	Aspect        string
	Width, Height int
}

// Shapes are the supported output frames. Output is always rendered at
// these sizes, the sizes social platforms expect, scaling the source up
// if it is smaller.
var Shapes = []Shape{
	{Aspect: "9:16", Width: 1080, Height: 1920},
	{Aspect: "1:1", Width: 1080, Height: 1080},
	{Aspect: "4:5", Width: 1080, Height: 1350},
	{Aspect: "16:9", Width: 1920, Height: 1080},
}

func shape(aspect string) (Shape, bool) {
	for _, s := range Shapes {
		if s.Aspect == aspect {
			return s, true
		}
	}
	return Shape{}, false
}

// Composition describes one clip cut from one source file.
type Composition struct {
	Version       int    `json:"version"`
	SourceAssetID string `json:"source_asset_id"`
	StartMS       int64  `json:"start_ms"`
	EndMS         int64  `json:"end_ms"`
	// Aspect is one of Shapes.
	Aspect string `json:"aspect"`
	// Focus places the frame along the axis the source is cropped on: 0 is
	// the left (or top) edge, 0.5 the centre, 1 the right (or bottom).
	Focus float64 `json:"focus"`
	// Framing is FramingFixed (the default, at Focus) or FramingAuto
	// (following the people in shot; Focus where there are none).
	Framing string `json:"framing,omitempty"`
	// Captions (version 2) are burned in when present.
	Captions *Captions `json:"captions,omitempty"`
	// Segments and Hook are used by montages (version 3); the fields
	// above then mirror the first segment.
	Segments  []Segment     `json:"segments,omitempty"`
	Hook      *Hook         `json:"hook,omitempty"`
	Voiceover *VoiceoverMix `json:"voiceover,omitempty"`
	SignOff   *SignOff      `json:"sign_off,omitempty"`
	// Tighten (montages) cuts the pauses out of every segment; the server
	// works out each segment's Gaps from its file's pauses.
	Tighten bool `json:"tighten,omitempty"`
}

// Source is what rendering needs to know about the source file.
type Source struct {
	Width, Height int
	DurationMS    int64
	HasAudio      bool
}

func invalid(field, msg string) error { return &account.ValidationError{Field: field, Message: msg} }

// Validate checks c against its source. Field names match the API's
// composition object.
func (c Composition) Validate(src Source) error {
	if c.Version != 1 && c.Version != CompositionVersion {
		return invalid("composition.version", fmt.Sprintf("Composition version must be 1 or %d.", CompositionVersion))
	}
	if len(c.Segments) > 0 || c.Hook != nil || c.Voiceover != nil || c.SignOff != nil || c.Tighten {
		return invalid("composition.version", fmt.Sprintf("Segments and hooks need composition version %d.", MontageVersion))
	}
	if c.Version == 1 && c.Captions != nil {
		return invalid("composition.version", "Captions need composition version 2.")
	}
	if err := validFraming(c); err != nil {
		return err
	}
	if src.Width < 2 || src.Height < 2 || src.DurationMS <= 0 {
		return invalid("composition.source_asset_id", "Choose a processed video file.")
	}
	if c.StartMS < 0 || c.StartMS >= src.DurationMS {
		return invalid("composition.start_ms", "The clip must start within the file.")
	}
	if c.EndMS > src.DurationMS {
		return invalid("composition.end_ms", "The clip can’t end after the file does.")
	}
	d := time.Duration(c.EndMS-c.StartMS) * time.Millisecond
	if d < MinClip {
		return invalid("composition.end_ms", "Clips must be at least a second long.")
	}
	if d > MaxClip {
		return invalid("composition.end_ms", fmt.Sprintf("Clips can be at most %d minutes long.", int(MaxClip.Minutes())))
	}
	if _, ok := shape(c.Aspect); !ok {
		return invalid("composition.aspect", "Choose 9:16, 1:1, 4:5 or 16:9.")
	}
	if math.IsNaN(c.Focus) || c.Focus < 0 || c.Focus > 1 {
		return invalid("composition.focus", "Framing must be between 0 and 1.")
	}
	if c.Captions != nil {
		return c.Captions.validate(c.EndMS - c.StartMS)
	}
	return nil
}

// even rounds n to the nearest even number, at least 2.
func even(n float64) int {
	e := int(math.Round(n/2)) * 2
	return max(e, 2)
}

// Plan works out the FFmpeg clip for a valid composition: the largest
// region of the source with the output's aspect ratio, placed by Focus
// (or moving along path, when auto framed), scaled to the output size.
func Plan(c Composition, src Source, path []framing.Key) ffmpeg.Clip {
	sh, _ := shape(c.Aspect)
	cl := ffmpeg.Clip{
		Start:  time.Duration(c.StartMS) * time.Millisecond,
		Length: time.Duration(c.EndMS-c.StartMS) * time.Millisecond,
		Width:  sh.Width, Height: sh.Height,
		Audio: src.HasAudio,
	}
	sw, shh := src.Width, src.Height
	if sw*sh.Height > shh*sh.Width {
		// Source is wider than the output: use its full height.
		cl.CropH = shh - shh%2
		cl.CropW = min(even(float64(shh)*float64(sh.Width)/float64(sh.Height)), sw-sw%2)
		cl.CropX = int(math.Round(float64(sw-cl.CropW) * c.Focus))
	} else {
		cl.CropW = sw - sw%2
		cl.CropH = min(even(float64(sw)*float64(sh.Height)/float64(sh.Width)), shh-shh%2)
		cl.CropY = int(math.Round(float64(shh-cl.CropH) * c.Focus))
	}
	if len(path) > 0 {
		cl.Pan, cl.PanVertical = pan(path, sh, src)
	}
	return cl
}
