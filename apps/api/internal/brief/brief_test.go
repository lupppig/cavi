package brief

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

const briefText = `Key timestamps: 01:30 – how it began.
0:47 - 0:53 (this should not be included in any content)
Licence plates: 03:13 – blur/crop.`

// fakeReader returns a fixed brief, or err.
type fakeReader struct {
	brief ai.Brief
	err   error
	calls int
}

func (f *fakeReader) ReadBrief(_ context.Context, _ ai.Model, text string, durationMS int64) (ai.Brief, ai.Usage, error) {
	f.calls++
	if f.err != nil {
		return ai.Brief{}, ai.Usage{InputTokens: 10}, f.err
	}
	b, err := ai.ValidateBrief(f.brief, text, durationMS)
	return b, ai.Usage{InputTokens: 100, OutputTokens: 50}, err
}

func goodBrief() ai.Brief {
	return ai.Brief{CampaignMessage: "Diamonds fund schools.", Guidance: "Warm and organic.", Rules: []ai.BriefRule{
		{Kind: ai.RulePriority, StartMS: 90_000, EndMS: 90_000, Note: "How it began"},
		{Kind: ai.RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"},
		{Kind: ai.RuleBlur, StartMS: 193_000, EndMS: 193_000, Note: "Licence plate"},
	}}
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
	return &world{t: t, ctx: context.Background(), db: db, accounts: account.NewService(db),
		projects: project.NewService(db), svc: NewService(db)}
}

type person struct{ id, email string }

func (w *world) user(first string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_brief_" + id, Email: "brief-" + id + "@example.com", EmailVerified: true, FirstName: first}
	u, err := w.accounts.SignIn(w.ctx, p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _, _ = w.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
	return person{id: u.ID, email: p.Email}
}

func (w *world) team(owner person, members map[string]person) string {
	w.t.Helper()
	org, err := w.accounts.CreateOrganisation(w.ctx, owner.id, "brieftest team")
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
	p, err := w.projects.Create(w.ctx, owner.id, orgID, "", "Brief test", "")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.ID
}

func (w *world) video(orgID, projectID string, durationMS int64) string {
	w.t.Helper()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var id string
	if err := w.db.QueryRowContext(w.ctx, `
		INSERT INTO media_assets (organisation_id, project_id, storage_key, filename, content_type, size_bytes, status, kind,
			uploaded_at, processing_status, width, height, duration_ms)
		VALUES (?, ?, ?, 'vlog.mp4', 'video/mp4', 100, 'uploaded', 'video', now(), 'ready', 1920, 1080, ?)
		RETURNING id`, orgID, projectID, "brieftest/"+hex.EncodeToString(b), durationMS).Scan(&id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) job(assetID string) jobs.Job {
	w.t.Helper()
	j := jobs.Job{MediaAssetID: assetID, Kind: jobs.KindBriefRead}
	if err := w.db.QueryRowContext(w.ctx, `
		SELECT id, organisation_id FROM jobs WHERE media_asset_id = ? AND kind = 'brief.read' ORDER BY created_at DESC LIMIT 1`,
		assetID).Scan(&j.ID, &j.OrganisationID); err != nil {
		w.t.Fatalf("no brief job: %v", err)
	}
	return j
}

func (w *world) reader(f *fakeReader) *Reader {
	return NewReader(w.db, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func wantErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var v *account.ValidationError
	if !errors.As(err, &v) || v.Field != field {
		t.Fatalf("err = %v, want a validation error on %s", err, field)
	}
}

func TestBriefLifecycle(t *testing.T) {
	w := newWorld(t)
	ana, vic, cli := w.user("Ana"), w.user("Vic"), w.user("Cli")
	org := w.team(ana, map[string]person{authz.RoleViewer: vic, authz.RoleClient: cli})
	proj := w.project(ana, org)
	vid := w.video(org, proj, 2_635_557)

	if _, err := w.svc.Get(w.ctx, ana.id, org, proj); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before submit: %v", err)
	}
	b, err := w.svc.Submit(w.ctx, ana.id, org, proj, "  "+briefText+"  ", vid)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != StatusQueued || b.MediaAssetID != vid || b.SourceText != briefText || !b.CanEdit || b.PromptVersion != ai.BriefsPromptVersion {
		t.Fatalf("submitted brief = %+v", b)
	}
	_, err = w.svc.Submit(w.ctx, ana.id, org, proj, briefText, vid)
	wantErr(t, err, ErrInProgress)
	_, err = w.svc.Update(w.ctx, ana.id, org, proj, Edit{})
	wantErr(t, err, ErrInProgress)

	fake := &fakeReader{brief: goodBrief()}
	if err := w.reader(fake).Process(w.ctx, w.job(vid)); err != nil {
		t.Fatal(err)
	}
	b, err = w.svc.Get(w.ctx, vic.id, org, proj)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != StatusReady || b.CampaignMessage != "Diamonds fund schools." || len(b.Rules) != 3 || b.CanEdit {
		t.Fatalf("read brief (viewer) = %+v", b)
	}
	if b.Rules[0].Kind != ai.RuleExclude || b.Rules[0].MediaAssetID != vid {
		t.Fatalf("rules not in time order: %+v", b.Rules)
	}
	var usage int
	_ = w.db.QueryRowContext(w.ctx, `SELECT count(*) FROM ai_usage WHERE organisation_id = ? AND task = 'briefs'`, org).Scan(&usage)
	if usage != 1 {
		t.Fatalf("ai usage rows = %d", usage)
	}
	// A rerun of a finished job does nothing.
	if err := w.reader(fake).Process(w.ctx, w.job(vid)); err != nil || fake.calls != 1 {
		t.Fatalf("rerun: err %v, calls %d", err, fake.calls)
	}

	// Rules feed clip checks.
	rules, err := Rules(w.ctx, w.db, org, vid)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := Excluded(rules, 40_000, 48_000); !ok || r.Note != "Not to be used" {
		t.Fatalf("Excluded = %+v, %v", r, ok)
	}
	if _, ok := Excluded(rules, 53_000, 120_000); ok {
		t.Fatal("clip after the excluded range was excluded")
	}
	if f := Flags(rules, 60_000, 200_000); len(f) != 1 || f[0].Kind != ai.RuleBlur {
		t.Fatalf("Flags = %+v (priority must not be a flag)", f)
	}
	// A plate named at 3:13 is usually still on screen at 3:18.
	if f := Flags(rules, 198_000, 220_000); len(f) != 1 || f[0].Kind != ai.RuleBlur {
		t.Fatalf("Flags just after a single-moment rule = %+v", f)
	}
	if f := Flags(rules, 204_000, 220_000); len(f) != 0 {
		t.Fatalf("Flags beyond the window = %+v", f)
	}

	// Review: edit the whole list.
	edit := Edit{CampaignMessage: "Natural diamonds fund schools.", Guidance: "", Rules: []Rule{
		{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: ai.RuleMute, StartMS: 835_000, EndMS: 845_000, Note: "Music"}},
		{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: ai.RuleExclude, StartMS: 47_000, EndMS: 53_000, Note: "Not to be used"}},
	}}
	b, err = w.svc.Update(w.ctx, ana.id, org, proj, edit)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Rules) != 2 || b.Rules[0].Kind != ai.RuleExclude || b.CampaignMessage != "Natural diamonds fund schools." {
		t.Fatalf("updated brief = %+v", b)
	}

	bad := func(r Rule) Edit { return Edit{Rules: []Rule{r}} }
	other := w.video(org, w.project(ana, org), 60_000)
	for field, e := range map[string]Edit{
		"rules[0].media_asset_id": bad(Rule{MediaAssetID: other, BriefRule: ai.BriefRule{Kind: ai.RuleExclude, StartMS: 0, EndMS: 1, Note: "x"}}),
		"rules[0].kind":           bad(Rule{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: "cut", StartMS: 0, EndMS: 1, Note: "x"}}),
		"rules[0].start_ms":       bad(Rule{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: ai.RuleBlur, StartMS: 3_000_000, EndMS: 3_000_000, Note: "x"}}),
		"rules[0].end_ms":         bad(Rule{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: ai.RuleBlur, StartMS: 10, EndMS: 5, Note: "x"}}),
		"rules[0].note":           bad(Rule{MediaAssetID: vid, BriefRule: ai.BriefRule{Kind: ai.RuleBlur, StartMS: 10, EndMS: 10, Note: " "}}),
	} {
		_, err := w.svc.Update(w.ctx, ana.id, org, proj, e)
		wantField(t, err, field)
	}

	// Viewers and clients can read but not change it.
	if _, err := w.svc.Update(w.ctx, vic.id, org, proj, edit); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer update: %v", err)
	}
	if err := w.projects.GrantAccess(w.ctx, ana.id, org, proj, cli.id); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Get(w.ctx, cli.id, org, proj); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if err := w.svc.Delete(w.ctx, cli.id, org, proj); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client delete: %v", err)
	}

	if err := w.svc.Delete(w.ctx, ana.id, org, proj); err != nil {
		t.Fatal(err)
	}
	if rules, _ := Rules(w.ctx, w.db, org, vid); len(rules) != 0 {
		t.Fatalf("rules left after delete: %+v", rules)
	}
}

func TestBriefTenantIsolation(t *testing.T) {
	w := newWorld(t)
	ana, eve := w.user("Ana"), w.user("Eve")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	vid := w.video(org, proj, 600_000)
	evesOrg := w.team(eve, nil)
	evesProj := w.project(eve, evesOrg)

	if _, err := w.svc.Submit(w.ctx, ana.id, org, proj, briefText, vid); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Get(w.ctx, eve.id, org, proj); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider Get: %v", err)
	}
	if err := w.svc.Delete(w.ctx, eve.id, org, proj); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider Delete: %v", err)
	}
	// Another tenant's video can't be named in a brief.
	_, err := w.svc.Submit(w.ctx, eve.id, evesOrg, evesProj, briefText, vid)
	wantField(t, err, "media_asset_id")
	// Nor can a file from another project in the same organisation.
	_, err = w.svc.Submit(w.ctx, ana.id, org, w.project(ana, org), briefText, vid)
	wantField(t, err, "media_asset_id")
	// A job naming the video under the wrong organisation does nothing.
	fake := &fakeReader{brief: goodBrief()}
	j := w.job(vid)
	j.OrganisationID = evesOrg
	if err := w.reader(fake).Process(w.ctx, j); err != nil || fake.calls != 0 {
		t.Fatalf("cross-tenant job: err %v, calls %d", err, fake.calls)
	}
}

func TestBriefReadFailures(t *testing.T) {
	w := newWorld(t)
	ana := w.user("Ana")
	org := w.team(ana, nil)
	proj := w.project(ana, org)
	vid := w.video(org, proj, 600_000)

	if _, err := w.svc.Submit(w.ctx, ana.id, org, proj, briefText, vid); err != nil {
		t.Fatal(err)
	}
	// Invented times fail validation and are retried.
	invented := goodBrief()
	invented.Rules = append(invented.Rules, ai.BriefRule{Kind: ai.RuleExclude, StartMS: 500_000, EndMS: 500_000, Note: "made up"})
	err := w.reader(&fakeReader{brief: invented}).Process(w.ctx, w.job(vid))
	if !errors.Is(err, ai.ErrInvalidOutput) || jobs.IsPermanent(err) {
		t.Fatalf("invented time: %v", err)
	}
	// Giving up records a message for people.
	if err := w.reader(&fakeReader{}).Failed(w.ctx, w.job(vid), err); err != nil {
		t.Fatal(err)
	}
	b, err := w.svc.Get(w.ctx, ana.id, org, proj)
	if err != nil || b.Status != StatusFailed || b.Error != msgGaveUp || len(b.Rules) != 0 {
		t.Fatalf("failed brief = %+v, %v", b, err)
	}

	// Deleting the video mid-read leaves a failed brief that can be resubmitted.
	if _, err := w.svc.Submit(w.ctx, ana.id, org, proj, briefText, vid); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(w.ctx, `DELETE FROM media_assets WHERE id = ?`, vid); err != nil {
		t.Fatal(err)
	}
	b, err = w.svc.Get(w.ctx, ana.id, org, proj)
	if err != nil || b.Status != StatusFailed || b.Error != msgVideoGone || b.MediaAssetID != "" {
		t.Fatalf("brief after video deleted = %+v, %v", b, err)
	}
	if _, err := w.svc.Submit(w.ctx, ana.id, org, proj, briefText, w.video(org, proj, 600_000)); err != nil {
		t.Fatalf("resubmit after video deleted: %v", err)
	}
}
