package framing

import (
	"math"
	"testing"
)

type fakeDetector struct{ boxes [][]Box }

func (f *fakeDetector) Detect([]byte) ([]Box, error) {
	b := f.boxes[0]
	if len(f.boxes) > 1 {
		f.boxes = f.boxes[1:]
	}
	return b, nil
}

func frame(w, h int, v byte) []byte {
	b := make([]byte, w*h*3)
	for i := range b {
		b[i] = v
	}
	return b
}

func TestFrameSize(t *testing.T) {
	for _, c := range []struct{ w, h, ww, wh int }{{1920, 1080, 480, 270}, {1080, 1920, 270, 480}, {640, 640, 480, 480}, {3840, 1600, 480, 200}} {
		if w, h := FrameSize(c.w, c.h); w != c.ww || h != c.wh {
			t.Errorf("FrameSize(%d, %d) = %d×%d, want %d×%d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}

func TestAnalyserFindsCutsAndFaces(t *testing.T) {
	w, h := 48, 27
	det := &fakeDetector{boxes: [][]Box{{{X: 12, Y: 4, W: 10, H: 12, Score: 0.9}, {X: 0, Y: 0, W: 0.5, H: 0.5, Score: 0.9}}, {}}}
	a := NewAnalyser(w, h, det)
	for i := range 12 {
		v := byte(20)
		if i >= 7 {
			v = 230 // a shot change between samples 6 and 7
		}
		if err := a.Frame(i, frame(w, h, v)); err != nil {
			t.Fatal(err)
		}
	}
	tr := a.Track()
	if len(tr.Cuts) != 1 || tr.Cuts[0] != 650 {
		t.Fatalf("cuts = %v, want [650]", tr.Cuts)
	}
	// Frames 0, 4 and 8 are searched; the tiny box is dropped.
	if len(tr.Faces) != 3 || len(tr.Faces[0]) != 1 || len(tr.Faces[1]) != 0 {
		t.Fatalf("faces = %+v", tr.Faces)
	}
	f := tr.Faces[0][0]
	if f.X != 0.354 || f.Y != 0.37 || f.Q != 0.9 || math.Abs(f.S-math.Sqrt(120)/27) > 0.001 {
		t.Fatalf("face = %+v", f)
	}
	if err := a.Frame(12, make([]byte, 5)); err == nil {
		t.Fatal("a wrong-sized frame was accepted")
	}
}

func TestParseFaces(t *testing.T) {
	b, err := parseFaces("2 10.0 20.0 30.0 40.0 0.910 1 2 3 4 0.6\n")
	if err != nil || len(b) != 2 || b[0] != (Box{10, 20, 30, 40, 0.91}) || b[1].Score != 0.6 {
		t.Fatalf("parseFaces = %+v, %v", b, err)
	}
	if b, err := parseFaces("0\n"); err != nil || len(b) != 0 {
		t.Fatalf("no faces = %+v, %v", b, err)
	}
	for _, bad := range []string{"", "x", "1 1 2 3", "2 1 2 3 4 5", "-1", "1 a b c d e"} {
		if _, err := parseFaces(bad); err == nil {
			t.Errorf("parseFaces(%q) accepted", bad)
		}
	}
}

// track makes a track with samples every 400 ms from fn.
func track(seconds float64, cuts []int64, fn func(ms int64) []Face) Track {
	tr := Track{Version: Version, IntervalMS: FaceIntervalMS, Cuts: cuts}
	for ms := int64(0); float64(ms) < seconds*1000; ms += FaceIntervalMS {
		tr.Faces = append(tr.Faces, fn(ms))
	}
	return tr
}

var vertical = Window{Size: 0.316} // 16:9 footage made 9:16

func want(x float64) float64 { return (x - vertical.Size/2) / (1 - vertical.Size) }

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// settled allows for the camera stopping once it is close (settle × the
// dead zone) rather than exactly on target.
func settled(a, b float64) bool { return math.Abs(a-b) < 0.02 }

func TestPathHoldsStillOnAStillSubject(t *testing.T) {
	tr := track(10, nil, func(int64) []Face { return []Face{{X: 0.7, Y: 0.4, S: 0.3, Q: 0.9}} })
	keys := tr.Path(1000, 9000, vertical, 0.5)
	if len(keys) != 2 || keys[0].AtMS != 0 || !near(keys[0].Focus, want(0.7)) || keys[1].Focus != keys[0].Focus {
		t.Fatalf("keys = %+v, want one steady focus near %.3f", keys, want(0.7))
	}
}

func TestPathIgnoresSmallMoves(t *testing.T) {
	// Swaying within the dead zone (a few % of the crop) moves nothing.
	tr := track(10, nil, func(ms int64) []Face {
		return []Face{{X: 0.5 + 0.01*math.Sin(float64(ms)/300), Y: 0.4, S: 0.3, Q: 0.9}}
	})
	keys := tr.Path(0, 10000, vertical, 0.5)
	for _, k := range keys {
		if k.Focus != keys[0].Focus {
			t.Fatalf("camera moved for a small sway: %+v", keys)
		}
	}
}

func TestPathFollowsAWalkSmoothly(t *testing.T) {
	// The subject walks from the left third to the right third over 4 s.
	tr := track(8, nil, func(ms int64) []Face {
		x := 0.25 + 0.5*min(max(float64(ms-2000)/4000, 0), 1)
		return []Face{{X: x, Y: 0.4, S: 0.3, Q: 0.9}}
	})
	keys := tr.Path(0, 8000, vertical, 0.5)
	if !near(FocusAt(keys, 0), want(0.25)) || !settled(FocusAt(keys, 7900), want(0.75)) {
		t.Fatalf("path from %.3f to %.3f, want %.3f to %.3f: %+v", FocusAt(keys, 0), FocusAt(keys, 7900), want(0.25), want(0.75), keys)
	}
	// No jumps: at most a fast glide between frames.
	for ms := int64(0); ms < 8000; ms += 33 {
		if d := math.Abs(FocusAt(keys, ms+33) - FocusAt(keys, ms)); d > 0.05 {
			t.Fatalf("jump of %.3f at %d ms", d, ms)
		}
	}
}

func TestPathCutsWithTheShot(t *testing.T) {
	// A shot change at 4 s moves the subject from left to right: the
	// camera cuts with it instead of panning.
	tr := track(8, []int64{4050}, func(ms int64) []Face {
		if ms < 4050 {
			return []Face{{X: 0.2, Y: 0.4, S: 0.3, Q: 0.9}}
		}
		return []Face{{X: 0.8, Y: 0.4, S: 0.3, Q: 0.9}}
	})
	keys := tr.Path(1000, 8000, vertical, 0.5)
	if !near(FocusAt(keys, 3000), want(0.2)) || !near(FocusAt(keys, 3050), want(0.8)) || !near(FocusAt(keys, 3000-40), want(0.2)) {
		t.Fatalf("no clean cut at 3050 ms: %+v", keys)
	}
}

func TestPathFallsBackWithoutFaces(t *testing.T) {
	tr := track(6, []int64{3000}, func(ms int64) []Face {
		if ms < 3000 {
			return nil
		}
		return []Face{{X: 0.9, Y: 0.4, S: 0.3, Q: 0.9}}
	})
	keys := tr.Path(0, 6000, vertical, 0.3)
	if FocusAt(keys, 1000) != 0.3 || !near(FocusAt(keys, 5000), 1) {
		t.Fatalf("keys = %+v", keys)
	}
	// No room to move, or nothing known: one key.
	if k := tr.Path(0, 6000, Window{Size: 1}, 0.3); len(k) != 1 || k[0] != (Key{0, 0.3}) {
		t.Fatalf("full-width crop = %+v", k)
	}
	if k := (Track{}).Path(0, 6000, vertical, 0.6); len(k) != 1 || k[0].Focus != 0.6 {
		t.Fatalf("empty track = %+v", k)
	}
}

func TestPathKeepsItsSubject(t *testing.T) {
	// The subject (left) stays the subject when someone bigger passes
	// through for a second, but a bigger face that stays takes over.
	tr := track(12, nil, func(ms int64) []Face {
		f := []Face{{X: 0.2, Y: 0.4, S: 0.2, Q: 0.9}}
		if ms >= 2000 && ms < 3000 || ms >= 6000 {
			f = append(f, Face{X: 0.8, Y: 0.4, S: 0.4, Q: 0.9})
		}
		return f
	})
	keys := tr.Path(0, 12000, vertical, 0.5)
	if !near(FocusAt(keys, 3500), want(0.2)) {
		t.Fatalf("a passer-by took over: %+v", keys)
	}
	if !settled(FocusAt(keys, 11000), want(0.8)) {
		t.Fatalf("the bigger face never took over: %+v", keys)
	}
}

func TestPathDropsOneOffDetections(t *testing.T) {
	tr := track(6, nil, func(ms int64) []Face {
		f := []Face{{X: 0.3, Y: 0.4, S: 0.2, Q: 0.9}}
		if ms == 2000 {
			f = append(f, Face{X: 0.9, Y: 0.1, S: 0.6, Q: 0.7}) // a poster, once
		}
		return f
	})
	for _, k := range tr.Path(0, 6000, vertical, 0.5) {
		if !near(k.Focus, want(0.3)) {
			t.Fatalf("followed a one-off detection: %+v", k)
		}
	}
}

func TestPathVertical(t *testing.T) {
	// A tall source in a wide frame moves up and down, keeping headroom.
	tr := track(4, nil, func(int64) []Face { return []Face{{X: 0.5, Y: 0.3, S: 0.2, Q: 0.9}} })
	win := Window{Size: 0.32, Vertical: true}
	got := FocusAt(tr.Path(0, 4000, win, 0.5), 1000)
	if !near(got, (0.3-0.4*0.32)/(1-0.32)) {
		t.Fatalf("vertical focus = %.3f", got)
	}
}

func TestPathJumpsIntoAShotWithoutFaces(t *testing.T) {
	// A face on the left, then (after a cut at 3 s) a shot of scenery:
	// the camera jumps to the fallback at the cut instead of sliding.
	tr := track(6, []int64{3000}, func(ms int64) []Face {
		if ms < 3000 {
			return []Face{{X: 0.2, Y: 0.4, S: 0.3, Q: 0.9}}
		}
		return nil
	})
	keys := tr.Path(0, 6000, vertical, 0.5)
	if !near(FocusAt(keys, 2990), want(0.2)) || FocusAt(keys, 3000) != 0.5 || FocusAt(keys, 4500) != 0.5 {
		t.Fatalf("keys = %+v", keys)
	}
}
