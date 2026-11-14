package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

func TestOrganisationModelSettings(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	accounts := account.NewService(db)
	user := func(name string) (string, string) {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		u, err := accounts.SignIn(ctx, identity.Profile{ProviderUserID: "user_ai_" + id, Email: "ai-" + id + "@example.com", EmailVerified: true, FirstName: name})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID) })
		return u.ID, "ai-" + id + "@example.com"
	}
	owner, _ := user("Owner")
	editor, editorEmail := user("Ed")
	outsider, _ := user("Eve")
	org, err := accounts.CreateOrganisation(ctx, owner, "aitest team")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM organisations WHERE id = ?`, org.ID) })
	inv, _ := accounts.CreateInvitation(ctx, owner, org.ID, editorEmail, authz.RoleEditor)
	if _, err := accounts.AcceptInvitation(ctx, editor, inv.ID); err != nil {
		t.Fatal(err)
	}

	// Default until chosen.
	m, err := Resolve(ctx, db, org.ID, TaskTranscription)
	if err != nil || m != Default(TaskTranscription) {
		t.Fatalf("resolve = %+v, %v", m, err)
	}
	s, err := Settings(ctx, db, editor, org.ID)
	if err != nil || len(s) != len(Tasks()) || s[0].Custom || len(s[1].Options) < 2 {
		t.Fatalf("settings = %+v, %v", s, err)
	}

	// Only managers of the organisation can change it, and only to supported
	// models. (Highlights offers two models; transcription currently one.)
	lite := "gemini-3.5-flash-lite"
	if err := SetModel(ctx, db, editor, org.ID, TaskHighlights, ProviderGemini, lite); !errors.Is(err, account.ErrForbidden) {
		t.Fatalf("editor: %v", err)
	}
	if err := SetModel(ctx, db, outsider, org.ID, TaskHighlights, ProviderGemini, lite); !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	var verr *account.ValidationError
	if err := SetModel(ctx, db, owner, org.ID, TaskHighlights, ProviderGemini, "gemini-ultra-9000"); !errors.As(err, &verr) || verr.Field != "model" {
		t.Fatalf("unsupported model: %v", err)
	}
	if err := SetModel(ctx, db, owner, org.ID, "telepathy", ProviderGemini, lite); !errors.As(err, &verr) || verr.Field != "task" {
		t.Fatalf("unknown task: %v", err)
	}
	if err := SetModel(ctx, db, owner, org.ID, TaskHighlights, ProviderGemini, lite); err != nil {
		t.Fatal(err)
	}
	if m, _ := Resolve(ctx, db, org.ID, TaskHighlights); m.ID != lite {
		t.Fatalf("after set: %+v", m)
	}
	if s, _ := Settings(ctx, db, editor, org.ID); !s[1].Custom || s[1].Model.ID != lite {
		t.Fatalf("settings after set = %+v", s)
	}

	// A stored model that's later removed from the catalogue falls back to the default.
	if _, err := db.ExecContext(ctx, `UPDATE organisation_ai_models SET model = 'gemini-retired' WHERE organisation_id = ?`, org.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := Resolve(ctx, db, org.ID, TaskHighlights); m != Default(TaskHighlights) {
		t.Fatalf("retired model resolved to %+v", m)
	}

	// Reset to default.
	if err := SetModel(ctx, db, owner, org.ID, TaskHighlights, "", ""); err != nil {
		t.Fatal(err)
	}
	if s, _ := Settings(ctx, db, owner, org.ID); s[1].Custom {
		t.Fatal("reset did not return to default")
	}
	var n int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE organisation_id = ? AND action = 'ai.model_changed'`, org.ID).Scan(&n)
	if n != 2 {
		t.Errorf("audit events = %d, want 2", n)
	}
}
