package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/ffmpeg"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/media"
)

func montageOf(segs ...Segment) Composition {
	return Composition{Version: MontageVersion, Aspect: "9:16", Segments: segs}
}

func TestValidateMontage(t *testing.T) {
	srcs := map[string]Source{
		"a": {Width: 1920, Height: 1080, DurationMS: 60_000, HasAudio: true},
		"b": {Width: 1280, Height: 720, DurationMS: 10_000},
	}
	ok := montageOf(
		Segment{SourceAssetID: "a", StartMS: 1000, EndMS: 5000, Focus: 0.5,
			Mute: []Range{{StartMS: 2000, EndMS: 3000}},
			Blur: []BlurBox{{Range: Range{StartMS: 1000, EndMS: 2000}, X: 0.1, Y: 0.6, W: 0.2, H: 0.1}}},
		Segment{SourceAssetID: "b", StartMS: 0, EndMS: 3000, Focus: 0.3},
	)
	ok.Hook = &Hook{Text: "  Diamonds   built this ", EndMS: 2500}
	if err := ok.ValidateMontage(srcs); err != nil {
		t.Fatal(err)
	}
	if ok.LengthMS() != 7000 || ok.SourceAssetID != "a" || ok.StartMS != 1000 || ok.Hook.Text != "Diamonds built this" {
		t.Fatalf("validated = %+v", ok)
	}
	if ids := ok.SourceIDs(); len(ids) != 2 || ids[0] != "a" {
		t.Fatalf("SourceIDs = %v", ids)
	}

	cases := map[string]func(c *Composition){
		"composition.segments":                    func(c *Composition) { c.Segments = nil },
		"composition.segments[0].end_ms":          func(c *Composition) { c.Segments[0].EndMS = 1200 },
		"composition.segments[1].end_ms":          func(c *Composition) { c.Segments[1].EndMS = 11_000 },
		"composition.segments[1].source_asset_id": func(c *Composition) { c.Segments[1].SourceAssetID = "elsewhere" },
		"composition.segments[0].mute[0]":         func(c *Composition) { c.Segments[0].Mute[0].EndMS = 9000 },
		"composition.segments[0].blur[0]":         func(c *Composition) { c.Segments[0].Blur[0].X = 0.95 },
		"composition.hook.end_ms":                 func(c *Composition) { c.Hook.EndMS = 9000 },
		"composition.hook.text":                   func(c *Composition) { c.Hook.Text = strings.Repeat("x", 121) },
		"composition.aspect":                      func(c *Composition) { c.Aspect = "2:1" },
	}
	for field, mutate := range cases {
		c := montageOf(
			Segment{SourceAssetID: "a", StartMS: 1000, EndMS: 5000, Focus: 0.5, Mute: []Range{{StartMS: 2000, EndMS: 3000}},
				Blur: []BlurBox{{Range: Range{StartMS: 1000, EndMS: 2000}, X: 0.1, Y: 0.6, W: 0.2, H: 0.1}}},
			Segment{SourceAssetID: "b", StartMS: 0, EndMS: 3000},
		)
		c.Hook = &Hook{Text: "hook", EndMS: 2000}
		mutate(&c)
		wantField(t, c.ValidateMontage(srcs), field)
	}
	// Single-clip versions can't carry montage fields.
	v1 := Composition{Version: 1, SourceAssetID: "a", StartMS: 0, EndMS: 5000, Aspect: "9:16", Hook: &Hook{Text: "x", EndMS: 1000}}
	wantField(t, v1.Validate(srcs["a"]), "composition.version")
}

func TestPlanMontage(t *testing.T) {
	srcs := map[string]Source{"a": {Width: 1920, Height: 1080, DurationMS: 60_000, HasAudio: true}, "b": {Width: 1280, Height: 720, DurationMS: 10_000}}
	c := montageOf(
		Segment{SourceAssetID: "a", StartMS: 1000, EndMS: 5000, Focus: 0.5, Mute: []Range{{StartMS: 2000, EndMS: 3000}},
			Blur: []BlurBox{{Range: Range{StartMS: 1500, EndMS: 2500}, X: 0.5, Y: 0.5, W: 0.25, H: 0.1}}},
		Segment{SourceAssetID: "b", StartMS: 0, EndMS: 3000, Focus: 0},
	)
	m := PlanMontage(c, srcs, map[string]string{"a": "https://a", "b": "https://b"}, nil)
	if m.Width != 1080 || m.Height != 1920 || len(m.Parts) != 2 || m.Length() != 7*time.Second {
		t.Fatalf("plan = %+v", m)
	}
	a, b := m.Parts[0], m.Parts[1]
	if a.Input != "https://a" || a.Start != time.Second || a.CropW != 608 || a.CropH != 1080 || a.CropX != 656 || !a.Audio {
		t.Errorf("part a = %+v", a)
	}
	if len(a.Mute) != 1 || a.Mute[0].From != time.Second || a.Mute[0].To != 2*time.Second {
		t.Errorf("mute = %+v (times are from the part's start)", a.Mute)
	}
	if bl := a.Blur[0]; bl.X != 960 || bl.Y != 540 || bl.W != 480 || bl.H != 108 || bl.From != 500*time.Millisecond {
		t.Errorf("blur = %+v (source pixels)", bl)
	}
	if b.Audio || b.CropX != 0 || b.CropH != 720 {
		t.Errorf("part b = %+v", b)
	}
}

func TestMontageCaptionsAndOverlay(t *testing.T) {
	tr := map[string][]ai.Segment{
		"a": {{StartMS: 1000, EndMS: 3000, Speaker: "Speaker 1", Text: "Diamonds pay for schools"}},
		"b": {{StartMS: 0, EndMS: 2000, Speaker: "Speaker 1", Text: "Look at that elephant"}},
	}
	cues := MontageCaptions([]Segment{{SourceAssetID: "a", StartMS: 1000, EndMS: 3000}, {SourceAssetID: "b", StartMS: 0, EndMS: 2000}}, tr, "9:16", ModeLines)
	if len(cues) < 2 || cues[0].StartMS != 0 || cues[len(cues)-1].EndMS > 4000 || cues[len(cues)-1].StartMS < 2000 {
		t.Fatalf("cues = %+v", cues)
	}
	ass := OverlayASS(nil, &Hook{Text: "How diamonds built Botswana", EndMS: 2000}, nil, 0, 0, "9:16", "")
	if !strings.Contains(ass, "Style: Hook,") || !strings.Contains(ass, "Dialogue: 1,0:00:00.00,0:00:02.00,Hook,,0,0,0,,{\\q0}How diamonds built Botswana") {
		t.Fatalf("hook overlay:\n%s", ass)
	}
	caps := &Captions{CaptionOptions: CaptionOptions{Style: CaptionsBoxed}, Cues: cues}
	both := OverlayASS(caps, &Hook{Text: "Hook", EndMS: 1000}, nil, 0, 0, "9:16", "")
	if strings.Index(both, "Style: Hook") > strings.Index(both, "[Events]") || !strings.Contains(both, ",Cap,") {
		t.Fatalf("captions and hook:\n%s", both)
	}
	if OverlayASS(nil, nil, nil, 0, 0, "9:16", "") != "" {
		t.Fatal("empty overlay")
	}
	badge := OverlayASS(nil, nil, &SignOff{Text: "Follow for Part 2"}, 20_000, 22_500, "9:16", "PART 1/3")
	if !strings.Contains(badge, "Style: Badge,Inter Black,45,") || !strings.Contains(badge, "Dialogue: 3,0:00:00.00,0:00:20.00,Badge,,0,0,0,,PART 1/3") {
		t.Fatalf("badge overlay:\n%s", badge)
	}
	if OverlayASS(nil, nil, nil, 0, 0, "9:16", "") != "" {
		t.Fatal("empty overlay should be empty")
	}
}

func TestRenderMontage(t *testing.T) {
	w := newWorld(t)
	ana, vic := w.user("Ana"), w.user("Vic")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)
	talk := w.video(org, proj, "video", 1920, 1080, 600_000, true)
	broll := w.video(org, proj, "video", 1280, 720, 30_000, false)
	w.transcribe(org, talk, [3]any{90_000, 95_000, "Diamonds pay for the schools here"})
	w.transcribe(org, broll)
	w.briefRules(org, proj, talk,
		ai.BriefRule{Kind: ai.RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"},
		ai.BriefRule{Kind: ai.RuleBlur, StartMS: 91_000, EndMS: 91_000, Note: "Licence plate"})

	c := montageOf(
		Segment{SourceAssetID: talk, StartMS: 90_000, EndMS: 95_000, Focus: 0.5,
			Blur: []BlurBox{{Range: Range{StartMS: 90_500, EndMS: 92_000}, X: 0.4, Y: 0.7, W: 0.2, H: 0.08}}},
		Segment{SourceAssetID: broll, StartMS: 2000, EndMS: 5000, Focus: 0.5},
	)
	c.Hook = &Hook{Text: "Diamonds built this school", EndMS: 2500}
	opts := &CaptionOptions{Style: CaptionsBoxed}

	if _, err := w.svc.Create(w.ctx, vic.id, org, proj, "Montage", c, opts); err == nil {
		t.Fatal("viewer made a montage")
	}
	bad := c
	bad.Segments = append([]Segment{{SourceAssetID: talk, StartMS: 45_000, EndMS: 50_000}}, c.Segments...)
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Bad", bad, nil)
	wantField(t, err, "composition.segments[0].start_ms")
	vicsOrg := w.team(vic, nil)
	other := w.video(vicsOrg, w.project(vic, vicsOrg), "video", 640, 360, 10_000, false)
	cross := c
	cross.Segments = []Segment{c.Segments[0], {SourceAssetID: other, StartMS: 0, EndMS: 2000}}
	if _, err := w.svc.Create(w.ctx, ana.id, org, proj, "Cross", cross, nil); err == nil {
		t.Fatal("montage used another tenant's video")
	}

	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Montage", c, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Composition.Version != MontageVersion || r.SourceAssetID != talk || r.Composition.Captions == nil || len(r.Flags) != 1 || r.Flags[0].Kind != ai.RuleBlur {
		t.Fatalf("render = %+v", r)
	}
	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	if len(ff.montages) != 1 || len(ff.montages[0].Parts) != 2 || len(ff.montages[0].Parts[0].Blur) != 1 || ff.montages[0].Parts[1].Audio {
		t.Fatalf("montages = %+v", ff.montages)
	}
	// Multi-segment montages read local copies of their sources, removed
	// after the render.
	for _, p := range ff.montages[0].Parts {
		if !strings.HasPrefix(p.Input, "/") {
			t.Fatalf("part input %q is not a local copy", p.Input)
		}
		if _, err := os.Stat(p.Input); !os.IsNotExist(err) {
			t.Fatalf("local copy %s left behind: %v", p.Input, err)
		}
	}
	if len(ff.ass) != 1 || !strings.Contains(ff.ass[0], "Diamonds built this school") || !strings.Contains(ff.ass[0], ",Cap,") {
		t.Fatalf("overlay = %v", ff.ass)
	}
	got, _ := w.svc.Get(w.ctx, ana.id, org, proj, r.ID)
	if got.Status != StatusReady || got.DurationMS < 7900 || got.DurationMS > 8100 {
		t.Fatalf("finished render = %s %d ms", got.Status, got.DurationMS)
	}

	// Deleting any source fails a queued montage for good.
	r2, err := w.svc.Create(w.ctx, ana.id, org, proj, "Again", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, broll); err != nil {
		t.Fatal(err)
	}
	if err := w.renderer(ff).Process(w.ctx, w.job(r2.ID)); !jobs.IsPermanent(err) {
		t.Fatalf("montage with a deleted source: %v", err)
	}
}

// readyVoiceover adds a finished voiceover of durationMS to the project.
func (w *world) readyVoiceover(org, proj string, durationMS int64) string {
	w.t.Helper()
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO voiceovers (organisation_id, project_id, script, voice, status, provider, model, prompt_version, storage_key, duration_ms, completed_at)
		VALUES (?, ?, 'Diamonds paid for this school.', 'Kore', 'ready', 'gemini', 'gemini-3.8-flash-tts', 'voiceover-v1', ?, ?, now()) RETURNING id`,
		org, proj, "vo/"+proj+"/"+time.Now().Format("150405.000000"), durationMS).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func TestRenderMontageWithVoiceover(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 60_000, true)
	vo := w.readyVoiceover(org, proj, 4000)
	c := montageOf(Segment{SourceAssetID: src, StartMS: 10_000, EndMS: 16_000, Focus: 0.5})

	c.Voiceover = &VoiceoverMix{VoiceoverID: vo, StartMS: 3000, Duck: 0.25}
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Too long", c, nil)
	wantField(t, err, "composition.voiceover.start_ms")
	c.Voiceover = &VoiceoverMix{VoiceoverID: newID(), StartMS: 0, Duck: 0.25}
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Missing", c, nil)
	wantField(t, err, "composition.voiceover.voiceover_id")
	c.Voiceover = &VoiceoverMix{VoiceoverID: vo, StartMS: 1000, Duck: 1.5}
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Loud", c, nil)
	wantField(t, err, "composition.voiceover.duck")

	c.Voiceover = &VoiceoverMix{VoiceoverID: vo, StartMS: 1500, Duck: 0.25, DurationMS: 99}
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "With voiceover", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Composition.Voiceover.DurationMS != 4000 {
		t.Fatalf("duration not copied from the voiceover: %+v", r.Composition.Voiceover)
	}
	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	m := ff.montages[0]
	if len(m.Voices) != 1 || m.Voices[0].Input == "" || m.Voices[0].At != 1500*time.Millisecond || m.Voices[0].Length != 4*time.Second || m.Voices[0].Duck != 0.25 {
		t.Fatalf("plan voices = %+v", m.Voices)
	}

	r2, err := w.svc.Create(w.ctx, ana.id, org, proj, "Again", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM voiceovers WHERE id = ?`, vo); err != nil {
		t.Fatal(err)
	}
	j := w.job(r2.ID)
	err = w.renderer(ff).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("deleted voiceover: %v", err)
	}
	_ = w.renderer(ff).Failed(w.ctx, j, err)
	if got, _ := w.svc.Get(w.ctx, ana.id, org, proj, r2.ID); got.Error != msgVoiceoverGone {
		t.Fatalf("message = %q", got.Error)
	}
}

func TestVoiceoverCaptions(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 60_000, true)
	w.transcribe(org, src, [3]any{10_000, 14_000, "People talk over the opening"}, [3]any{20_000, 24_000, "and later about schools"})
	vo := w.readyVoiceover(org, proj, 5000)
	if _, err := w.db.ExecContext(w.ctx, `UPDATE voiceovers SET words = ?::jsonb WHERE id = ?`,
		`[{"start_ms":0,"end_ms":1500,"text":"Diamonds"},{"start_ms":1500,"end_ms":3000,"text":"paid"},{"start_ms":3000,"end_ms":5000,"text":"here."}]`, vo); err != nil {
		t.Fatal(err)
	}
	c := montageOf(Segment{SourceAssetID: src, StartMS: 10_000, EndMS: 25_000, Focus: 0.5})
	c.Voiceover = &VoiceoverMix{VoiceoverID: vo, StartMS: 500, Duck: 0.2}
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Captioned voiceover", c, &CaptionOptions{Style: CaptionsBoxed, Mode: ModeWords})
	if err != nil {
		t.Fatal(err)
	}
	var voText, laterText bool
	for _, q := range r.Composition.Captions.Cues {
		if q.StartMS < 5500 && q.EndMS > 500 && strings.Contains(q.Text, "People") {
			t.Fatalf("dialogue captioned under the voiceover: %+v", q)
		}
		voText = voText || (strings.Contains(q.Text, "Diamonds") && q.StartMS == 500)
		laterText = laterText || strings.Contains(q.Text, "schools")
	}
	if !voText || !laterText {
		t.Fatalf("cues = %+v", r.Composition.Captions.Cues)
	}
}

func TestRenderMontageWithSignOff(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 60_000, true)
	w.transcribe(org, src, [3]any{10_000, 20_000, "People talk right through to the very end of this clip"})
	intro := w.readyVoiceover(org, proj, 3000)
	outro := w.readyVoiceover(org, proj, 2000)
	c := montageOf(Segment{SourceAssetID: src, StartMS: 10_000, EndMS: 20_000, Focus: 0.5})

	c.SignOff = &SignOff{Text: "   "}
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Empty", c, nil)
	wantField(t, err, "composition.sign_off.text")
	c.SignOff = &SignOff{Text: "Follow for more", VoiceoverID: newID()}
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Missing voice", c, nil)
	wantField(t, err, "composition.sign_off.voiceover_id")
	// The intro would still be playing when the 2.5 s sign-off starts.
	c.SignOff = &SignOff{Text: "Follow for more", VoiceoverID: outro}
	c.Voiceover = &VoiceoverMix{VoiceoverID: intro, StartMS: 5000, Duck: 0.2}
	_, err = w.svc.Create(w.ctx, ana.id, org, proj, "Overlap", c, nil)
	wantField(t, err, "composition.voiceover.start_ms")

	c.Voiceover.StartMS = 500
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Signed off", c, &CaptionOptions{Style: CaptionsBold})
	if err != nil {
		t.Fatal(err)
	}
	so := r.Composition.SignOff
	if so.VoiceDurationMS != 2000 || r.Composition.SignOffStartMS() != 7500 {
		t.Fatalf("sign-off = %+v starts %d", so, r.Composition.SignOffStartMS())
	}
	for _, q := range r.Composition.Captions.Cues {
		if q.EndMS > 7500 {
			t.Fatalf("caption runs under the sign-off card: %+v", q)
		}
	}
	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	m := ff.montages[0]
	if !strings.HasPrefix(m.Parts[0].Input, "https://") {
		t.Fatalf("a single-segment montage should stream its source, got %q", m.Parts[0].Input)
	}
	if len(m.Voices) != 2 || m.Voices[1].At != 7500*time.Millisecond || m.Voices[1].Length != 2*time.Second || m.Voices[1].Duck != signOffDuck {
		t.Fatalf("plan voices = %+v", m.Voices)
	}

	// Silent: a fixed-length card.
	c.SignOff = &SignOff{Text: "Like and subscribe"}
	c.Voiceover = nil
	r, err = w.svc.Create(w.ctx, ana.id, org, proj, "Silent sign-off", c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Composition.SignOffStartMS() != 10_000-SignOffMS {
		t.Fatalf("silent sign-off starts %d", r.Composition.SignOffStartMS())
	}
	ass := OverlayASS(nil, nil, r.Composition.SignOff, 7500, 10_000, "9:16", "")
	if !strings.Contains(ass, "Style: SignOff") || !strings.Contains(ass, "Dialogue: 2,0:00:07.50,0:00:10.00,SignOff") || !strings.Contains(ass, "Like and subscribe") {
		t.Fatalf("overlay = %s", ass)
	}
}

func TestBeforeSignOff(t *testing.T) {
	cues := []Cue{
		{StartMS: 0, EndMS: 1000, Text: "a", Words: []CueWord{{0, 1000, "a"}}},
		{StartMS: 1500, EndMS: 3000, Text: "b c", Words: []CueWord{{1500, 2000, "b"}, {2600, 3000, "c"}}},
		{StartMS: 3000, EndMS: 4000, Text: "d", Words: []CueWord{{3000, 4000, "d"}}},
	}
	got := BeforeSignOff(cues, 2500)
	if len(got) != 2 || got[1].EndMS != 2500 || got[1].Words[1].StartMS != 2500 || got[1].Words[1].EndMS != 2500 {
		t.Fatalf("got %+v", got)
	}
	if cues[1].EndMS != 3000 || cues[1].Words[1].StartMS != 2600 {
		t.Fatal("input cues changed")
	}
}

func TestSplitScreenSegments(t *testing.T) {
	srcs := map[string]Source{"a": {Width: 1920, Height: 1080, DurationMS: 60_000, HasAudio: true}, "b": {Width: 1920, Height: 1080, DurationMS: 10_000}}
	c := montageOf(Segment{SourceAssetID: "a", StartMS: 1000, EndMS: 5000, Focus: 0.5,
		Tiles: []Tile{{SourceAssetID: "b", StartMS: 2000, Focus: 0.5}, {SourceAssetID: "a", StartMS: 30_000, Focus: 0.2}}})
	if err := c.ValidateMontage(srcs); err != nil {
		t.Fatal(err)
	}
	if ids := c.SourceIDs(); len(ids) != 2 {
		t.Fatalf("SourceIDs = %v", ids)
	}
	m := PlanMontage(c, srcs, map[string]string{"a": "A", "b": "B"}, nil)
	p := m.Parts[0]
	// Three 1080x640 panels: each crop is the 1080:640 region of its source.
	if len(p.Stack) != 2 || p.CropW != 1822 || p.CropH != 1080 || p.Stack[0].Input != "B" || p.Stack[0].Start != 2*time.Second || p.Stack[1].CropX != 20 {
		t.Fatalf("plan = %+v", p)
	}
	for field, mutate := range map[string]func(*Composition){
		"composition.segments[0].tiles":                    func(c *Composition) { c.Segments[0].Tiles = append(c.Segments[0].Tiles, Tile{SourceAssetID: "a"}) },
		"composition.segments[0].tiles[0].start_ms":        func(c *Composition) { c.Segments[0].Tiles[0].StartMS = 8000 },
		"composition.segments[0].tiles[1].source_asset_id": func(c *Composition) { c.Segments[0].Tiles[1].SourceAssetID = "z" },
	} {
		cc := montageOf(Segment{SourceAssetID: "a", StartMS: 1000, EndMS: 5000, Focus: 0.5,
			Tiles: []Tile{{SourceAssetID: "b", StartMS: 2000, Focus: 0.5}, {SourceAssetID: "a", StartMS: 30_000, Focus: 0.2}}})
		mutate(&cc)
		wantField(t, cc.ValidateMontage(srcs), field)
	}
}

func TestSplitScreenFollowsTheBrief(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 600_000, true)
	w.briefRules(org, proj, src,
		ai.BriefRule{Kind: ai.RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"},
		ai.BriefRule{Kind: ai.RuleCaution, StartMS: 300_000, EndMS: 300_000, Note: "Logo"})
	seg := Segment{SourceAssetID: src, StartMS: 100_000, EndMS: 104_000, Focus: 0.5,
		Tiles: []Tile{{SourceAssetID: src, StartMS: 45_000, Focus: 0.5}}} // the tile shows 0:45-0:49
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Split", montageOf(seg), nil)
	wantField(t, err, "composition.segments[0].start_ms")
	seg.Tiles[0].StartMS = 298_000
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Split", montageOf(seg), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Flags) != 1 || r.Flags[0].Kind != ai.RuleCaution {
		t.Fatalf("flags from the tile = %+v", r.Flags)
	}
	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	if len(ff.montages[0].Parts[0].Stack) != 1 {
		t.Fatalf("rendered without the tile: %+v", ff.montages[0].Parts[0])
	}
}

func TestGaps(t *testing.T) {
	ps := []media.Pause{
		{StartMS: 500, EndMS: 1500},      // starts the segment's range: cut from its start
		{StartMS: 3000, EndMS: 3300},     // too short to cut
		{StartMS: 5000, EndMS: 6000},     // cut, keeping a little either side
		{StartMS: 9500, EndMS: 12_000},   // runs past the end: cut to the end
		{StartMS: 20_000, EndMS: 21_000}, // outside
	}
	got := gaps(ps, 1000, 10_000)
	want := []Range{{1000, 1380}, {5150, 5880}, {9650, 10_000}}
	if len(got) != len(want) {
		t.Fatalf("gaps = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("gaps = %+v, want %+v", got, want)
		}
	}
	s := Segment{StartMS: 1000, EndMS: 10_000, Gaps: got}
	if p := s.Pieces(); len(p) != 2 || p[0] != (Range{1380, 5150}) || p[1] != (Range{5880, 9650}) || s.PlayMS() != 3770+3770 {
		t.Fatalf("pieces = %+v, plays %d", p, s.PlayMS())
	}
}

func TestRenderTightenedMontage(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	src := w.video(org, proj, "video", 1920, 1080, 60_000, true)
	w.transcribe(org, src, [3]any{10_000, 12_000, "First thought"}, [3]any{14_000, 16_000, "second thought"})
	c := montageOf(Segment{SourceAssetID: src, StartMS: 10_000, EndMS: 16_000, Focus: 0.5})
	c.Tighten = true

	// Pauses not found yet (processed before pause cutting).
	_, err := w.svc.Create(w.ctx, ana.id, org, proj, "Tight", c, nil)
	wantField(t, err, "composition.tighten")

	if _, err := w.db.ExecContext(w.ctx, `UPDATE media_assets SET pauses = '[[12000,14000],[30000,31000]]' WHERE id = ?`, src); err != nil {
		t.Fatal(err)
	}
	// Gaps a client sends are replaced by the server's.
	c.Segments[0].Gaps = []Range{{10_000, 15_000}}
	r, err := w.svc.Create(w.ctx, ana.id, org, proj, "Tight", c, &CaptionOptions{Style: CaptionsBold, Mode: ModeWords})
	if err != nil {
		t.Fatal(err)
	}
	seg := r.Composition.Segments[0]
	if len(seg.Gaps) != 1 || seg.Gaps[0] != (Range{12_150, 13_880}) || r.Composition.LengthMS() != 6000-1730 {
		t.Fatalf("gaps = %+v, length %d", seg.Gaps, r.Composition.LengthMS())
	}
	// "second" now starts 1.73 s earlier on the clip's timeline.
	var second *CueWord
	for _, q := range r.Composition.Captions.Cues {
		for i, wd := range q.Words {
			if wd.Text == "second" {
				second = &q.Words[i]
			}
		}
	}
	if second == nil || second.StartMS != 4000-1730 {
		t.Fatalf("second word at %+v, want %d ms", second, 4000-1730)
	}

	ff := &fakeFFmpeg{}
	if err := w.renderer(ff).Process(w.ctx, w.job(r.ID)); err != nil {
		t.Fatal(err)
	}
	p := ff.montages[0].Parts[0]
	if len(p.Keep) != 2 || p.Keep[0] != (ffmpeg.Span{From: 0, To: 2150 * time.Millisecond}) || p.Plays() != 4270*time.Millisecond {
		t.Fatalf("part = %+v", p)
	}
	if ff.montages[0].Length() != 4270*time.Millisecond {
		t.Fatalf("montage length %v", ff.montages[0].Length())
	}

	// Without tighten, gaps are cleared.
	c.Tighten = false
	r, err = w.svc.Create(w.ctx, ana.id, org, proj, "Loose", c, nil)
	if err != nil || len(r.Composition.Segments[0].Gaps) != 0 || r.Composition.LengthMS() != 6000 {
		t.Fatalf("untightened = %+v, %v", r.Composition.Segments, err)
	}
}
