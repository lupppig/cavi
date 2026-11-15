package framing

import (
	"math"
	"sort"
)

// Window is the crop, seen along the axis it moves on.
type Window struct {
	// Size is the crop's length along the axis as a fraction of the
	// frame's (0 to 1]: 0.316 for 16:9 footage made 9:16.
	Size float64
	// Vertical means the crop moves up and down (a tall source in a wider
	// output) rather than left and right.
	Vertical bool
}

// Key is a point on a camera path: AtMS after the stretch starts, the
// crop sits at Focus, with the same meaning as a composition's focus (0
// the left or top edge, 1 the right or bottom). The crop moves in a
// straight line from one key to the next.
type Key struct {
	AtMS  int64   `json:"at_ms"`
	Focus float64 `json:"focus"`
}

// How the camera behaves.
const (
	stepMS = 1000.0 / 30
	// deadZone is how far (as a fraction of the crop) the subject may drift
	// from where the camera aims before it moves: small moves are ignored.
	deadZone = 0.12
	// The camera glides at up to maxSpeed crops a second, easing in to the
	// subject (closing easeRate of the gap a second), and stops once within
	// settle×deadZone.
	maxSpeed = 1.0
	easeRate = 2.5
	settle   = 0.25
	// Another face takes over when it is switchRatio times the subject's
	// size for switchSamples samples in a row (2 s); the subject may be
	// missed for holdSamples samples before a new one is chosen.
	switchRatio   = 1.4
	switchSamples = 5
	holdSamples   = 3
	// matchDist is how far (fraction of the frame) a face may move between
	// samples and still be the same person.
	matchDist = 0.12
	// verticalAnchor places a followed face 40% of the way down a crop
	// that moves vertically, leaving headroom.
	verticalAnchor = 0.4
	// minShotMS: shot changes closer together than this are merged.
	minShotMS = 300
	// tolerance is how far (in focus) the simplified path may stray from
	// the simulated one.
	tolerance = 0.003
)

// Path is the camera path for startMS to endMS (source time) with a crop
// of win. Shots without faces sit at fallback (the composition's own
// focus). It always returns at least one key, the first at 0.
func (t Track) Path(startMS, endMS int64, win Window, fallback float64) []Key {
	fallback = clamp01(fallback)
	if win.Size <= 0 || win.Size >= 0.995 || endMS <= startMS || t.IntervalMS <= 0 {
		return []Key{{AtMS: 0, Focus: round4(fallback)}}
	}
	var keys []Key
	for _, sh := range t.shots(startMS, endMS) {
		pts := t.shotPath(sh[0], sh[1], win, fallback)
		for _, p := range simplify(pts, tolerance) {
			keys = append(keys, Key{AtMS: p.at - startMS, Focus: round4(p.focus)})
		}
	}
	return dedupe(keys)
}

// FocusAt is where a path puts the crop at atMS.
func FocusAt(keys []Key, atMS int64) float64 {
	if len(keys) == 0 {
		return 0.5
	}
	i := sort.Search(len(keys), func(i int) bool { return keys[i].AtMS > atMS })
	switch {
	case i == 0:
		return keys[0].Focus
	case i == len(keys):
		return keys[len(keys)-1].Focus
	}
	a, b := keys[i-1], keys[i]
	return a.Focus + (b.Focus-a.Focus)*float64(atMS-a.AtMS)/float64(b.AtMS-a.AtMS)
}

// shots splits [startMS, endMS) at shot changes.
func (t Track) shots(startMS, endMS int64) [][2]int64 {
	var out [][2]int64
	from := startMS
	for _, c := range t.Cuts {
		if c-from >= minShotMS && endMS-c >= minShotMS {
			out = append(out, [2]int64{from, c})
			from = c
		}
	}
	return append(out, [2]int64{from, endMS})
}

type point struct {
	at    int64
	focus float64
}

// shotPath simulates the camera through one shot, a point a frame.
func (t Track) shotPath(a, b int64, win Window, fallback float64) []point {
	first := int((a + t.IntervalMS - 1) / t.IntervalMS)
	last := int((b - 1) / t.IntervalMS)
	if last >= len(t.Faces) {
		last = len(t.Faces) - 1
	}
	if first > last {
		// Too short for a sample of its own: use the nearest one inside it,
		// if any (none means no faces known).
		if mid := int((a + b) / 2 / t.IntervalMS); mid < len(t.Faces) && int64(mid)*t.IntervalMS >= a && int64(mid)*t.IntervalMS < b {
			first, last = mid, mid
		}
	}
	var targets []float64
	if first <= last {
		targets = t.targets(first, last, win)
	}
	if targets == nil {
		return []point{{a, fallback}, {b - 1, fallback}}
	}
	at := func(ms float64) float64 {
		// Targets are at sample times; between them, interpolate.
		x := ms/float64(t.IntervalMS) - float64(first)
		if x <= 0 {
			return targets[0]
		}
		k := int(x)
		if k >= len(targets)-1 {
			return targets[len(targets)-1]
		}
		f := x - float64(k)
		return targets[k]*(1-f) + targets[k+1]*f
	}

	room := (1 - win.Size) / win.Size // focus units per crop
	dz, vmax := deadZone/room, maxSpeed/room
	p, moving := at(float64(a)), false
	var pts []point
	for ms := float64(a); ms < float64(b); ms += stepMS {
		d := at(ms) - p
		if !moving && math.Abs(d) > dz {
			moving = true
		}
		if moving {
			step := min(vmax, easeRate*math.Abs(d)) * stepMS / 1000
			p += math.Copysign(min(step, math.Abs(d)), d)
			if math.Abs(at(ms)-p) < dz*settle {
				moving = false
			}
		}
		pts = append(pts, point{int64(math.Round(ms)), clamp01(p)})
	}
	return pts
}

// targets are where the camera should aim at each sample from first to
// last (inclusive) to frame the shot's subject, smoothed; nil when the
// shot has no faces.
func (t Track) targets(first, last int, win Window) []float64 {
	n := last - first + 1
	pos := make([]float64, n)
	var cur *Face
	missing, streak := 0, 0
	found := false
	for i := range n {
		faces := t.steady(first+i, first, last)
		match := nearest(faces, cur)
		if match != nil {
			cur, missing = match, 0
		} else if cur != nil {
			if missing++; missing > holdSamples {
				cur = nil
			}
		}
		if cur == nil {
			cur = biggest(faces, nil)
			streak = 0
		} else if ch := biggest(faces, match); ch != nil && match != nil && ch.S > switchRatio*match.S {
			if streak++; streak >= switchSamples {
				cur, streak = ch, 0
			}
		} else {
			streak = 0
		}
		pos[i] = math.NaN()
		if cur != nil {
			found = true
			c := cur.X
			anchor := 0.5
			if win.Vertical {
				c, anchor = cur.Y, verticalAnchor
			}
			pos[i] = clamp01((c - anchor*win.Size) / (1 - win.Size))
		}
	}
	if !found {
		return nil
	}
	// Hold the last known aim through gaps; before the first, use it.
	last1 := math.NaN()
	for i := range pos {
		if math.IsNaN(pos[i]) {
			pos[i] = last1
		} else {
			last1 = pos[i]
		}
	}
	for i := n - 1; i >= 0; i-- {
		if math.IsNaN(pos[i]) {
			pos[i] = pos[i+1]
		}
	}
	return median5(pos)
}

// steady is sample i's faces that also appear (nearby) in a neighbouring
// sample of the shot, which drops one-off false detections.
func (t Track) steady(i, first, last int) []Face {
	if first == last {
		return t.Faces[i]
	}
	var out []Face
	for _, f := range t.Faces[i] {
		for _, j := range []int{i - 1, i + 1} {
			if j < first || j > last {
				continue
			}
			if nearest(t.Faces[j], &f) != nil {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// nearest is the face in faces closest to f, if within matchDist.
func nearest(faces []Face, f *Face) *Face {
	if f == nil {
		return nil
	}
	var best *Face
	bd := matchDist
	for i := range faces {
		if d := math.Hypot(faces[i].X-f.X, faces[i].Y-f.Y); d <= bd {
			best, bd = &faces[i], d
		}
	}
	if best != nil {
		c := *best
		return &c
	}
	return nil
}

// biggest is the largest face other than not.
func biggest(faces []Face, not *Face) *Face {
	var best *Face
	for i := range faces {
		if not != nil && faces[i] == *not {
			continue
		}
		if best == nil || faces[i].S > best.S {
			best = &faces[i]
		}
	}
	if best != nil {
		c := *best
		return &c
	}
	return nil
}

func median5(v []float64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		w := v[max(0, i-2):min(len(v), i+3)]
		s := append([]float64(nil), w...)
		sort.Float64s(s)
		out[i] = s[len(s)/2]
	}
	return out
}

// simplify drops points the path can do without (Ramer–Douglas–Peucker),
// keeping the first and last.
func simplify(pts []point, tol float64) []point {
	if len(pts) <= 2 {
		return pts
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	var rec func(i, j int)
	rec = func(i, j int) {
		far, fi := 0.0, -1
		for k := i + 1; k < j; k++ {
			f := pts[i].focus + (pts[j].focus-pts[i].focus)*float64(pts[k].at-pts[i].at)/float64(pts[j].at-pts[i].at)
			if d := math.Abs(pts[k].focus - f); d > far {
				far, fi = d, k
			}
		}
		if far > tol {
			keep[fi] = true
			rec(i, fi)
			rec(fi, j)
		}
	}
	rec(0, len(pts)-1)
	var out []point
	for i, p := range pts {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// dedupe drops keys that add nothing: repeats of the same time, and the
// middle of three equal focuses in a row.
func dedupe(keys []Key) []Key {
	var out []Key
	for _, k := range keys {
		if n := len(out); n > 0 && out[n-1].AtMS >= k.AtMS {
			continue
		}
		if n := len(out); n >= 2 && out[n-1].Focus == k.Focus && out[n-2].Focus == k.Focus {
			out[n-1] = k
			continue
		}
		out = append(out, k)
	}
	return out
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
