package render

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/framing"
	"github.com/lupppig/cavi/apps/api/internal/media"
	"github.com/lupppig/cavi/apps/api/internal/project"
)

// Framing modes. Fixed (the default) holds the crop at the composition's
// focus; auto follows the people in shot, falling back to the focus where
// there are none.
const (
	FramingFixed = "fixed"
	FramingAuto  = "auto"
)

// Auto reports whether c follows people.
func (c Composition) Auto() bool { return c.Framing == FramingAuto }

func validFraming(c Composition) error {
	switch c.Framing {
	case "", FramingFixed:
		return nil
	case FramingAuto:
		if c.Version == 1 {
			return invalid("composition.framing", "Auto framing needs composition version 2.")
		}
		return nil
	}
	return invalid("composition.framing", "Framing must be fixed or auto.")
}

// window is how a crop for shape sh moves over src.
func window(sh Shape, src Source) framing.Window {
	_, _, w, h := region(sh, src, 0)
	if w < src.Width-src.Width%2 {
		return framing.Window{Size: float64(w) / float64(src.Width)}
	}
	return framing.Window{Size: float64(h) / float64(src.Height), Vertical: true}
}

// FramingPath is the camera path auto framing follows for startMS to
// endMS of a source, cropped to aspect, with focus where nobody is in
// shot. The editor's preview and the render both use it.
func FramingPath(t framing.Track, src Source, aspect string, startMS, endMS int64, focus float64) []framing.Key {
	sh, ok := shape(aspect)
	if !ok {
		return []framing.Key{{AtMS: 0, Focus: focus}}
	}
	return t.Path(startMS, endMS, window(sh, src), focus)
}

// pan turns a path into crop positions, in pixels, for shape sh.
func pan(keys []framing.Key, sh Shape, src Source) ([]ffmpeg.PanKey, bool) {
	_, _, w, h := region(sh, src, 0)
	win := window(sh, src)
	room := src.Width - w
	if win.Vertical {
		room = src.Height - h
	}
	out := make([]ffmpeg.PanKey, len(keys))
	for i, k := range keys {
		out[i] = ffmpeg.PanKey{At: time.Duration(k.AtMS) * time.Millisecond, Pos: int(math.Round(float64(room) * k.Focus))}
	}
	return out, win.Vertical
}

// framingTracks loads the framing analysis of every source an auto-framed
// composition follows (segments' own scenes; split-screen tiles keep
// their focus). A source without one is a validation error naming it.
func framingTracks(ctx context.Context, db bun.IDB, orgID string, c Composition) (map[string]framing.Track, error) {
	if !c.Auto() {
		return nil, nil
	}
	ids := []string{c.SourceAssetID}
	field := "composition.framing"
	if c.Version == MontageVersion {
		ids = nil
		for _, s := range c.Segments {
			ids = append(ids, s.SourceAssetID)
		}
	}
	tracks := map[string]framing.Track{}
	for _, id := range ids {
		if _, ok := tracks[id]; ok {
			continue
		}
		t, ok, err := media.FramingTrack(ctx, db, orgID, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			var name string
			_ = db.QueryRowContext(ctx, `SELECT filename FROM media_assets WHERE id = ? AND organisation_id = ?`, id, orgID).Scan(&name)
			return nil, invalid(field, fmt.Sprintf("Auto framing isn’t ready for “%s” yet: it’s still finding the people in it, or that failed. Try again soon, or use fixed framing.", name))
		}
		tracks[id] = t
	}
	return tracks, nil
}

// FramingPreview is the camera path auto framing would follow for startMS
// to endMS of a video in the project, cropped to aspect, for the editor's
// preview. Anyone who can see the project can ask.
func (s *Service) FramingPreview(ctx context.Context, actorID, orgID, projectID, assetID, aspect string, startMS, endMS int64, focus float64) ([]framing.Key, error) {
	if _, _, err := project.Resolve(ctx, s.db, actorID, orgID, projectID, false); err != nil {
		return nil, err
	}
	src, _, err := source(ctx, s.db, orgID, projectID, assetID)
	if err != nil {
		return nil, err
	}
	switch {
	case !validAspect(aspect):
		return nil, invalid("aspect", "Choose 9:16, 1:1, 4:5 or 16:9.")
	case startMS < 0 || endMS <= startMS || endMS > src.DurationMS || endMS-startMS > MaxClip.Milliseconds():
		return nil, invalid("end_ms", "Choose a stretch within the video, at most 10 minutes long.")
	case math.IsNaN(focus) || focus < 0 || focus > 1:
		return nil, invalid("focus", "Framing must be between 0 and 1.")
	}
	t, ok, err := media.FramingTrack(ctx, s.db, orgID, assetID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, invalid("framing", "Auto framing isn’t ready for this video yet.")
	}
	return FramingPath(t, src, aspect, startMS, endMS, focus), nil
}

func validAspect(a string) bool { _, ok := shape(a); return ok }
