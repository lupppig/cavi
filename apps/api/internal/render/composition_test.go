package render

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
)

var landscape = Source{Width: 1920, Height: 1080, DurationMS: 120_000, HasAudio: true}

func comp(start, end int64, aspect string, focus float64) Composition {
	return Composition{Version: 1, SourceAssetID: "x", StartMS: start, EndMS: end, Aspect: aspect, Focus: focus}
}

func TestPlan(t *testing.T) {
	cases := []struct {
		name string
		c    Composition
		src  Source
		want ffmpeg.Clip
	}{
		{"landscape to vertical, centred", comp(10_000, 40_000, "9:16", 0.5), landscape, ffmpeg.Clip{
			Start: 10 * time.Second, Length: 30 * time.Second, CropX: 656, CropW: 608, CropH: 1080, Width: 1080, Height: 1920, Audio: true}},
		{"landscape to vertical, left edge", comp(0, 5000, "9:16", 0), landscape, ffmpeg.Clip{
			Length: 5 * time.Second, CropW: 608, CropH: 1080, Width: 1080, Height: 1920, Audio: true}},
		{"landscape to vertical, right edge", comp(0, 5000, "9:16", 1), landscape, ffmpeg.Clip{
			Length: 5 * time.Second, CropX: 1312, CropW: 608, CropH: 1080, Width: 1080, Height: 1920, Audio: true}},
		{"landscape stays landscape", comp(0, 5000, "16:9", 0.5), landscape, ffmpeg.Clip{
			Length: 5 * time.Second, CropW: 1920, CropH: 1080, Width: 1920, Height: 1080, Audio: true}},
		{"portrait phone to 1:1, top third", comp(0, 5000, "1:1", 0.25),
			Source{Width: 1080, Height: 1920, DurationMS: 9000}, ffmpeg.Clip{
				Length: 5 * time.Second, CropY: 210, CropW: 1080, CropH: 1080, Width: 1080, Height: 1080}},
		{"odd-sized source to 4:5", comp(0, 5000, "4:5", 0.5),
			Source{Width: 641, Height: 361, DurationMS: 9000, HasAudio: true}, ffmpeg.Clip{
				Length: 5 * time.Second, CropX: 177, CropW: 288, CropH: 360, Width: 1080, Height: 1350, Audio: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.c.Validate(tc.src); err != nil {
				t.Fatal(err)
			}
			got := Plan(tc.c, tc.src)
			if got != tc.want {
				t.Fatalf("plan = %+v\nwant   %+v", got, tc.want)
			}
			// The crop stays inside the source, is even, and has the
			// output's shape to within a pixel.
			if got.CropX < 0 || got.CropY < 0 || got.CropX+got.CropW > tc.src.Width || got.CropY+got.CropH > tc.src.Height ||
				got.CropW%2 != 0 || got.CropH%2 != 0 {
				t.Fatalf("crop out of bounds or odd: %+v", got)
			}
			want := float64(got.Width) / float64(got.Height)
			if math.Abs(float64(got.CropW)/float64(got.CropH)-want)*float64(got.CropH) > 2 {
				t.Errorf("crop aspect %d:%d is not %s", got.CropW, got.CropH, tc.c.Aspect)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	for field, c := range map[string]Composition{
		"composition.version":  {Version: 2, StartMS: 0, EndMS: 5000, Aspect: "9:16"},
		"composition.start_ms": comp(-1, 5000, "9:16", 0.5),
		"composition.end_ms":   comp(0, 500, "9:16", 0.5),
		"composition.aspect":   comp(0, 5000, "21:9", 0.5),
		"composition.focus":    comp(0, 5000, "9:16", 1.5),
	} {
		var v *account.ValidationError
		if err := c.Validate(landscape); !errors.As(err, &v) || v.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}
	var v *account.ValidationError
	if err := comp(0, 5000, "9:16", math.NaN()).Validate(landscape); !errors.As(err, &v) || v.Field != "composition.focus" {
		t.Errorf("NaN focus: %v", err)
	}
	if err := comp(119_000, 121_000, "9:16", 0.5).Validate(landscape); !errors.As(err, &v) || v.Field != "composition.end_ms" {
		t.Errorf("past the end: %v", err)
	}
	if err := comp(130_000, 131_000, "9:16", 0.5).Validate(landscape); !errors.As(err, &v) || v.Field != "composition.start_ms" {
		t.Errorf("starts after the end: %v", err)
	}
	long := Source{Width: 1920, Height: 1080, DurationMS: 3_600_000}
	if err := comp(0, 601_000, "9:16", 0.5).Validate(long); !errors.As(err, &v) || v.Field != "composition.end_ms" {
		t.Errorf("too long: %v", err)
	}
	if err := comp(0, 5000, "9:16", 0.5).Validate(Source{DurationMS: 9000}); !errors.As(err, &v) || v.Field != "composition.source_asset_id" {
		t.Errorf("audio-only source: %v", err)
	}
}
