// Package framing finds where the people are in a video and works out a
// camera path that keeps them in frame when the video is cropped to
// another shape (16:9 footage made vertical, say).
//
// Analysis runs once per video in the media worker: frames are sampled,
// faces are found with OpenCV's YuNet detector (the cavi-faces helper)
// and shot changes are found from brightness histograms. The result, a
// Track, is stored and later turned into a Path for any stretch and
// output shape, so the editor's preview and the render follow the same
// camera.
//
// It follows faces, not voices: with several people in shot it keeps to
// the most prominent one rather than whoever is talking.
package framing

import (
	"fmt"
	"math"
)

// Version identifies the analyser; tracks from older versions are redone.
const Version = 1

// Sampling. Frames are read SampleFPS times a second for shot changes, and
// every faceEvery-th one is searched for faces.
const (
	SampleFPS = 10
	faceEvery = 4
	// FaceIntervalMS is the time between face samples.
	FaceIntervalMS = 1000 * faceEvery / SampleFPS
	longSide       = 480
)

// Detection thresholds, tuned on real footage.
const (
	// minFace is the smallest face kept, as a fraction of the frame's
	// shorter side: smaller ones are crowds and posters.
	minFace = 0.04
	// cutDistance is how different two consecutive frames' brightness
	// histograms must be (0 to 1) to count as a shot change.
	cutDistance = 0.4
	histBins    = 32
)

// Box is a face a Detector found, in pixels of the analysis frame.
type Box struct{ X, Y, W, H, Score float64 }

// Detector finds faces in a packed BGR frame of the analysis size.
type Detector interface {
	Detect(bgr []byte) ([]Box, error)
}

// Face is a face found in a frame. X and Y are its centre as fractions of
// the frame's width and height; S is its size as a fraction of the
// frame's shorter side; Q is the detector's confidence (0 to 1).
type Face struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	S float64 `json:"s"`
	Q float64 `json:"q"`
}

// Track is what analysis learned about a video: the faces in each face
// sample (sample i is at i×IntervalMS) and the shot changes.
type Track struct {
	Version    int      `json:"version"`
	IntervalMS int64    `json:"interval_ms"`
	Faces      [][]Face `json:"faces"`
	// Cuts are shot changes, in milliseconds, ascending.
	Cuts []int64 `json:"cuts"`
}

// FrameSize is the analysis frame size for a source of w×h: the longer
// side 480 pixels, both even.
func FrameSize(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if w >= h {
		return longSide, max(16, int(math.Round(float64(h)*longSide/float64(w)/2))*2)
	}
	return max(16, int(math.Round(float64(w)*longSide/float64(h)/2))*2), longSide
}

// Analyser builds a Track from frames fed in order.
type Analyser struct {
	det     Detector
	w, h    int
	prev    [histBins]float64
	hasPrev bool
	track   Track
}

// NewAnalyser returns an analyser for frames of w×h (from FrameSize).
func NewAnalyser(w, h int, det Detector) *Analyser {
	return &Analyser{det: det, w: w, h: h, track: Track{Version: Version, IntervalMS: FaceIntervalMS, Faces: [][]Face{}, Cuts: []int64{}}}
}

// Frame takes sampled frame i (at i/SampleFPS seconds), w×h packed BGR.
func (a *Analyser) Frame(i int, bgr []byte) error {
	if len(bgr) != a.w*a.h*3 {
		return fmt.Errorf("framing: frame is %d bytes, want %d", len(bgr), a.w*a.h*3)
	}
	hist := histogram(bgr)
	if a.hasPrev && distance(a.prev, hist) > cutDistance {
		// The change happened between the two samples: put it halfway.
		a.track.Cuts = append(a.track.Cuts, int64(i)*1000/SampleFPS-500/SampleFPS)
	}
	a.prev, a.hasPrev = hist, true
	if i%faceEvery != 0 {
		return nil
	}
	boxes, err := a.det.Detect(bgr)
	if err != nil {
		return err
	}
	short := float64(min(a.w, a.h))
	faces := []Face{}
	for _, b := range boxes {
		s := math.Sqrt(b.W*b.H) / short
		if s < minFace || b.W <= 0 || b.H <= 0 {
			continue
		}
		faces = append(faces, Face{
			X: round3(clamp01((b.X + b.W/2) / float64(a.w))),
			Y: round3(clamp01((b.Y + b.H/2) / float64(a.h))),
			S: round3(s),
			Q: round3(clamp01(b.Score)),
		})
	}
	a.track.Faces = append(a.track.Faces, faces)
	return nil
}

// Track returns what was learned so far.
func (a *Analyser) Track() Track { return a.track }

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func clamp01(v float64) float64 { return max(0, min(1, v)) }

// histogram is the frame's normalised brightness histogram, from every
// other pixel.
func histogram(bgr []byte) [histBins]float64 {
	var h [histBins]float64
	n := 0.0
	for i := 0; i+2 < len(bgr); i += 6 {
		y := (int(bgr[i]) + 2*int(bgr[i+1]) + int(bgr[i+2])) / 4
		h[y*histBins/256]++
		n++
	}
	if n > 0 {
		for k := range h {
			h[k] /= n
		}
	}
	return h
}

// distance is half the L1 distance between two histograms: 0 for the
// same, 1 for no overlap.
func distance(a, b [histBins]float64) float64 {
	d := 0.0
	for k := range a {
		d += math.Abs(a[k] - b[k])
	}
	return d / 2
}
