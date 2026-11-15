package render

import (
	"strings"
	"testing"
	"time"

	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
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
	m := PlanMontage(c, srcs, map[string]string{"a": "https://a", "b": "https://b"})
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
	ass := OverlayASS(nil, &Hook{Text: "How diamonds built Botswana", EndMS: 2000}, "9:16")
	if !strings.Contains(ass, "Style: Hook,") || !strings.Contains(ass, "Dialogue: 1,0:00:00.00,0:00:02.00,Hook,,0,0,0,,{\\q0}How diamonds built Botswana") {
		t.Fatalf("hook overlay:\n%s", ass)
	}
	caps := &Captions{CaptionOptions: CaptionOptions{Style: CaptionsBoxed}, Cues: cues}
	both := OverlayASS(caps, &Hook{Text: "Hook", EndMS: 1000}, "9:16")
	if strings.Index(both, "Style: Hook") > strings.Index(both, "[Events]") || !strings.Contains(both, ",Cap,") {
		t.Fatalf("captions and hook:\n%s", both)
	}
	if OverlayASS(nil, nil, "9:16") != "" {
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
	if m.Voiceover == "" || m.VoiceoverAt != 1500*time.Millisecond || m.VoiceoverLength != 4*time.Second || m.Duck != 0.25 {
		t.Fatalf("plan voiceover = %q at %v for %v duck %v", m.Voiceover, m.VoiceoverAt, m.VoiceoverLength, m.Duck)
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
