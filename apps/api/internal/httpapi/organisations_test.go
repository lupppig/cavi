package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/identity"
	"github.com/lupppig/cavi/apps/api/internal/project"
	"github.com/lupppig/cavi/apps/api/internal/session"
	"github.com/lupppig/cavi/apps/api/internal/testdb"
)

// orgWorld is a real router backed by real services and Postgres.
type orgWorld struct {
	t        *testing.T
	router   *gin.Engine
	accounts *account.Service
	sessions *session.Store
}

func newOrgWorld(t *testing.T) *orgWorld {
	db := testdb.Open(t)
	accounts := account.NewService(db)
	sessions := session.NewStore(db, time.Hour)
	r, err := NewRouter(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:             fakePinger{},
		AllowedOrigins: []string{webURL},
		Auth:           &AuthOptions{Provider: &fakeProvider{}, Accounts: accounts, Sessions: sessions, WebURL: webURL},
		Organisations:  accounts,
		Projects:       project.NewService(db),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM organisations WHERE name LIKE 'httptest-%'`)
	})
	return &orgWorld{t: t, router: r, accounts: accounts, sessions: sessions}
}

type person struct {
	id     string
	email  string
	cookie *http.Cookie
}

func (w *orgWorld) person(name string) person {
	w.t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	p := identity.Profile{ProviderUserID: "user_http_" + id, Email: name + "-" + id + "@example.com", EmailVerified: true, FirstName: name}
	u, err := w.accounts.SignIn(context.Background(), p)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() {
		_, _ = testdb.Open(w.t).ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, u.ID)
	})
	tok, _, err := w.sessions.Create(context.Background(), u.ID, "")
	if err != nil {
		w.t.Fatal(err)
	}
	return person{id: u.ID, email: p.Email, cookie: &http.Cookie{Name: sessionCookie, Value: tok}}
}

func (w *orgWorld) call(p person, method, path, body string, origin ...string) *httptest.ResponseRecorder {
	w.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if len(origin) > 0 {
		req.Header.Set("Origin", origin[0])
	} else {
		req.Header.Set("Origin", webURL)
	}
	if p.cookie != nil {
		req.AddCookie(p.cookie)
	}
	return do(w.t, w.router, req)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body.String())
	}
	if code != "" && decodeError(t, rec).Code != code {
		t.Fatalf("code = %q, want %q", decodeError(t, rec).Code, code)
	}
}

func TestOrganisationLifecycleOverHTTP(t *testing.T) {
	w := newOrgWorld(t)
	ana, ben, eve := w.person("ana"), w.person("ben"), w.person("eve")

	// Ana creates a team.
	rec := w.call(ana, "POST", "/api/v1/organisations", `{"name":"httptest-studio"}`)
	expect(t, rec, http.StatusCreated, "")
	org := decode[organisationResponse](t, rec)
	if org.Role != "owner" || org.Personal || len(org.Permissions) == 0 {
		t.Fatalf("org = %+v", org)
	}
	base := "/api/v1/organisations/" + org.ID

	// Outsiders get 404 for everything, never 403.
	expect(t, w.call(eve, "GET", base, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "GET", base+"/members", ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "POST", base+"/invitations", `{"email":"x@example.com","role":"owner"}`), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "PATCH", base, `{"name":"pwned"}`), http.StatusNotFound, CodeNotFound)

	// Signed-out callers get 401.
	expect(t, w.call(person{}, "GET", base, ""), http.StatusUnauthorized, CodeUnauthenticated)

	// Cross-site writes are refused even with a valid session.
	expect(t, w.call(ana, "PATCH", base, `{"name":"httptest-x"}`, "https://evil.example"), http.StatusForbidden, CodeForbidden)

	// Validation errors use the field envelope.
	rec = w.call(ana, "POST", base+"/invitations", `{"email":"nope","role":"viewer"}`)
	expect(t, rec, http.StatusUnprocessableEntity, CodeValidation)
	if f := decodeError(t, rec).Fields; len(f) != 1 || f[0].Field != "email" {
		t.Errorf("fields = %+v", f)
	}
	expect(t, w.call(ana, "POST", base+"/invitations", `{"email":"a@b.co","role":"emperor"}`), http.StatusUnprocessableEntity, CodeValidation)

	// Ana invites Ben as a viewer; Ben sees and accepts it.
	rec = w.call(ana, "POST", base+"/invitations", `{"email":"`+strings.ToUpper(ben.email)+`","role":"viewer"}`)
	expect(t, rec, http.StatusCreated, "")
	inv := decode[invitationResponse](t, rec)
	expect(t, w.call(ana, "POST", base+"/invitations", `{"email":"`+ben.email+`","role":"viewer"}`), http.StatusConflict, CodeInvitationExists)

	expect(t, w.call(eve, "POST", "/api/v1/invitations/"+inv.ID+"/accept", ""), http.StatusNotFound, CodeNotFound)
	mine := decode[listResponse[invitationResponse]](t, w.call(ben, "GET", "/api/v1/invitations", ""))
	if len(mine.Data) != 1 || mine.Data[0].OrganisationName != "httptest-studio" {
		t.Fatalf("ben's invitations = %+v", mine)
	}
	rec = w.call(ben, "POST", "/api/v1/invitations/"+inv.ID+"/accept", "")
	expect(t, rec, http.StatusOK, "")
	if got := decode[organisationResponse](t, rec); got.Role != "viewer" {
		t.Errorf("accepted role = %q", got.Role)
	}

	// Ben (viewer) can list members but not manage them.
	members := decode[listResponse[memberResponse]](t, w.call(ben, "GET", base+"/members", ""))
	if len(members.Data) != 2 {
		t.Fatalf("members = %+v", members)
	}
	expect(t, w.call(ben, "POST", base+"/invitations", `{"email":"z@example.com"}`), http.StatusForbidden, CodeForbidden)
	expect(t, w.call(ben, "PATCH", base+"/members/"+ben.id, `{"role":"admin"}`), http.StatusForbidden, CodeForbidden)

	// The last owner can't leave.
	expect(t, w.call(ana, "DELETE", base+"/members/"+ana.id, ""), http.StatusConflict, CodeLastOwner)

	// Ana promotes Ben, then Ben can manage; finally Ana removes Ben.
	expect(t, w.call(ana, "PATCH", base+"/members/"+ben.id, `{"role":"admin"}`), http.StatusNoContent, "")
	expect(t, w.call(ben, "GET", base+"/invitations", ""), http.StatusOK, "")
	expect(t, w.call(ana, "DELETE", base+"/members/"+ben.id, ""), http.StatusNoContent, "")
	expect(t, w.call(ben, "GET", base, ""), http.StatusNotFound, CodeNotFound)
}

func TestPersonalOrganisationConflictOverHTTP(t *testing.T) {
	w := newOrgWorld(t)
	ana := w.person("ana")
	_, ms, err := w.accounts.Me(context.Background(), ana.id)
	if err != nil {
		t.Fatal(err)
	}
	rec := w.call(ana, "POST", "/api/v1/organisations/"+ms[0].OrganisationID+"/invitations", `{"email":"f@example.com","role":"editor"}`)
	expect(t, rec, http.StatusConflict, CodePersonalOrganisation)
}
