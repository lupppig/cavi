package httpapi

import (
	"net/http"
	"strconv"
	"testing"
)

func TestRendersOverHTTP(t *testing.T) {
	w := newOrgWorld(t)
	ana, eve := w.person("ana"), w.person("eve")
	org := decode[organisationResponse](t, w.call(ana, "POST", "/api/v1/organisations", `{"name":"httptest-renders"}`))
	proj := decode[projectResponse](t, w.call(ana, "POST", "/api/v1/organisations/"+org.ID+"/projects", `{"name":"Ep. 12"}`))
	base := "/api/v1/organisations/" + org.ID + "/projects/" + proj.ID

	// A processed video to cut from.
	rec := w.call(ana, "POST", base+"/media/uploads", `{"filename":"talk.mp4","content_type":"video/mp4","size_bytes":`+strconv.Itoa(len(testMP4))+`}`)
	up := decode[uploadResponse](t, rec)
	w.store.uploadTo(up.Upload.URL, testMP4)
	expect(t, w.call(ana, "POST", base+"/media/"+up.Asset.ID+"/complete", ""), http.StatusOK, "")
	if _, err := w.db.Exec(`UPDATE media_assets SET processing_status = 'ready', width = 1920, height = 1080, duration_ms = 151000,
		audio_codec = 'aac' WHERE id = ?`, up.Asset.ID); err != nil {
		t.Fatal(err)
	}

	body := func(start, end int, aspect string) string {
		return `{"name":"Release advice","composition":{"version":1,"source_asset_id":"` + up.Asset.ID +
			`","start_ms":` + strconv.Itoa(start) + `,"end_ms":` + strconv.Itoa(end) + `,"aspect":"` + aspect + `","focus":0.5}}`
	}
	expect(t, w.call(ana, "POST", base+"/renders", `{"name":"x","composition":{"version":1,"extra":true}}`), http.StatusBadRequest, CodeInvalidRequest)
	rec = w.call(ana, "POST", base+"/renders", body(0, 5000, "21:9"))
	expect(t, rec, http.StatusUnprocessableEntity, CodeValidation)
	if f := decodeError(t, rec).Fields; len(f) != 1 || f[0].Field != "composition.aspect" {
		t.Fatalf("fields = %+v", f)
	}
	expect(t, w.call(eve, "POST", base+"/renders", body(0, 5000, "9:16")), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "POST", base+"/renders", body(0, 5000, "9:16"), "https://evil.example"), http.StatusForbidden, CodeForbidden)

	rec = w.call(ana, "POST", base+"/renders", body(87000, 150000, "9:16"))
	expect(t, rec, http.StatusAccepted, "")
	r := decode[renderResponse](t, rec)
	if r.Status != "queued" || r.Output != nil || r.SourceAssetID == nil || *r.SourceAssetID != up.Asset.ID ||
		r.Composition.Aspect != "9:16" || r.Composition.EndMS != 150000 || r.RequestedBy == nil || !r.CanDelete {
		t.Fatalf("created = %+v", r)
	}

	page := decode[renderPage](t, w.call(ana, "GET", base+"/renders?source_asset_id="+up.Asset.ID, ""))
	if len(page.Data) != 1 || page.Data[0].ID != r.ID || page.NextCursor != nil {
		t.Fatalf("list = %+v", page)
	}
	expect(t, w.call(ana, "GET", base+"/renders?source_asset_id=nope", ""), http.StatusUnprocessableEntity, CodeValidation)
	expect(t, w.call(eve, "GET", base+"/renders", ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(eve, "GET", base+"/renders/"+r.ID, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "GET", base+"/renders/"+r.ID, ""), http.StatusOK, "")

	expect(t, w.call(eve, "DELETE", base+"/renders/"+r.ID, ""), http.StatusNotFound, CodeNotFound)
	expect(t, w.call(ana, "DELETE", base+"/renders/"+r.ID, ""), http.StatusNoContent, "")
	expect(t, w.call(ana, "GET", base+"/renders/"+r.ID, ""), http.StatusNotFound, CodeNotFound)
}
