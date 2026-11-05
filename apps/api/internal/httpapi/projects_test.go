package httpapi

import (
	"net/http"
	"net/url"
	"testing"
)

func TestProjectLifecycleOverHTTP(t *testing.T) {
	w := newOrgWorld(t)
	ana, cli, eve := w.person("ana"), w.person("cli"), w.person("eve")

	org := decode[organisationResponse](t, w.call(ana, "POST", "/api/v1/organisations", `{"name":"httptest-agency"}`))
	base := "/api/v1/organisations/" + org.ID + "/projects"

	// Create, with validation and strict bodies.
	expect(t, w.call(ana, "POST", base, `{"name":""}`), http.StatusUnprocessableEntity, CodeValidation)
	expect(t, w.call(ana, "POST", base, `{"name":"x","owner":"me"}`), http.StatusBadRequest, CodeInvalidRequest)
	expect(t, w.call(ana, "POST", base, `{"name":"x"}`, "https://evil.example"), http.StatusForbidden, CodeForbidden)
	rec := w.call(ana, "POST", base, `{"name":"Spring campaign","description":"Six cutdowns"}`)
	expect(t, rec, http.StatusCreated, "")
	p := decode[projectResponse](t, rec)
	if p.Workspace.Name != "Main" || p.CreatedBy == nil || p.CreatedBy.ID != ana.id || len(p.Permissions) != 4 {
		t.Fatalf("project = %+v", p)
	}
	for i := 0; i < 2; i++ {
		expect(t, w.call(ana, "POST", base, `{"name":"Filler"}`), http.StatusCreated, "")
	}

	// Paging: 3 projects at limit 2 → 2 + 1.
	page := decode[projectPage](t, w.call(ana, "GET", base+"?limit=2", ""))
	if len(page.Data) != 2 || page.NextCursor == nil {
		t.Fatalf("page 1 = %+v", page)
	}
	page2 := decode[projectPage](t, w.call(ana, "GET", base+"?limit=2&cursor="+url.QueryEscape(*page.NextCursor), ""))
	if len(page2.Data) != 1 || page2.NextCursor != nil || page2.Data[0].ID != p.ID {
		t.Fatalf("page 2 = %+v", page2)
	}
	expect(t, w.call(ana, "GET", base+"?limit=abc", ""), http.StatusUnprocessableEntity, CodeValidation)
	expect(t, w.call(ana, "GET", base+"?limit=0", ""), http.StatusUnprocessableEntity, CodeValidation)

	// Partial update keeps the other field.
	one := base + "/" + p.ID
	rec = w.call(ana, "PATCH", one, `{"name":"Spring campaign v2"}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[projectResponse](t, rec); got.Name != "Spring campaign v2" || got.Description != "Six cutdowns" {
		t.Fatalf("updated = %+v", got)
	}
	expect(t, w.call(ana, "PATCH", one, `{}`), http.StatusUnprocessableEntity, CodeValidation)

	// Outsiders and bad IDs get 404.
	expect(t, w.call(eve, "GET", one, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "GET", base, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "GET", base+"/not-a-uuid", ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(person{}, "GET", one, ""), http.StatusUnauthorized, CodeUnauthenticated)

	// A client sees nothing until the project is shared.
	inv := decode[invitationResponse](t, w.call(ana, "POST", "/api/v1/organisations/"+org.ID+"/invitations",
		`{"email":"`+cli.email+`","role":"client"}`))
	expect(t, w.call(cli, "POST", "/api/v1/invitations/"+inv.ID+"/accept", ""), http.StatusOK, "")
	if got := decode[projectPage](t, w.call(cli, "GET", base, "")); len(got.Data) != 0 {
		t.Fatalf("client sees %d projects before sharing", len(got.Data))
	}
	expect(t, w.call(cli, "GET", one, ""), http.StatusNotFound, CodeNotFound)

	expect(t, w.call(ana, "POST", one+"/access", `{"user_id":"`+eve.id+`"}`), http.StatusUnprocessableEntity, CodeValidation)
	expect(t, w.call(ana, "POST", one+"/access", `{"user_id":"`+cli.id+`"}`), http.StatusNoContent, "")
	grants := decode[listResponse[grantResponse]](t, w.call(ana, "GET", one+"/access", ""))
	if len(grants.Data) != 1 || grants.Data[0].UserID != cli.id {
		t.Fatalf("grants = %+v", grants)
	}
	got := decode[projectPage](t, w.call(cli, "GET", base, ""))
	if len(got.Data) != 1 || got.Data[0].ID != p.ID || len(got.Data[0].Permissions) != 1 {
		t.Fatalf("client list = %+v", got)
	}
	expect(t, w.call(cli, "PATCH", one, `{"name":"client edit"}`), http.StatusForbidden, CodeForbidden)
	expect(t, w.call(cli, "DELETE", one, ""), http.StatusForbidden, CodeForbidden)
	expect(t, w.call(cli, "GET", one+"/access", ""), http.StatusForbidden, CodeForbidden)

	expect(t, w.call(ana, "DELETE", one+"/access/"+cli.id, ""), http.StatusNoContent, "")
	expect(t, w.call(cli, "GET", one, ""), http.StatusNotFound, CodeNotFound)

	// Delete.
	expect(t, w.call(ana, "DELETE", one, ""), http.StatusNoContent, "")
	expect(t, w.call(ana, "GET", one, ""), http.StatusNotFound, CodeNotFound)
}
