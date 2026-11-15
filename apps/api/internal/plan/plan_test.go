package plan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/ai"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/jobs"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// fakePlanner returns the configured plans, validated as the real one does.
type fakePlanner struct {
	plans []ai.Plan
	err   error
	calls []ai.PlanBrief
	files [][]ai.PlanFile
}

func (f *fakePlanner) Plans(_ context.Context, _ ai.Model, files []ai.PlanFile, b ai.PlanBrief) ([]ai.Plan, ai.Usage, error) {
	f.calls = append(f.calls, b)
	f.files = append(f.files, files)
	if f.err != nil {
		return nil, ai.Usage{}, f.err
	}
	ps, err := ai.ValidatePlans(f.plans, files, b)
	return ps, ai.Usage{InputTokens: 5000, OutputTokens: 400}, err
}

type world struct {
	t        *testing.T
	ctx      context.Context
	db       *bun.DB
	accounts *account.Service
	projects *project.Service
	svc      *Service
}

func newWorld(t *testing.T) *world {
	db := testdb.Open(t)
	return &world{t: t, ctx: context.Background(), db: db, accounts: account.NewService(db), projects: project.NewService(db),
		svc: NewService(db)}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_plan_" + id, Email: "plan-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "plantest team")
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = w.db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID)
	})
	for role, p := range members {
		inv, err := w.accounts.CreateInvitation(w.ctx, owner.id, org.ID, p.email, role)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := w.accounts.AcceptInvitation(w.ctx, p.id, inv.ID); err != nil {
			w.t.Fatal(err)
		}
	}
	return org.ID
}

func (w *world) project(owner person, orgID string) string {
	w.t.Helper()
	p, err := w.projects.Create(w.ctx, owner.id, orgID, "", "Plan test", "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

// transcribed adds a processed video with a ready transcript of n
// ten-second lines.
func (w *world) transcribed(org, proj, name string, n int) string {
	w.t.Helper()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var id, tr string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO media_assets (organisation_id, project_id, storage_key, filename, content_type, size_bytes, status, kind,
			uploaded_at, processing_status, width, height, duration_ms)
		VALUES (?, ?, ?, ?, 'video/mp4', 100, 'uploaded', 'video', now(), 'ready', 1920, 1080, ?) RETURNING id`,
		org, proj, "plantest/"+hex.EncodeToString(b), name, int64(n)*10_000).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO transcripts (organisation_id, media_asset_id, status, provider, model, prompt_version, completed_at)
		VALUES (?, ?, 'ready', 'gemini', 'gemini-3.5-flash-lite', 'transcription-v3', now()) RETURNING id`, org, id).Scan(&tr); err != nil {
		w.t.Fatal(err)
	}
	for i := range n {
		if _, err := w.db.ExecContext(w.ctx, `
			INSERT INTO transcript_segments (transcript_id, position, start_ms, end_ms, speaker, text) VALUES (?, ?, ?, ?, 'Speaker 1', ?)`,
			tr, i, i*10_000, (i+1)*10_000, fmt.Sprintf("line %d", i)); err != nil {
			w.t.Fatal(err)
		}
	}
	return id
}

func (w *world) job(runID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{SubjectID: runID, Kind: jobs.KindPlansCreate}
	if err := w.db.QueryRowContext(w.ctx, `SELECT id, organisation_id FROM jobs WHERE subject_id = ? ORDER BY created_at DESC LIMIT 1`, runID).
		Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no plans job: %v", err)
	}
	return j
}

func (w *world) planner(f *fakePlanner) *Planner {
	return NewPlanner(w.db, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPlans(t *testing.T) {
	w := newWorld(t)
	ana, vic, eve := w.user("Ana"), w.user("Vic"), w.user("Eve")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic})
	proj := w.project(ana, org)
	req := Request{Objective: "diamonds and schools", MinSeconds: 30, MaxSeconds: 60}

	_, err := w.svc.Request(w.ctx, ana.id, org, proj, req)
	var v *account.ValidationError
	if !errors.As(err, &v) || v.Field != "media_asset_ids" {
		t.Fatalf("no transcripts: %v", err)
	}
	talk := w.transcribed(org, proj, "talk.mp4", 20)
	safari := w.transcribed(org, proj, "safari.mp4", 6)
	// The talk has a brief with an excluded moment.
	var briefID string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO briefs (organisation_id, project_id, media_asset_id, source_text, campaign_message, guidance, status, provider, model, prompt_version)
		VALUES (?, ?, ?, 'brief', 'Diamonds fund schools.', '', 'ready', 'gemini', 'gemini-3.5-flash', 'briefs-v1') RETURNING id`,
		org, proj, talk).Scan(&briefID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `
		INSERT INTO brief_rules (brief_id, position, organisation_id, media_asset_id, kind, start_ms, end_ms, note)
		VALUES (?, 0, ?, ?, 'exclude', 47000, 53000, 'Not to be used'), (?, 1, ?, ?, 'blur', 105000, 105000, 'Plate')`,
		briefID, org, talk, briefID, org, talk); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.Request(w.ctx, vic.id, org, proj, req); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := w.svc.Request(w.ctx, eve.id, org, proj, req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	evesOrg := w.team(eve, nil)
	evesFile := w.transcribed(evesOrg, w.project(eve, evesOrg), "theirs.mp4", 5)
	_, err = w.svc.Request(w.ctx, ana.id, org, proj, Request{MinSeconds: 30, MaxSeconds: 60, MediaAssetIDs: []string{talk, evesFile}})
	if !errors.As(err, &v) || v.Field != "media_asset_ids" {
		t.Fatalf("another tenant's file: %v", err)
	}

	run, err := w.svc.Request(w.ctx, ana.id, org, proj, req)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusQueued || len(run.MediaAssetIDs) != 2 || !run.UsedBrief || run.Model != "gemini-3.5-flash" {
		t.Fatalf("run = %+v", run)
	}
	if _, err := w.svc.Request(w.ctx, ana.id, org, proj, req); !errors.Is(err, ErrInProgress) {
		t.Fatalf("second request: %v", err)
	}

	seg := func(f, a, b int) ai.PlanSegment { return ai.PlanSegment{File: f, FirstSegment: a, LastSegment: b} }
	fp := &fakePlanner{plans: []ai.Plan{
		{Title: "Schools", Hook: "Diamonds built this school", VoiceoverScript: "Narration.", Reason: "r", CampaignLink: "l",
			Segments: []ai.PlanSegment{seg(0, 2, 3), seg(1, 0, 1), seg(1, 10, 11)}}, // newest file (safari) is F1
		{Title: "Excluded", Hook: "h", Reason: "r", CampaignLink: "l", Segments: []ai.PlanSegment{seg(1, 4, 7)}},
	}}
	if err := w.planner(fp).Process(w.ctx, w.job(run.ID)); err != nil {
		t.Fatal(err)
	}
	if b := fp.calls[0]; b.Campaign == nil || b.Campaign.Message != "Diamonds fund schools." || len(b.Rules[talk]) != 2 || b.Objective != "diamonds and schools" {
		t.Fatalf("planner brief = %+v", b)
	}
	if f := fp.files[0]; len(f) != 2 || f[0].ID != safari || f[0].Name != "safari.mp4" || len(f[1].Segments) != 20 {
		t.Fatalf("planner files = %+v", f)
	}
	got, err := w.svc.Get(w.ctx, vic.id, org, proj)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusReady || len(got.Plans) != 1 {
		t.Fatalf("plans = %+v", got)
	}
	p := got.Plans[0]
	if len(p.Segments) != 3 || p.Segments[0].SourceAssetID != safari || p.Segments[1].SourceAssetID != talk ||
		p.Segments[1].StartMS != 0 || p.Segments[1].EndMS != 20_000 || p.Hook != "Diamonds built this school" {
		t.Fatalf("plan = %+v", p)
	}
	if f := p.Segments[2].Flags; len(f) != 1 || f[0].Kind != ai.RuleBlur {
		t.Fatalf("flags on the talk's 100-120 s segment = %+v", f)
	}

	// A job naming the run under another organisation does nothing.
	if _, err := w.svc.Request(w.ctx, ana.id, org, proj, req); err != nil {
		t.Fatal(err)
	}
	j := w.job(run.ID)
	j.OrganisationID = evesOrg
	if err := w.planner(fp).Process(w.ctx, j); err != nil || len(fp.calls) != 1 {
		t.Fatalf("cross-tenant job: %v, %d calls", err, len(fp.calls))
	}
	// A deleted file fails the run with a message.
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, safari); err != nil {
		t.Fatal(err)
	}
	j = w.job(run.ID)
	err = w.planner(fp).Process(w.ctx, j)
	if !jobs.IsPermanent(err) {
		t.Fatalf("deleted file: %v", err)
	}
	_ = w.planner(fp).Failed(w.ctx, j, err)
	if got, _ := w.svc.Get(w.ctx, ana.id, org, proj); got.Status != StatusFailed || got.Error != msgGone {
		t.Fatalf("after deletion = %s %q", got.Status, got.Error)
	}
}
