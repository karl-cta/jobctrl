package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // the time zone tests must not depend on the host's zoneinfo

	"github.com/go-chi/chi/v5"

	"job-ctrl/internal/db"
	"job-ctrl/internal/handlers"
	"job-ctrl/internal/models"
)

// testServer creates an isolated server with a temp DB for each test.
type testServer struct {
	handler http.Handler
	h       *handlers.Handler
	dbPath  string
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	f, err := os.CreateTemp("", "job-ctrl-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()

	database, err := db.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open db: %v", err)
	}

	h := handlers.New(database)
	r := chi.NewRouter()
	r.Get("/api/applications/stats", h.GetStats)
	r.Get("/api/applications", h.ListApplications)
	r.Post("/api/applications", h.CreateApplication)
	r.Get("/api/applications/duplicates", h.CheckDuplicates)
	r.Put("/api/applications/bulk/status", h.BulkUpdateStatus)
	r.Delete("/api/applications/bulk", h.BulkDelete)
	r.Get("/api/applications/{id}", h.GetApplication)
	r.Put("/api/applications/{id}", h.UpdateApplication)
	r.Put("/api/applications/{id}/snooze", h.SnoozeFollowUp)
	r.Delete("/api/applications/{id}", h.DeleteApplication)
	r.Get("/api/sources", h.ListSources)
	r.Get("/api/applications/{id}/interviews", h.ListInterviews)
	r.Post("/api/applications/{id}/interviews", h.CreateInterview)
	r.Get("/api/interviews/{id}", h.GetInterview)
	r.Put("/api/interviews/{id}", h.UpdateInterview)
	r.Delete("/api/interviews/{id}", h.DeleteInterview)
	r.Get("/api/applications/{id}/contacts", h.ListContacts)
	r.Post("/api/applications/{id}/contacts", h.CreateContact)
	r.Get("/api/contacts/{id}", h.GetContact)
	r.Put("/api/contacts/{id}", h.UpdateContact)
	r.Delete("/api/contacts/{id}", h.DeleteContact)
	r.Get("/api/stats", h.GetStats)
	r.Get("/api/activity", h.GetActivityByDay)
	r.Get("/api/export", h.Export)
	r.Post("/api/import", h.Import)
	r.Get("/api/export/csv", h.ExportCSV)

	ts := &testServer{handler: r, h: h, dbPath: f.Name()}
	t.Cleanup(func() {
		database.Close()
		os.Remove(ts.dbPath)
		os.Remove(ts.dbPath + "-shm")
		os.Remove(ts.dbPath + "-wal")
	})
	return ts
}

func (ts *testServer) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&reqBody).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, w.Body.String())
	}
	return v
}

type listResponse struct {
	Data       []models.Application `json:"data"`
	Total      int                  `json:"total"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	TotalPages int                  `json:"total_pages"`
}

func createApp(t *testing.T, ts *testServer, overrides map[string]any) models.Application {
	t.Helper()
	payload := map[string]any{
		"company_name":  "TestCo",
		"job_title":     "Dev",
		"contract_type": "CDI",
		"work_mode":     "Remote",
	}
	for k, v := range overrides {
		payload[k] = v
	}
	w := ts.do(t, "POST", "/api/applications", payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("createApp: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	return decode[models.Application](t, w)
}

// --- Application CRUD ---

func TestCreateApplication(t *testing.T) {
	ts := newTestServer(t)

	payload := map[string]any{
		"company_name":  "Acme Corp",
		"job_title":     "Backend Engineer",
		"contract_type": "CDI",
		"work_mode":     "Remote",
	}
	w := ts.do(t, "POST", "/api/applications", payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	app := decode[models.Application](t, w)
	if app.ID == "" {
		t.Error("expected non-empty ID")
	}
	if app.CompanyName != "Acme Corp" {
		t.Errorf("expected company_name=Acme Corp, got %q", app.CompanyName)
	}
	if app.Status != models.StatusWishlist {
		t.Errorf("expected default status=Wishlist, got %q", app.Status)
	}
	if app.SalaryCurrency != "EUR" {
		t.Errorf("expected default currency=EUR, got %q", app.SalaryCurrency)
	}
}

func TestGetApplication(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "TestCo", "job_title": "SWE"})

	w := ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := decode[models.Application](t, w)
	if got.ID != app.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, app.ID)
	}
}

func TestGetApplicationNotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/applications/nonexistent-id", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListApplications_EmptyDB(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/applications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 apps on empty DB, got %d", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("expected page=1, got %d", resp.Page)
	}
}

func TestListApplications_Filter(t *testing.T) {
	ts := newTestServer(t)

	for _, s := range []string{"Applied", "Wishlist", "Applied"} {
		createApp(t, ts, map[string]any{"status": s})
	}

	w := ts.do(t, "GET", "/api/applications?status=Applied", nil)
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 Applied apps, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}

	w = ts.do(t, "GET", "/api/applications?status=Wishlist", nil)
	resp = decode[listResponse](t, w)
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 Wishlist app, got %d", len(resp.Data))
	}
}

func TestListApplications_Search(t *testing.T) {
	ts := newTestServer(t)

	createApp(t, ts, map[string]any{"company_name": "Alphabet", "job_title": "SRE"})
	createApp(t, ts, map[string]any{"company_name": "Beta Corp", "job_title": "Backend Engineer"})

	w := ts.do(t, "GET", "/api/applications?search=Alphabet", nil)
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 match for search=Alphabet, got %d", len(resp.Data))
	}
}

func TestListApplications_Pagination(t *testing.T) {
	ts := newTestServer(t)

	for i := 0; i < 5; i++ {
		createApp(t, ts, map[string]any{"company_name": "Co"})
	}

	w := ts.do(t, "GET", "/api/applications?per_page=2&page=1", nil)
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 2 {
		t.Errorf("page 1: expected 2 items, got %d", len(resp.Data))
	}
	if resp.Total != 5 {
		t.Errorf("expected total=5, got %d", resp.Total)
	}
	if resp.TotalPages != 3 {
		t.Errorf("expected total_pages=3, got %d", resp.TotalPages)
	}
	if resp.Page != 1 {
		t.Errorf("expected page=1, got %d", resp.Page)
	}

	w = ts.do(t, "GET", "/api/applications?per_page=2&page=3", nil)
	resp = decode[listResponse](t, w)
	if len(resp.Data) != 1 {
		t.Errorf("page 3: expected 1 item, got %d", len(resp.Data))
	}
}

func TestUpdateApplication(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "OldCo", "job_title": "Dev"})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "NewCo", "job_title": "Senior Dev", "status": "Applied",
		"contract_type": "CDI", "work_mode": "Remote",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	updated := decode[models.Application](t, w)
	if updated.CompanyName != "NewCo" {
		t.Errorf("expected NewCo, got %q", updated.CompanyName)
	}
	if updated.Status != "Applied" {
		t.Errorf("expected Applied status, got %q", updated.Status)
	}
}

func TestUpdateApplication_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "PUT", "/api/applications/nonexistent", map[string]any{
		"company_name": "X", "job_title": "Y", "contract_type": "CDI", "work_mode": "Remote", "status": "Wishlist",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent update, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateApplication_StatusChangeCreatesTimeline(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Wishlist"})

	ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev",
		"contract_type": "CDI", "work_mode": "Remote", "status": "Applied",
	})

	w := ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, w)

	// Should have "created" + "status_change" events
	if len(got.TimelineEvents) < 2 {
		t.Fatalf("expected at least 2 timeline events, got %d", len(got.TimelineEvents))
	}
	found := false
	for _, e := range got.TimelineEvents {
		if e.EventType == "status_change" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a status_change timeline event")
	}
}

func TestDeleteApplication(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "ToDelete"})

	w := ts.do(t, "DELETE", "/api/applications/"+app.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	w = ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}

// --- Validation ---

func TestCreateApplication_ValidationErrors(t *testing.T) {
	ts := newTestServer(t)

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{"missing company_name", map[string]any{"job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote"}},
		{"missing job_title", map[string]any{"company_name": "Co", "contract_type": "CDI", "work_mode": "Remote"}},
		{"invalid contract_type", map[string]any{"company_name": "Co", "job_title": "Dev", "contract_type": "INVALID", "work_mode": "Remote"}},
		{"invalid work_mode", map[string]any{"company_name": "Co", "job_title": "Dev", "contract_type": "CDI", "work_mode": "INVALID"}},
		{"invalid status", map[string]any{"company_name": "Co", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote", "status": "INVALID"}},
		{"rating too low", map[string]any{"company_name": "Co", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote", "rating": 0}},
		{"rating too high", map[string]any{"company_name": "Co", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote", "rating": 6}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := ts.do(t, "POST", "/api/applications", tt.payload)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// --- Full CRUD lifecycle ---

func TestFullCRUDLifecycle(t *testing.T) {
	ts := newTestServer(t)

	app := createApp(t, ts, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer", "status": "Wishlist",
	})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Phone",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create interview: expected 201, got %d", w.Code)
	}
	iv := decode[models.Interview](t, w)
	if iv.ApplicationID != app.ID {
		t.Errorf("interview app ID mismatch")
	}

	ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer",
		"contract_type": "CDI", "work_mode": "Hybrid", "status": "Applied",
	})

	ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer",
		"contract_type": "CDI", "work_mode": "Hybrid", "status": "Interviewing",
	})

	w = ts.do(t, "GET", "/api/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stats: expected 200, got %d", w.Code)
	}
	stats := decode[models.Stats](t, w)
	if stats.Total != 1 {
		t.Errorf("expected 1 total app, got %d", stats.Total)
	}
	if stats.ByStatus["Interviewing"] != 1 {
		t.Errorf("expected 1 Interviewing, got %v", stats.ByStatus)
	}

	w = ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	full := decode[models.Application](t, w)
	if len(full.Interviews) != 1 {
		t.Errorf("expected 1 interview in detail, got %d", len(full.Interviews))
	}
	// created + interview_added + status_change(Wishlist->Applied) + status_change(Applied->Interviewing)
	if len(full.TimelineEvents) != 4 {
		t.Errorf("expected 4 timeline events, got %d", len(full.TimelineEvents))
	}

	w = ts.do(t, "DELETE", "/api/applications/"+app.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	w = ts.do(t, "GET", "/api/applications", nil)
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 apps after delete, got %d", len(resp.Data))
	}
}

// --- Interviews ---

func TestGetInterview(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "IntCo"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Technical",
	})
	iv := decode[models.Interview](t, w)

	w = ts.do(t, "GET", "/api/interviews/"+iv.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got := decode[models.Interview](t, w)
	if got.ID != iv.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, iv.ID)
	}
	if got.ApplicationID != app.ID {
		t.Errorf("ApplicationID mismatch")
	}
}

func TestGetInterview_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/interviews/nonexistent-id", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInterview_ValidationErrors(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "ValCo"})

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{"invalid type", map[string]any{"round": 1, "type": "INVALID"}},
		{"invalid outcome", map[string]any{"round": 1, "type": "Phone", "outcome": "INVALID"}},
		{"negative round", map[string]any{"round": -1, "type": "Phone"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", tt.payload)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestUpdateInterview_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "PUT", "/api/interviews/nonexistent", map[string]any{
		"round": 1, "type": "Phone",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInterviewCRUD(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "IntCo"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Technical",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	iv := decode[models.Interview](t, w)

	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/interviews", nil)
	ivs := decode[[]models.Interview](t, w)
	if len(ivs) != 1 {
		t.Errorf("expected 1 interview, got %d", len(ivs))
	}

	w = ts.do(t, "PUT", "/api/interviews/"+iv.ID, map[string]any{
		"round": 1, "type": "Technical", "outcome": "Passed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update interview: expected 200, got %d", w.Code)
	}

	w = ts.do(t, "DELETE", "/api/interviews/"+iv.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete interview: expected 204, got %d", w.Code)
	}

	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/interviews", nil)
	ivs = decode[[]models.Interview](t, w)
	if len(ivs) != 0 {
		t.Errorf("expected 0 interviews after delete, got %d", len(ivs))
	}
}

// --- Contacts ---

func TestGetContact(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "ContactCo"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{
		"name": "Bob Smith",
	})
	c := decode[models.Contact](t, w)

	w = ts.do(t, "GET", "/api/contacts/"+c.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got := decode[models.Contact](t, w)
	if got.ID != c.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, c.ID)
	}
	if got.Name != "Bob Smith" {
		t.Errorf("expected Bob Smith, got %q", got.Name)
	}
}

func TestGetContact_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/contacts/nonexistent-id", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestContact_ValidationErrors(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "ValCo"})

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{"missing name", map[string]any{"role": "HR"}},
		{"empty name", map[string]any{"name": "   ", "role": "HR"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", tt.payload)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestUpdateContact_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "PUT", "/api/contacts/nonexistent", map[string]any{"name": "X"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestContactCRUD(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "ContactCo", "contract_type": "CDD", "work_mode": "On-site"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{
		"name": "Alice Martin", "role": "HR Manager", "email": "alice@contactco.com",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	c := decode[models.Contact](t, w)

	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/contacts", nil)
	contacts := decode[[]models.Contact](t, w)
	if len(contacts) != 1 {
		t.Errorf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Name != "Alice Martin" {
		t.Errorf("expected Alice Martin, got %q", contacts[0].Name)
	}

	w = ts.do(t, "PUT", "/api/contacts/"+c.ID, map[string]any{
		"name": "Alice Dupont", "role": "Recruiter",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update contact: expected 200, got %d", w.Code)
	}

	w = ts.do(t, "DELETE", "/api/contacts/"+c.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete contact: expected 204, got %d", w.Code)
	}

	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/contacts", nil)
	contacts = decode[[]models.Contact](t, w)
	if len(contacts) != 0 {
		t.Errorf("expected 0 contacts after delete, got %d", len(contacts))
	}
}

// --- Stats ---

func TestStats_Empty(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	stats := decode[models.Stats](t, w)
	if stats.Total != 0 {
		t.Errorf("expected 0 total, got %d", stats.Total)
	}
	if stats.ResponseRate != 0 {
		t.Errorf("expected 0 response rate, got %f", stats.ResponseRate)
	}
}

func TestStats_ResponseRate(t *testing.T) {
	ts := newTestServer(t)

	for _, s := range []string{"Applied", "Applied", "Screening", "Rejected"} {
		createApp(t, ts, map[string]any{"status": s})
	}

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	if stats.Total != 4 {
		t.Errorf("expected 4 total, got %d", stats.Total)
	}
	// Applied: 2, responded (Screening+Rejected): 2 — response rate = 2/(2+2)*100 = 50%
	expectedRate := 50.0
	if stats.ResponseRate != expectedRate {
		t.Errorf("expected response rate %.1f, got %.1f", expectedRate, stats.ResponseRate)
	}
}

// --- Export ---

func TestExport(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "ExportCo"})

	w := ts.do(t, "GET", "/api/export", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if _, ok := result["applications"]; !ok {
		t.Error("export missing 'applications' key")
	}
	if _, ok := result["exported_at"]; !ok {
		t.Error("export missing 'exported_at' key")
	}
	apps, _ := result["applications"].([]any)
	if len(apps) != 1 {
		t.Errorf("expected 1 app in export, got %d", len(apps))
	}
}

// --- Edge cases ---

func TestSpecialCharacters(t *testing.T) {
	ts := newTestServer(t)

	special := `Acme & Co. — "Top" Employer <Paris> 100% 'remote' ñoño`
	app := createApp(t, ts, map[string]any{
		"company_name": special,
		"job_title":    "L'ingénieur logiciel — 'senior'",
	})

	w := ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, w)
	if got.CompanyName != special {
		t.Errorf("special chars not preserved: got %q", got.CompanyName)
	}
}

func TestLargeTextField(t *testing.T) {
	ts := newTestServer(t)

	largeDesc := make([]byte, 50000)
	for i := range largeDesc {
		largeDesc[i] = 'x'
	}

	desc := string(largeDesc)
	app := createApp(t, ts, map[string]any{
		"company_name":    "BigDescCo",
		"job_description": desc,
		"notes":           desc,
	})

	w := ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, w)
	if got.JobDescription == nil || len(*got.JobDescription) != len(desc) {
		t.Errorf("large job_description not preserved: got len=%d", func() int {
			if got.JobDescription == nil {
				return -1
			}
			return len(*got.JobDescription)
		}())
	}
}

func TestInvalidJSON(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/applications", bytes.NewBufferString("not-json{{{"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestSortByInvalidField(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "SortCo"})

	w := ts.do(t, "GET", "/api/applications?sort=DROP+TABLE", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with invalid sort (fallback), got %d", w.Code)
	}
}

func TestDeleteApplication_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "DELETE", "/api/applications/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteInterview_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "DELETE", "/api/interviews/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteContact_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "DELETE", "/api/contacts/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAutoAppliedAt(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Wishlist"})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev",
		"contract_type": "CDI", "work_mode": "Remote", "status": "Applied",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	updated := decode[models.Application](t, w)
	if updated.AppliedAt == nil {
		t.Error("expected applied_at to be auto-set when status changes to Applied")
	}
}

func TestAutoAppliedAt_NoOverwrite(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": "2025-01-15T10:00:00Z"})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev",
		"contract_type": "CDI", "work_mode": "Remote", "status": "Screening",
		"applied_at": "2025-01-15T10:00:00Z",
	})
	updated := decode[models.Application](t, w)

	w = ts.do(t, "PUT", "/api/applications/"+updated.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev",
		"contract_type": "CDI", "work_mode": "Remote", "status": "Applied",
		"applied_at": "2025-01-15T10:00:00Z",
	})
	final := decode[models.Application](t, w)
	if final.AppliedAt == nil {
		t.Fatal("expected applied_at to be preserved")
	}
}

func TestInterviewTimelineEvents(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "TimelineCo"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Phone",
	})
	iv := decode[models.Interview](t, w)

	ts.do(t, "DELETE", "/api/interviews/"+iv.ID, nil)

	w = ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, w)

	types := map[string]bool{}
	for _, e := range got.TimelineEvents {
		types[e.EventType] = true
	}
	if !types["interview_added"] {
		t.Error("expected interview_added timeline event")
	}
	if !types["interview_deleted"] {
		t.Error("expected interview_deleted timeline event")
	}
}

func TestContactTimelineEvents(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "TimelineCo"})

	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{
		"name": "Alice",
	})
	c := decode[models.Contact](t, w)

	ts.do(t, "DELETE", "/api/contacts/"+c.ID, nil)

	w = ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, w)

	types := map[string]bool{}
	for _, e := range got.TimelineEvents {
		types[e.EventType] = true
	}
	if !types["contact_added"] {
		t.Error("expected contact_added timeline event")
	}
	if !types["contact_deleted"] {
		t.Error("expected contact_deleted timeline event")
	}
}

func TestListApplications_Counts(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "CountCo"})

	ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{"round": 1, "type": "Phone"})
	ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{"round": 2, "type": "Technical"})
	ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{"name": "Alice"})

	w := ts.do(t, "GET", "/api/applications", nil)
	var resp struct {
		Data []struct {
			ID             string `json:"id"`
			InterviewCount int    `json:"interview_count"`
			ContactCount   int    `json:"contact_count"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 app, got %d", len(resp.Data))
	}
	if resp.Data[0].InterviewCount != 2 {
		t.Errorf("expected interview_count=2, got %d", resp.Data[0].InterviewCount)
	}
	if resp.Data[0].ContactCount != 1 {
		t.Errorf("expected contact_count=1, got %d", resp.Data[0].ContactCount)
	}
}

func TestListApplications_SearchLocationSource(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Co1", "location": "Paris"})
	createApp(t, ts, map[string]any{"company_name": "Co2", "source": "LinkedIn"})
	createApp(t, ts, map[string]any{"company_name": "Co3", "location": "Lyon"})

	w := ts.do(t, "GET", "/api/applications?search=Paris", nil)
	resp := decode[listResponse](t, w)
	if resp.Total != 1 {
		t.Errorf("search by location: expected 1, got %d", resp.Total)
	}

	w = ts.do(t, "GET", "/api/applications?search=LinkedIn", nil)
	resp = decode[listResponse](t, w)
	if resp.Total != 1 {
		t.Errorf("search by source: expected 1, got %d", resp.Total)
	}
}

func TestListApplications_SortDirection(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Alpha"})
	createApp(t, ts, map[string]any{"company_name": "Zulu"})

	w := ts.do(t, "GET", "/api/applications?sort=company_name&dir=asc", nil)
	resp := decode[listResponse](t, w)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(resp.Data))
	}
	if resp.Data[0].CompanyName != "Alpha" {
		t.Errorf("expected Alpha first with asc sort, got %q", resp.Data[0].CompanyName)
	}

	w = ts.do(t, "GET", "/api/applications?sort=company_name&dir=desc", nil)
	resp = decode[listResponse](t, w)
	if resp.Data[0].CompanyName != "Zulu" {
		t.Errorf("expected Zulu first with desc sort, got %q", resp.Data[0].CompanyName)
	}
}

func TestListApplications_SortByRating(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Low", "rating": 1})
	createApp(t, ts, map[string]any{"company_name": "High", "rating": 5})

	w := ts.do(t, "GET", "/api/applications?sort=rating&dir=desc", nil)
	resp := decode[listResponse](t, w)
	if resp.Data[0].CompanyName != "High" {
		t.Errorf("expected High first sorted by rating desc, got %q", resp.Data[0].CompanyName)
	}
}

// --- Import tests ---

func TestImport_Empty(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "POST", "/api/import", map[string]any{"applications": []any{}})
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImport_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/import", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImport_Single(t *testing.T) {
	ts := newTestServer(t)
	payload := map[string]any{
		"applications": []map[string]any{
			{"company_name": "ImportCo", "job_title": "Dev"},
		},
	}
	w := ts.do(t, "POST", "/api/import", payload)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decode[map[string]any](t, w)
	if int(result["imported"].(float64)) != 1 {
		t.Errorf("expected 1 imported, got %v", result["imported"])
	}

	list := ts.do(t, "GET", "/api/applications", nil)
	resp := decode[listResponse](t, list)
	if resp.Total != 1 || resp.Data[0].CompanyName != "ImportCo" {
		t.Errorf("expected ImportCo, got %v", resp)
	}
}

func TestImport_WithRelations(t *testing.T) {
	ts := newTestServer(t)
	payload := map[string]any{
		"applications": []map[string]any{
			{
				"id": "test-import-123", "company_name": "RelCo", "job_title": "Lead",
				"interviews": []map[string]any{
					{"round": 1, "type": "Phone"},
				},
				"contacts": []map[string]any{
					{"name": "Jane Doe"},
				},
				"timeline_events": []map[string]any{
					{"event_type": "created", "description": "Application created"},
				},
			},
		},
	}
	w := ts.do(t, "POST", "/api/import", payload)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	detail := ts.do(t, "GET", "/api/applications/test-import-123", nil)
	app := decode[models.Application](t, detail)
	if len(app.Interviews) != 1 {
		t.Errorf("expected 1 interview, got %d", len(app.Interviews))
	}
	if len(app.Contacts) != 1 {
		t.Errorf("expected 1 contact, got %d", len(app.Contacts))
	}
	if len(app.TimelineEvents) != 1 {
		t.Errorf("expected 1 timeline event, got %d", len(app.TimelineEvents))
	}
}

func TestImport_DuplicateSkip(t *testing.T) {
	ts := newTestServer(t)
	payload := map[string]any{
		"applications": []map[string]any{
			{"id": "dup-id-1", "company_name": "DupCo", "job_title": "Dev"},
		},
	}

	w1 := ts.do(t, "POST", "/api/import", payload)
	r1 := decode[map[string]any](t, w1)
	if int(r1["imported"].(float64)) != 1 {
		t.Errorf("first import: expected 1 imported, got %v", r1["imported"])
	}

	w2 := ts.do(t, "POST", "/api/import", payload)
	r2 := decode[map[string]any](t, w2)
	if int(r2["skipped"].(float64)) != 1 {
		t.Errorf("second import: expected 1 skipped, got %v", r2["skipped"])
	}
	if int(r2["imported"].(float64)) != 0 {
		t.Errorf("second import: expected 0 imported, got %v", r2["imported"])
	}
}

func TestImport_RoundTrip(t *testing.T) {
	ts := newTestServer(t)

	app := ts.do(t, "POST", "/api/applications", map[string]any{
		"company_name": "RoundTrip Inc", "job_title": "Engineer", "status": "Applied",
	})
	created := decode[models.Application](t, app)

	ts.do(t, "POST", "/api/applications/"+created.ID+"/interviews", map[string]any{
		"round": 1, "type": "Phone",
	})
	ts.do(t, "POST", "/api/applications/"+created.ID+"/contacts", map[string]any{
		"name": "Bob",
	})

	exportW := ts.do(t, "GET", "/api/export", nil)
	var exportData map[string]any
	json.NewDecoder(exportW.Body).Decode(&exportData)

	ts2 := newTestServer(t)
	importW := ts2.do(t, "POST", "/api/import", exportData)
	if importW.Code != 200 {
		t.Fatalf("import failed: %d %s", importW.Code, importW.Body.String())
	}
	result := decode[map[string]any](t, importW)
	if int(result["imported"].(float64)) != 1 {
		t.Errorf("expected 1 imported, got %v", result["imported"])
	}

	list := ts2.do(t, "GET", "/api/applications", nil)
	resp := decode[listResponse](t, list)
	if resp.Total != 1 || resp.Data[0].CompanyName != "RoundTrip Inc" {
		t.Errorf("round-trip data mismatch: %+v", resp)
	}
}

// --- CSV Export tests ---

func TestExportCSV_Empty(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/export/csv", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("expected text/csv, got %q", ct)
	}
	// BOM (3 bytes) + header row only
	lines := strings.Split(strings.TrimSpace(w.Body.String()[3:]), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d", len(lines))
	}
}

func TestExportCSV_WithData(t *testing.T) {
	ts := newTestServer(t)
	ts.do(t, "POST", "/api/applications", map[string]any{
		"company_name": "CSV Co", "job_title": "Dev", "salary": 50000,
	})
	ts.do(t, "POST", "/api/applications", map[string]any{
		"company_name": "Another Co", "job_title": "Lead",
	})

	w := ts.do(t, "GET", "/api/export/csv", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()[3:]), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}
	if disp := w.Header().Get("Content-Disposition"); disp == "" {
		t.Error("expected Content-Disposition header")
	}
}

func TestExportCSV_SpecialChars(t *testing.T) {
	ts := newTestServer(t)
	ts.do(t, "POST", "/api/applications", map[string]any{
		"company_name": `Acme, Inc. "Best"`, "job_title": "Dev",
	})

	w := ts.do(t, "GET", "/api/export/csv", nil)
	body := w.Body.String()[3:] // skip BOM
	if !strings.Contains(body, `"Acme, Inc. ""Best"""`) {
		t.Errorf("CSV escaping failed, got: %s", body)
	}
}

// --- Snooze / Follow-up tests ---

func TestSnoozeFollowUp(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Interviewing"})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{
		"until": "2099-01-01",
	})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	detail := ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	got := decode[models.Application](t, detail)
	if got.FollowUpSnoozedUntil == nil {
		t.Error("expected follow_up_snoozed_until to be set")
	}
}

func TestSnoozeFollowUp_Skip(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Screening"})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{"skip": true})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSnoozeFollowUp_NotFound(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "PUT", "/api/applications/nonexistent/snooze", map[string]any{"skip": true})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSnoozeFollowUp_InvalidBody(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Interviewing"})
	w := ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStats_FollowUps(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Interviewing"})

	ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Phone",
		"scheduled_at": time.Now().AddDate(0, 0, -15).Format(time.RFC3339),
	})

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	if len(stats.FollowUps) != 1 {
		t.Errorf("expected 1 follow-up, got %d", len(stats.FollowUps))
	}
}

func TestStats_FollowUps_SnoozedExcluded(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Interviewing"})

	ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Phone",
		"scheduled_at": time.Now().AddDate(0, 0, -15).Format(time.RFC3339),
	})

	ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{"until": "2099-01-01"})

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	if len(stats.FollowUps) != 0 {
		t.Errorf("expected 0 follow-ups after snooze, got %d", len(stats.FollowUps))
	}
}

// --- Duplicate detection tests ---

func TestCheckDuplicates_NoMatch(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Acme Corp"})
	w := ts.do(t, "GET", "/api/applications/duplicates?company_name=NonExistent", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var results []map[string]any
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCheckDuplicates_CaseInsensitive(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Acme Corp", "job_title": "Dev"})
	w := ts.do(t, "GET", "/api/applications/duplicates?company_name=acme+corp", nil)
	var results []map[string]any
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Errorf("expected 1 result (case-insensitive), got %d", len(results))
	}
}

func TestCheckDuplicates_Multiple(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Google", "job_title": "SRE"})
	createApp(t, ts, map[string]any{"company_name": "Google", "job_title": "SWE"})
	w := ts.do(t, "GET", "/api/applications/duplicates?company_name=Google", nil)
	var results []map[string]any
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestCheckDuplicates_EmptyParam(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "GET", "/api/applications/duplicates?company_name=", nil)
	var results []map[string]any
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty param, got %d", len(results))
	}
}

// --- Bulk actions tests ---

func TestBulkUpdateStatus(t *testing.T) {
	ts := newTestServer(t)
	a1 := createApp(t, ts, map[string]any{"company_name": "A", "status": "Applied"})
	a2 := createApp(t, ts, map[string]any{"company_name": "B", "status": "Applied"})
	createApp(t, ts, map[string]any{"company_name": "C", "status": "Applied"})

	w := ts.do(t, "PUT", "/api/applications/bulk/status", map[string]any{
		"ids": []string{a1.ID, a2.ID}, "status": "Rejected",
	})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decode[map[string]any](t, w)
	if int(result["updated"].(float64)) != 2 {
		t.Errorf("expected 2 updated, got %v", result["updated"])
	}

	detail := ts.do(t, "GET", "/api/applications/"+a1.ID, nil)
	app := decode[models.Application](t, detail)
	if app.Status != "Rejected" {
		t.Errorf("expected Rejected, got %s", app.Status)
	}
}

func TestBulkUpdateStatus_InvalidStatus(t *testing.T) {
	ts := newTestServer(t)
	a := createApp(t, ts, map[string]any{"company_name": "X"})
	w := ts.do(t, "PUT", "/api/applications/bulk/status", map[string]any{
		"ids": []string{a.ID}, "status": "InvalidStatus",
	})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestBulkDelete(t *testing.T) {
	ts := newTestServer(t)
	a1 := createApp(t, ts, map[string]any{"company_name": "A"})
	a2 := createApp(t, ts, map[string]any{"company_name": "B"})
	createApp(t, ts, map[string]any{"company_name": "C"})

	w := ts.do(t, "DELETE", "/api/applications/bulk", map[string]any{
		"ids": []string{a1.ID, a2.ID},
	})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decode[map[string]any](t, w)
	if int(result["deleted"].(float64)) != 2 {
		t.Errorf("expected 2 deleted, got %v", result["deleted"])
	}

	list := ts.do(t, "GET", "/api/applications", nil)
	resp := decode[listResponse](t, list)
	if resp.Total != 1 {
		t.Errorf("expected 1 remaining, got %d", resp.Total)
	}
}

func TestBulkDelete_EmptyIds(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, "DELETE", "/api/applications/bulk", map[string]any{"ids": []string{}})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Stats: period-dependent metrics (see internal/handlers/stats_period.go) ---

// daysAgo / daysAhead produce RFC3339 timestamps relative to now, the format
// the API accepts for applied_at / scheduled_at.
func daysAgo(n int) string {
	return time.Now().UTC().AddDate(0, 0, -n).Format(time.RFC3339)
}

func daysAhead(n int) string {
	return time.Now().UTC().AddDate(0, 0, n).Format(time.RFC3339)
}

// putApp does a full-replacement PUT, carrying over the fields the stats code
// reads (status, source, applied_at) unless they are overridden.
func putApp(t *testing.T, ts *testServer, app models.Application, overrides map[string]any) models.Application {
	t.Helper()
	payload := map[string]any{
		"company_name":  app.CompanyName,
		"job_title":     app.JobTitle,
		"contract_type": app.ContractType,
		"work_mode":     app.WorkMode,
		"status":        app.Status,
	}
	if app.Source != nil {
		payload["source"] = *app.Source
	}
	if app.AppliedAt != nil {
		payload["applied_at"] = app.AppliedAt.UTC().Format(time.RFC3339)
	}
	for k, v := range overrides {
		payload[k] = v
	}
	w := ts.do(t, "PUT", "/api/applications/"+app.ID, payload)
	if w.Code != http.StatusOK {
		t.Fatalf("putApp %s: expected 200, got %d: %s", app.ID, w.Code, w.Body.String())
	}
	got := decode[models.Application](t, w)
	got.ID = app.ID
	return got
}

func getStats(t *testing.T, ts *testServer, query string) models.Stats {
	t.Helper()
	w := ts.do(t, "GET", "/api/stats"+query, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats%s: expected 200, got %d: %s", query, w.Code, w.Body.String())
	}
	return decode[models.Stats](t, w)
}

func assertFloat(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tolerance %v)", label, got, want, tol)
	}
}

func assertPrev(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s.prev = nil, want %v", label, want)
	}
	assertFloat(t, label+".prev", *got, want, 1e-9)
}

func sumFloats(xs []float64) float64 {
	var total float64
	for _, x := range xs {
		total += x
	}
	return total
}

func TestStats_PeriodDefaults(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(10)})
	createApp(t, ts, map[string]any{"status": "Screening", "applied_at": daysAgo(200)})

	tests := []struct {
		name     string
		query    string
		wantDays int
		wantPrev bool
	}{
		{"no period param defaults to 90 days", "", 90, true},
		{"empty period param defaults to 90 days", "?period=", 90, true},
		{"unknown period falls back to 90 days", "?period=bogus", 90, true},
		{"unsupported number falls back to 90 days", "?period=7", 90, true},
		{"30 days", "?period=30", 30, true},
		{"90 days", "?period=90", 90, true},
		{"365 days", "?period=365", 365, true},
		{"all time has no previous window", "?period=all", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stats := getStats(t, ts, tc.query)

			if stats.Period.Days != tc.wantDays {
				t.Errorf("period.days = %d, want %d", stats.Period.Days, tc.wantDays)
			}
			kpis := []struct {
				name string
				kpi  models.KPI
			}{
				{"sent", stats.Period.Sent},
				{"response_rate", stats.Period.ResponseRate},
				{"interviews", stats.Period.Interviews},
				{"rejected", stats.Period.Rejected},
				{"offers", stats.Period.Offers},
			}
			for _, k := range kpis {
				if len(k.kpi.Series) != 8 {
					t.Errorf("period.%s.series has %d entries, want 8", k.name, len(k.kpi.Series))
				}
				switch {
				case tc.wantPrev && k.kpi.Prev == nil:
					t.Errorf("period.%s.prev = nil, want a value for a %d-day window", k.name, tc.wantDays)
				case !tc.wantPrev && k.kpi.Prev != nil:
					t.Errorf("period.%s.prev = %v, want nil for all-time", k.name, *k.kpi.Prev)
				}
			}
			if len(stats.Weekly) != 12 {
				t.Errorf("weekly has %d points, want 12", len(stats.Weekly))
			}
		})
	}
}

func TestStats_PeriodDefaults_Empty(t *testing.T) {
	ts := newTestServer(t)

	stats := getStats(t, ts, "?period=all")
	if stats.Period.Days != 0 {
		t.Errorf("period.days = %d, want 0", stats.Period.Days)
	}
	if stats.Period.Sent.Prev != nil {
		t.Errorf("period.sent.prev = %v, want nil", *stats.Period.Sent.Prev)
	}
	if len(stats.Period.Sent.Series) != 8 {
		t.Errorf("period.sent.series has %d entries, want 8", len(stats.Period.Sent.Series))
	}
	if stats.Period.Sent.Value != 0 {
		t.Errorf("period.sent.value = %v, want 0", stats.Period.Sent.Value)
	}
	if stats.Period.Funnel != (models.FunnelStats{}) {
		t.Errorf("funnel = %+v, want zero value", stats.Period.Funnel)
	}
}

func TestStats_PeriodCohort(t *testing.T) {
	ts := newTestServer(t)

	// The cohort is defined by when an application was sent (applied_at), and
	// counted by its *current* status.
	seed := []struct {
		status string
		days   int
	}{
		// Current 30-day window.
		{"Applied", 10},
		{"Applied", 10},
		{"Screening", 10},
		{"Interviewing", 10},
		{"Offer", 10},
		{"Accepted", 10},
		{"Rejected", 10},
		{"Wishlist", 10}, // never sent
		// Previous 30-day window ([-60d, -30d)).
		{"Applied", 50},
		{"Rejected", 50},
		{"Offer", 50},
		{"Wishlist", 50},
		// Outside both windows.
		{"Applied", 120},
		{"Rejected", 120},
	}
	for _, s := range seed {
		createApp(t, ts, map[string]any{
			"status":     s.status,
			"applied_at": daysAgo(s.days),
		})
	}

	t.Run("30 day window", func(t *testing.T) {
		p := getStats(t, ts, "?period=30").Period
		if p.Days != 30 {
			t.Fatalf("period.days = %d, want 30", p.Days)
		}

		// Sent: 2 Applied + Screening + Interviewing + Offer + Accepted + Rejected.
		// Wishlist is not "sent".
		assertFloat(t, "sent.value", p.Sent.Value, 7, 0)
		assertPrev(t, "sent", p.Sent.Prev, 3)
		assertFloat(t, "sum(sent.series)", sumFloats(p.Sent.Series), 7, 0)

		// Responded = Screening/Interviewing/Offer/Accepted/Rejected = 5 of 7.
		assertFloat(t, "response_rate.value", p.ResponseRate.Value, 500.0/7.0, 1e-9)
		assertPrev(t, "response_rate", p.ResponseRate.Prev, 200.0/3.0)

		assertFloat(t, "rejected.value", p.Rejected.Value, 1, 0)
		assertPrev(t, "rejected", p.Rejected.Prev, 1)

		// Replies received: the 5 responded statuses now, 2 (Rejected + Offer) before.
		assertFloat(t, "responded.value", p.Responded.Value, 5, 0)
		assertPrev(t, "responded", p.Responded.Prev, 2)
		assertFloat(t, "sum(responded.series)", sumFloats(p.Responded.Series), 5, 0)

		// Offers = Offer + Accepted.
		assertFloat(t, "offers.value", p.Offers.Value, 2, 0)
		assertPrev(t, "offers", p.Offers.Prev, 1)

		// No interviews were created in this test.
		assertFloat(t, "interviews.value", p.Interviews.Value, 0, 0)
		assertPrev(t, "interviews", p.Interviews.Prev, 0)

		want := models.FunnelStats{Sent: 7, Responded: 5, Interviewing: 3, Offers: 2, Accepted: 1, Pending: 2}
		if p.Funnel != want {
			t.Errorf("funnel = %+v, want %+v", p.Funnel, want)
		}
	})

	t.Run("all time window", func(t *testing.T) {
		p := getStats(t, ts, "?period=all").Period
		if p.Days != 0 {
			t.Fatalf("period.days = %d, want 0", p.Days)
		}
		// Every sent application, whichever window it fell in: 7 + 3 + 2.
		assertFloat(t, "sent.value", p.Sent.Value, 12, 0)
		if p.Sent.Prev != nil {
			t.Errorf("sent.prev = %v, want nil for all-time", *p.Sent.Prev)
		}
		assertFloat(t, "rejected.value", p.Rejected.Value, 3, 0)
		assertFloat(t, "offers.value", p.Offers.Value, 3, 0)
		// Responded: 5 (current) + 2 (previous) + 1 (older Rejected) = 8 of 12.
		assertFloat(t, "response_rate.value", p.ResponseRate.Value, 800.0/12.0, 1e-9)
		want := models.FunnelStats{Sent: 12, Responded: 8, Interviewing: 4, Offers: 3, Accepted: 1, Pending: 4}
		if p.Funnel != want {
			t.Errorf("funnel = %+v, want %+v", p.Funnel, want)
		}
	})

	t.Run("wishlist applications never count as sent", func(t *testing.T) {
		p := getStats(t, ts, "?period=all").Period
		total := getStats(t, ts, "?period=all").Total
		if total != len(seed) {
			t.Fatalf("total = %d, want %d", total, len(seed))
		}
		if int(p.Sent.Value) == total {
			t.Errorf("sent.value (%v) should exclude the Wishlist rows out of %d", p.Sent.Value, total)
		}
	})
}

func TestStats_Weekly(t *testing.T) {
	ts := newTestServer(t)

	// Four applications created now as Applied (applied_at is stamped with the
	// creation time), so they all land in the current week.
	a := createApp(t, ts, map[string]any{"company_name": "Alpha", "status": "Applied"})
	b := createApp(t, ts, map[string]any{"company_name": "Bravo", "status": "Applied"})
	createApp(t, ts, map[string]any{"company_name": "Charlie", "status": "Applied"})
	d := createApp(t, ts, map[string]any{"company_name": "Delta", "status": "Applied"})

	t.Run("shape and week starts", func(t *testing.T) {
		weekly := getStats(t, ts, "").Weekly
		if len(weekly) != 12 {
			t.Fatalf("weekly has %d points, want 12", len(weekly))
		}

		now := time.Now().UTC()
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		thisMonday := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))

		if got := weekly[11].WeekStart; got != thisMonday.Format("2006-01-02") {
			t.Errorf("last week_start = %q, want %q", got, thisMonday.Format("2006-01-02"))
		}
		for i, p := range weekly {
			want := thisMonday.AddDate(0, 0, -7*(11-i)).Format("2006-01-02")
			if p.WeekStart != want {
				t.Errorf("weekly[%d].week_start = %q, want %q", i, p.WeekStart, want)
			}
			parsed, err := time.Parse("2006-01-02", p.WeekStart)
			if err != nil {
				t.Fatalf("weekly[%d].week_start %q is not a date: %v", i, p.WeekStart, err)
			}
			if parsed.Weekday() != time.Monday {
				t.Errorf("weekly[%d].week_start %q is a %v, want Monday", i, p.WeekStart, parsed.Weekday())
			}
			if i < 11 && (p.Sent != 0 || p.Replies != 0) {
				t.Errorf("weekly[%d] (%s) = sent %d / replies %d, want zero-filled", i, p.WeekStart, p.Sent, p.Replies)
			}
		}
		if weekly[11].Sent != 4 {
			t.Errorf("current week sent = %d, want 4", weekly[11].Sent)
		}
		if weekly[11].Replies != 0 {
			t.Errorf("current week replies = %d, want 0 before any status change", weekly[11].Replies)
		}
	})

	t.Run("replies come from status_change events", func(t *testing.T) {
		// Alpha: Applied -> Screening -> Interviewing. Two status changes in the
		// same week, but only one reply (one conversation).
		a2 := putApp(t, ts, a, map[string]any{"status": "Screening"})
		putApp(t, ts, a2, map[string]any{"status": "Interviewing"})
		// Bravo: a rejection is still a reply.
		putApp(t, ts, b, map[string]any{"status": "Rejected"})
		// Delta: back to the wishlist is not a reply, and drops it out of "sent".
		putApp(t, ts, d, map[string]any{"status": "Wishlist"})

		weekly := getStats(t, ts, "").Weekly
		cur := weekly[len(weekly)-1]
		if cur.Replies != 2 {
			t.Errorf("current week replies = %d, want 2 (Alpha counted once, Bravo once, Delta not at all)", cur.Replies)
		}
		if cur.Sent != 3 {
			t.Errorf("current week sent = %d, want 3 (Delta is back in Wishlist)", cur.Sent)
		}
	})
}

func TestStats_UpcomingInterviews(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "InterviewCo", "status": "Interviewing"})

	interviews := []struct {
		name     string
		payload  map[string]any
		upcoming bool
	}{
		{
			name:     "two days ahead counts",
			payload:  map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAhead(2)},
			upcoming: true,
		},
		{
			name:     "twenty days ahead is beyond the 7 day horizon",
			payload:  map[string]any{"round": 2, "type": "Video", "scheduled_at": daysAhead(20)},
			upcoming: false,
		},
		{
			name:     "already happened",
			payload:  map[string]any{"round": 3, "type": "Technical", "scheduled_at": daysAgo(2)},
			upcoming: false,
		},
		{
			name:     "unscheduled falls back to created_at and is never upcoming",
			payload:  map[string]any{"round": 4, "type": "HR"},
			upcoming: false,
		},
		{
			// Same horizon as round 1, but called off: not something to prepare for.
			name:     "cancelled interview inside the horizon is not upcoming",
			payload:  map[string]any{"round": 5, "type": "Phone", "scheduled_at": daysAhead(3), "outcome": "Cancelled"},
			upcoming: false,
		},
		{
			// A cancelled interview never took place: it is not upcoming, and it
			// does not make an application count in the interviews KPI either
			// (see TestStats_InterviewsKPI_CountsApplications).
			name:     "cancelled interview in the past is not upcoming",
			payload:  map[string]any{"round": 6, "type": "Video", "scheduled_at": daysAgo(3), "outcome": "Cancelled"},
			upcoming: false,
		},
	}

	wantUpcoming := 0
	for _, tc := range interviews {
		w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", tc.payload)
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: expected 201, got %d: %s", tc.name, w.Code, w.Body.String())
		}
		iv := decode[models.Interview](t, w)
		if iv.ApplicationID != app.ID {
			t.Errorf("%s: application_id = %q, want %q", tc.name, iv.ApplicationID, app.ID)
		}
		if _, ok := tc.payload["scheduled_at"]; ok && iv.ScheduledAt == nil {
			t.Errorf("%s: scheduled_at round-tripped as nil", tc.name)
		}
		if want, ok := tc.payload["outcome"]; ok {
			if iv.Outcome == nil {
				t.Errorf("%s: outcome round-tripped as nil, want %v", tc.name, want)
			} else if string(*iv.Outcome) != want.(string) {
				t.Errorf("%s: outcome = %q, want %v", tc.name, *iv.Outcome, want)
			}
		}
		if tc.upcoming {
			wantUpcoming++
		}
	}

	stats := getStats(t, ts, "")
	if stats.UpcomingInterviews != wantUpcoming {
		t.Errorf("upcoming_interviews = %d, want %d", stats.UpcomingInterviews, wantUpcoming)
	}
	// The period KPI counts applications that landed at least one interview,
	// not interview rows: all six rounds belong to one application.
	assertFloat(t, "period.interviews.value", stats.Period.Interviews.Value, 1, 0)
	assertPrev(t, "period.interviews", stats.Period.Interviews.Prev, 0)
}

func TestStats_InterviewsKPI_CountsApplications(t *testing.T) {
	ts := newTestServer(t)
	a := createApp(t, ts, map[string]any{"company_name": "Two rounds", "status": "Rejected", "applied_at": daysAgo(20)})
	b := createApp(t, ts, map[string]any{"company_name": "Cancelled only", "status": "Applied", "applied_at": daysAgo(20)})
	c := createApp(t, ts, map[string]any{"company_name": "Old", "status": "Interviewing", "applied_at": daysAgo(50)})
	createApp(t, ts, map[string]any{"company_name": "None", "status": "Applied", "applied_at": daysAgo(5)})
	post := func(id string, body map[string]any) {
		if w := ts.do(t, "POST", "/api/applications/"+id+"/interviews", body); w.Code != http.StatusCreated {
			t.Fatalf("create interview: %d %s", w.Code, w.Body.String())
		}
	}
	post(a.ID, map[string]any{"round": 1, "type": "Screening", "scheduled_at": daysAgo(15), "outcome": "Passed"})
	post(a.ID, map[string]any{"round": 2, "type": "Technical", "scheduled_at": daysAgo(10), "outcome": "Failed"})
	post(b.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(10), "outcome": "Cancelled"})
	post(c.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(40)})

	p := getStats(t, ts, "?period=30").Period
	assertFloat(t, "interviews.value", p.Interviews.Value, 1, 0) // A only: B is cancelled, C is in the previous window
	assertPrev(t, "interviews", p.Interviews.Prev, 1)            // C

	list := decode[listResponse](t, ts.do(t, "GET", "/api/applications?has_interviews=1", nil))
	if list.Total != 2 {
		t.Fatalf("has_interviews list total = %d, want 2 (A and C)", list.Total)
	}
	for _, app := range list.Data {
		if app.ID == b.ID {
			t.Errorf("cancelled-only application must not match has_interviews")
		}
	}
}

// --- Automatic "no reply" transition ---

// getApp fetches one application with its relations (timeline included).
func getApp(t *testing.T, ts *testServer, id string) models.Application {
	t.Helper()
	w := ts.do(t, "GET", "/api/applications/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/applications/%s: expected 200, got %d: %s", id, w.Code, w.Body.String())
	}
	return decode[models.Application](t, w)
}

func markNoReply(t *testing.T, ts *testServer, days int) int {
	t.Helper()
	n, err := ts.h.MarkNoReply(context.Background(), time.Now().UTC(), days)
	if err != nil {
		t.Fatalf("MarkNoReply: %v", err)
	}
	return n
}

func TestCreateApplication_NoReplyStatus(t *testing.T) {
	ts := newTestServer(t)

	app := createApp(t, ts, map[string]any{"status": "NoReply"})
	if app.Status != models.StatusNoReply {
		t.Fatalf("created status = %q, want NoReply", app.Status)
	}

	applied := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(5)})
	updated := putApp(t, ts, applied, map[string]any{"status": "NoReply"})
	if updated.Status != models.StatusNoReply {
		t.Fatalf("updated status = %q, want NoReply", updated.Status)
	}
	if got := getApp(t, ts, applied.ID).Status; got != models.StatusNoReply {
		t.Fatalf("persisted status = %q, want NoReply", got)
	}
}

func TestMarkNoReply(t *testing.T) {
	ts := newTestServer(t)

	stale := createApp(t, ts, map[string]any{"company_name": "Stale", "status": "Applied", "applied_at": daysAgo(40)})
	recent := createApp(t, ts, map[string]any{"company_name": "Recent", "status": "Applied", "applied_at": daysAgo(10)})
	screening := createApp(t, ts, map[string]any{"company_name": "Screening", "status": "Screening", "applied_at": daysAgo(40)})

	if n := markNoReply(t, ts, 30); n != 1 {
		t.Fatalf("MarkNoReply returned %d, want 1", n)
	}

	if got := getApp(t, ts, stale.ID); got.Status != models.StatusNoReply {
		t.Errorf("stale application status = %q, want NoReply", got.Status)
	}
	if got := getApp(t, ts, recent.ID); got.Status != models.StatusApplied {
		t.Errorf("recent application status = %q, want Applied", got.Status)
	}
	if got := getApp(t, ts, screening.ID); got.Status != models.StatusScreening {
		t.Errorf("screening application status = %q, want Screening", got.Status)
	}

	var found int
	for _, e := range getApp(t, ts, stale.ID).TimelineEvents {
		if e.EventType == "status_change" && e.Description == "Status changed from Applied to NoReply" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("timeline has %d \"Applied to NoReply\" events, want 1", found)
	}

	// Idempotent: nothing left to transition on a second run.
	if n := markNoReply(t, ts, 30); n != 0 {
		t.Errorf("second MarkNoReply returned %d, want 0", n)
	}
	if got := len(getApp(t, ts, stale.ID).TimelineEvents); got != 2 {
		t.Errorf("timeline has %d events after second run, want 2 (created + status_change)", got)
	}
}

func TestMarkNoReply_ReapplyRestartsClock(t *testing.T) {
	ts := newTestServer(t)

	app := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(40)})
	// Moving away and back to Applied writes a fresh "... to Applied" event.
	app = putApp(t, ts, app, map[string]any{"status": "Screening", "applied_at": daysAgo(40)})
	putApp(t, ts, app, map[string]any{"status": "Applied", "applied_at": daysAgo(40)})

	if n := markNoReply(t, ts, 30); n != 0 {
		t.Fatalf("MarkNoReply returned %d, want 0 (clock restarted by re-apply)", n)
	}
	if got := getApp(t, ts, app.ID).Status; got != models.StatusApplied {
		t.Errorf("status = %q, want Applied", got)
	}
}

func TestMarkNoReply_Disabled(t *testing.T) {
	ts := newTestServer(t)

	app := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(400)})
	if n := markNoReply(t, ts, 0); n != 0 {
		t.Fatalf("MarkNoReply with days=0 returned %d, want 0", n)
	}
	if got := getApp(t, ts, app.ID).Status; got != models.StatusApplied {
		t.Errorf("status = %q, want Applied (feature disabled)", got)
	}
}

func TestStats_NoReplyCountsAsSentNotResponded(t *testing.T) {
	ts := newTestServer(t)

	createApp(t, ts, map[string]any{"status": "NoReply", "applied_at": daysAgo(10)})
	createApp(t, ts, map[string]any{"status": "Screening", "applied_at": daysAgo(10)})

	stats := getStats(t, ts, "?period=30")

	if got := stats.ByStatus["NoReply"]; got != 1 {
		t.Errorf("by_status[NoReply] = %d, want 1", got)
	}
	if got := stats.Period.Sent.Value; got != 2 {
		t.Errorf("period.sent.value = %v, want 2", got)
	}
	if got := stats.Period.Funnel.Sent; got != 2 {
		t.Errorf("funnel.sent = %d, want 2", got)
	}
	if got := stats.Period.Funnel.Responded; got != 1 {
		t.Errorf("funnel.responded = %d, want 1", got)
	}
	// NoReply is in the denominator but never in the numerator.
	assertFloat(t, "period.response_rate", stats.Period.ResponseRate.Value, 50, 0.01)
	assertFloat(t, "response_rate", stats.ResponseRate, 50, 0.01)

	// Dedicated KPI: one silent application this period, none the period before.
	if got := stats.Period.NoReply.Value; got != 1 {
		t.Errorf("period.no_reply.value = %v, want 1", got)
	}
	assertPrev(t, "period.no_reply", stats.Period.NoReply.Prev, 0)
	if got := sumFloats(stats.Period.NoReply.Series); got != 1 {
		t.Errorf("sum(period.no_reply.series) = %v, want 1", got)
	}
}

// The funnel footer splits what got no answer into no_reply and pending. A
// NoReply application that landed an interview is a reply, never also in
// no_reply, so responded + no_reply + pending is exactly sent.
func TestStats_FunnelNoReplyAndPendingAddUpToSent(t *testing.T) {
	ts := newTestServer(t)
	interviewed := createApp(t, ts, map[string]any{"status": "NoReply", "applied_at": daysAgo(10)})
	postInterview(t, ts, interviewed.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(3)})
	createApp(t, ts, map[string]any{"status": "NoReply", "applied_at": daysAgo(10)})
	createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(10)})
	createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(10)})
	createApp(t, ts, map[string]any{"status": "Screening", "applied_at": daysAgo(10)})

	p := getStats(t, ts, "?period=30").Period
	want := models.FunnelStats{Sent: 5, Responded: 2, Interviewing: 1, NoReply: 1, Pending: 2}
	if p.Funnel != want {
		t.Errorf("funnel = %+v, want %+v", p.Funnel, want)
	}
	if f := p.Funnel; f.Responded+f.NoReply+f.Pending != f.Sent {
		t.Errorf("responded %d + no_reply %d + pending %d != sent %d", f.Responded, f.NoReply, f.Pending, f.Sent)
	}
	// The KPI tile still counts both NoReply applications: it links to status=NoReply.
	if got := p.NoReply.Value; got != 2 {
		t.Errorf("period.no_reply.value = %v, want 2", got)
	}
}

func TestImport_KeepsScreeningAndRejectedInterviews(t *testing.T) {
	ts := newTestServer(t)
	payload := map[string]any{
		"applications": []map[string]any{{
			"company_name": "RWS", "job_title": "DevOps", "contract_type": "CDI", "work_mode": "Hybrid", "status": "Interviewing",
			"interviews": []map[string]any{
				{"round": 1, "type": "Screening", "outcome": "Rejected"},
				{"round": 2, "type": "Technical", "outcome": "Pending"},
			},
		}},
	}
	w := ts.do(t, "POST", "/api/import", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	list := decode[listResponse](t, ts.do(t, "GET", "/api/applications", nil))
	if len(list.Data) != 1 {
		t.Fatalf("expected 1 application, got %d", len(list.Data))
	}
	ivs := decode[[]models.Interview](t, ts.do(t, "GET", "/api/applications/"+list.Data[0].ID+"/interviews", nil))
	if len(ivs) != 2 {
		t.Fatalf("expected both interviews to survive the import, got %d", len(ivs))
	}
}

func TestStats_ActiveProcesses(t *testing.T) {
	ts := newTestServer(t)
	rws := createApp(t, ts, map[string]any{"company_name": "RWS", "status": "Interviewing", "applied_at": daysAgo(18)})
	createApp(t, ts, map[string]any{"company_name": "Silent", "status": "Applied"})
	createApp(t, ts, map[string]any{"company_name": "Screen", "status": "Screening"})

	post := func(body map[string]any) {
		if w := ts.do(t, "POST", "/api/applications/"+rws.ID+"/interviews", body); w.Code != http.StatusCreated {
			t.Fatalf("create interview: %d %s", w.Code, w.Body.String())
		}
	}
	post(map[string]any{"round": 1, "type": "Screening", "scheduled_at": daysAgo(11), "outcome": "Passed"})
	post(map[string]any{"round": 2, "type": "Technical", "scheduled_at": daysAgo(1), "outcome": "Pending"})
	post(map[string]any{"round": 3, "type": "Final", "scheduled_at": daysAhead(3)})
	post(map[string]any{"round": 4, "type": "Final", "scheduled_at": daysAhead(1), "outcome": "Cancelled"})

	stats := getStats(t, ts, "")
	if len(stats.ActiveProcesses) != 2 {
		t.Fatalf("expected 2 active processes, got %d", len(stats.ActiveProcesses))
	}
	p := stats.ActiveProcesses[0]
	if p.CompanyName != "RWS" {
		t.Fatalf("Interviewing application should come first, got %s", p.CompanyName)
	}
	if p.Rounds != 4 {
		t.Errorf("rounds = %d, want 4", p.Rounds)
	}
	if p.LastInterview == nil || p.LastInterview.Round != 2 || p.LastInterview.Outcome != "Pending" {
		t.Errorf("last interview = %+v, want round 2 / Pending", p.LastInterview)
	}
	if p.NextInterview == nil || p.NextInterview.Round != 3 {
		t.Errorf("next interview = %+v, want round 3 (cancelled round 4 skipped)", p.NextInterview)
	}
	if p.SilentDays != 1 {
		// Silence runs from the last interview held (round 2, a day ago), not
		// from the interviews being entered just now.
		t.Errorf("silent_days = %d, want 1", p.SilentDays)
	}
	if stats.ActiveProcesses[1].CompanyName != "Screen" || stats.ActiveProcesses[1].LastInterview != nil {
		t.Errorf("second process = %+v, want Screen with no interview", stats.ActiveProcesses[1])
	}
}

func TestImport_LegacyWithdrawnStatusIsKept(t *testing.T) {
	ts := newTestServer(t)
	payload := map[string]any{
		"applications": []map[string]any{
			{"company_name": "OldCo", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Hybrid", "status": "Withdrawn"},
			{"company_name": "NewCo", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Hybrid", "status": "Applied"},
		},
	}
	w := ts.do(t, "POST", "/api/import", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	res := decode[map[string]int](t, w)
	if res["imported"] != 2 || res["skipped"] != 0 {
		t.Fatalf("import result = %v, want 2 imported / 0 skipped", res)
	}
	list := decode[listResponse](t, ts.do(t, "GET", "/api/applications?search=OldCo", nil))
	if len(list.Data) != 1 || list.Data[0].Status != models.StatusApplied {
		t.Fatalf("legacy Withdrawn application should be imported as Applied, got %+v", list.Data)
	}
}

func TestGetActivityByDay(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "DayCo", "status": "Applied"})
	putApp(t, ts, app, map[string]any{"status": "Screening"})
	today := time.Now().UTC().Format("2006-01-02")

	w := ts.do(t, "GET", "/api/activity?date="+today, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	items := decode[[]models.ActivityItem](t, w)
	if len(items) != 2 {
		t.Fatalf("expected the created + status_change events, got %d: %+v", len(items), items)
	}
	types := map[string]bool{}
	for _, it := range items {
		types[it.EventType] = true
		if it.CompanyName != "DayCo" || it.ApplicationID != app.ID {
			t.Errorf("event not joined with its application: %+v", it)
		}
	}
	if !types["created"] || !types["status_change"] {
		t.Errorf("expected created + status_change, got %v", types)
	}

	if w := ts.do(t, "GET", "/api/activity?date=2000-01-01", nil); w.Code != http.StatusOK || w.Body.String() != "[]\n" {
		t.Errorf("empty day should return an empty list, got %d %q", w.Code, w.Body.String())
	}
	if w := ts.do(t, "GET", "/api/activity?date=nope", nil); w.Code != http.StatusBadRequest {
		t.Errorf("bad date should be 400, got %d", w.Code)
	}
}

func TestListApplications_HasReply(t *testing.T) {
	ts := newTestServer(t)
	for _, st := range []string{"Wishlist", "Applied", "NoReply", "Screening", "Interviewing", "Offer", "Accepted", "Rejected"} {
		createApp(t, ts, map[string]any{"company_name": st + " Co", "status": st})
	}
	list := decode[listResponse](t, ts.do(t, "GET", "/api/applications?has_reply=1&per_page=50", nil))
	if list.Total != 5 || len(list.Data) != 5 {
		t.Fatalf("has_reply should match the 5 responded statuses, got total=%d len=%d", list.Total, len(list.Data))
	}
	for _, a := range list.Data {
		switch a.Status {
		case models.StatusScreening, models.StatusInterviewing, models.StatusOffer, models.StatusAccepted, models.StatusRejected:
		default:
			t.Errorf("unexpected status %s in has_reply list", a.Status)
		}
	}
}

func TestGetActivityByDay_IncludesInterviewsHeld(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "RWS", "status": "Interviewing"})
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	day := yesterday.Format("2006-01-02")
	post := func(body map[string]any) {
		if w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", body); w.Code != http.StatusCreated {
			t.Fatalf("create interview: %d %s", w.Code, w.Body.String())
		}
	}
	post(map[string]any{"round": 2, "type": "Technical", "scheduled_at": yesterday.Format(time.RFC3339), "outcome": "Pending"})
	post(map[string]any{"round": 3, "type": "Final", "scheduled_at": yesterday.Format(time.RFC3339), "outcome": "Cancelled"})

	items := decode[[]models.ActivityItem](t, ts.do(t, "GET", "/api/activity?date="+day, nil))
	held := 0
	for _, it := range items {
		if it.EventType == "interview_held" {
			held++
			if it.Description != "Technical · round 2" || it.CompanyName != "RWS" {
				t.Errorf("unexpected held interview item: %+v", it)
			}
		}
	}
	if held != 1 {
		t.Errorf("expected 1 interview held yesterday (cancelled one excluded), got %d: %+v", held, items)
	}

	// And the heatmap counts that day too.
	stats := getStats(t, ts, "")
	found := false
	for _, d := range stats.ActivityHeatmap {
		if d.Date == day && d.Count >= 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("heatmap should count the interview held on %s: %+v", day, stats.ActivityHeatmap)
	}
}

// --- Regression tests for the audit fixes ---

// rawDB opens a second connection to the test database. Foreign keys are off
// on it (SQLite's default), so it can plant what the API refuses to write:
// orphans left by older versions, out-of-range legacy values, or a trigger
// that makes one statement fail halfway through a request.
func rawDB(t *testing.T, ts *testServer) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ts.dbPath)
	if err != nil {
		t.Fatalf("open raw connection: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func rawExec(t *testing.T, ts *testServer, query string, args ...any) {
	t.Helper()
	if _, err := rawDB(t, ts).Exec(query, args...); err != nil {
		t.Fatalf("raw exec %q: %v", query, err)
	}
}

func rawCount(t *testing.T, ts *testServer, query string, args ...any) int {
	t.Helper()
	var n int
	if err := rawDB(t, ts).QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("raw count %q: %v", query, err)
	}
	return n
}

func postInterview(t *testing.T, ts *testServer, appID string, body map[string]any) models.Interview {
	t.Helper()
	w := ts.do(t, "POST", "/api/applications/"+appID+"/interviews", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create interview: %d %s", w.Code, w.Body.String())
	}
	return decode[models.Interview](t, w)
}

func listTotal(t *testing.T, ts *testServer, query string) int {
	t.Helper()
	w := ts.do(t, "GET", "/api/applications?"+query, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/applications?%s: %d %s", query, w.Code, w.Body.String())
	}
	return decode[listResponse](t, w).Total
}

func importApps(t *testing.T, ts *testServer, apps ...map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return ts.do(t, "POST", "/api/import", map[string]any{"applications": apps})
}

func TestUpdateApplication_PartialBodyIsRejected(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Applied", "notes": "keep me", "salary": 42000})

	w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{"company_name": "X", "job_title": "Y"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("partial PUT: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	got := getApp(t, ts, app.ID)
	if got.Status != models.StatusApplied || got.ContractType != "CDI" || got.Notes == nil || *got.Notes != "keep me" {
		t.Errorf("row changed by a rejected PUT: %+v", got)
	}
	for _, e := range got.TimelineEvents {
		if e.EventType == "status_change" {
			t.Errorf("rejected PUT wrote a timeline event: %+v", e)
		}
	}

	// A PUT without a currency gets the same default as a create.
	updated := putApp(t, ts, app, nil)
	if updated.SalaryCurrency != "EUR" {
		t.Errorf("salary_currency = %q, want EUR", updated.SalaryCurrency)
	}
}

func TestUpdateApplication_ReturnsStoredRow(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Screening"})
	if w := ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{"until": "2099-01-01"}); w.Code != http.StatusOK {
		t.Fatalf("snooze: %d", w.Code)
	}

	got := putApp(t, ts, app, map[string]any{"notes": "edited"})
	if !got.CreatedAt.Equal(app.CreatedAt) {
		t.Errorf("created_at = %v, want the stored %v", got.CreatedAt, app.CreatedAt)
	}
	if got.FollowUpSnoozedUntil == nil {
		t.Error("follow_up_snoozed_until missing from the update response")
	}
	if got.Notes == nil || *got.Notes != "edited" {
		t.Errorf("notes = %v, want edited", got.Notes)
	}
}

func TestApplication_RangeValidation(t *testing.T) {
	ts := newTestServer(t)
	for name, extra := range map[string]map[string]any{
		"confidence too high":   {"confidence": 5},
		"confidence too low":    {"confidence": 0},
		"negative salary":       {"salary": -5},
		"zero contract months":  {"contract_duration": 0},
		"negative contract len": {"contract_duration": -3},
	} {
		t.Run(name, func(t *testing.T) {
			payload := map[string]any{"company_name": "Co", "job_title": "Dev"}
			for k, v := range extra {
				payload[k] = v
			}
			if w := ts.do(t, "POST", "/api/applications", payload); w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestUpdateApplication_LegacyOutOfRangeValueStaysEditable(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Applied", "confidence": 3})
	// Written by an older version that did not check the range.
	rawExec(t, ts, `UPDATE applications SET confidence = 99 WHERE id = ?`, app.ID)

	// The detail page sends the whole row back: the stored 99 must not lock it.
	if w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote",
		"status": "Screening", "confidence": 99,
	}); w.Code != http.StatusOK {
		t.Fatalf("unchanged legacy value: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// A new invalid value is still refused.
	if w := ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "TestCo", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote",
		"status": "Screening", "confidence": 7,
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("new invalid value: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInterview_DefaultsAndStoredResponses(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, nil)

	iv := postInterview(t, ts, app.ID, map[string]any{})
	if iv.Round != 1 || iv.Type != models.InterviewPhone {
		t.Errorf("empty interview = round %d / %q, want 1 / Phone", iv.Round, iv.Type)
	}
	found := false
	for _, e := range getApp(t, ts, app.ID).TimelineEvents {
		if e.Description == "Interview round 1 (Phone) added" {
			found = true
		}
	}
	if !found {
		t.Error("timeline should record the defaulted round and type")
	}

	w := ts.do(t, "PUT", "/api/interviews/"+iv.ID, map[string]any{"round": 2, "type": "Video", "outcome": "Passed"})
	if w.Code != http.StatusOK {
		t.Fatalf("update interview: %d %s", w.Code, w.Body.String())
	}
	got := decode[models.Interview](t, w)
	if got.ApplicationID != app.ID || got.CreatedAt.IsZero() || got.Round != 2 {
		t.Errorf("update response is not the stored row: %+v", got)
	}

	w = ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{"name": "Alice"})
	c := decode[models.Contact](t, w)
	w = ts.do(t, "PUT", "/api/contacts/"+c.ID, map[string]any{"name": "Alice Martin"})
	if w.Code != http.StatusOK {
		t.Fatalf("update contact: %d %s", w.Code, w.Body.String())
	}
	gotC := decode[models.Contact](t, w)
	if gotC.ApplicationID != app.ID || gotC.CreatedAt.IsZero() || gotC.Name != "Alice Martin" {
		t.Errorf("contact update response is not the stored row: %+v", gotC)
	}
}

func TestChildren_UnknownApplication(t *testing.T) {
	ts := newTestServer(t)

	if w := ts.do(t, "POST", "/api/applications/does-not-exist/interviews", map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAhead(2)}); w.Code != http.StatusNotFound {
		t.Errorf("interview on unknown application: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if w := ts.do(t, "POST", "/api/applications/does-not-exist/contacts", map[string]any{"name": "Bob"}); w.Code != http.StatusNotFound {
		t.Errorf("contact on unknown application: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if w := ts.do(t, "GET", "/api/applications/does-not-exist/interviews", nil); w.Code != http.StatusNotFound {
		t.Errorf("list interviews of unknown application: expected 404, got %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/applications/does-not-exist/contacts", nil); w.Code != http.StatusNotFound {
		t.Errorf("list contacts of unknown application: expected 404, got %d", w.Code)
	}
	if got := getStats(t, ts, "").UpcomingInterviews; got != 0 {
		t.Errorf("upcoming_interviews = %d, want 0", got)
	}

	app := createApp(t, ts, nil)
	for _, path := range []string{"/interviews", "/contacts"} {
		w := ts.do(t, "GET", "/api/applications/"+app.ID+path, nil)
		if w.Code != http.StatusOK || w.Body.String() != "[]\n" {
			t.Errorf("GET %s of an application without any: got %d %q, want 200 []", path, w.Code, w.Body.String())
		}
	}
}

func TestStoredTimesAreUTC(t *testing.T) {
	ts := newTestServer(t)
	want := time.Date(2025, time.February, 28, 22, 30, 0, 0, time.UTC)

	app := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": "2025-03-01T00:30:00+02:00"})
	if got := getApp(t, ts, app.ID).AppliedAt; got == nil || !got.Equal(want) {
		t.Errorf("created applied_at = %v, want %v", got, want)
	}
	if app.AppliedAt == nil || !app.AppliedAt.Equal(want) {
		t.Errorf("create response applied_at = %v, want %v", app.AppliedAt, want)
	}

	w := importApps(t, ts, map[string]any{"id": "offset-import", "company_name": "Off", "job_title": "Set", "applied_at": "2025-03-01T00:30:00+02:00"})
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	if got := getApp(t, ts, "offset-import").AppliedAt; got == nil || !got.Equal(want) {
		t.Errorf("imported applied_at = %v, want %v", got, want)
	}
}

func TestListApplications_PageOverflow(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, nil)
	w := ts.do(t, "GET", "/api/applications?page=9223372036854775807&per_page=500", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := decode[listResponse](t, w)
	if resp.Page < 1 || len(resp.Data) != 0 {
		t.Errorf("huge page: page=%d with %d rows, want a page >= 1 with no rows", resp.Page, len(resp.Data))
	}
}

func TestListApplications_SortOrder(t *testing.T) {
	ts := newTestServer(t)
	for _, c := range []struct {
		name       string
		status     string
		confidence any
	}{
		{"Zenika", "Rejected", 2},
		{"leboncoin", "Wishlist", nil},
		{"École 42", "NoReply", 4},
		{"iAdvize", "Applied", 1},
		{"Alpha", "Offer", nil},
	} {
		createApp(t, ts, map[string]any{"company_name": c.name, "status": c.status, "confidence": c.confidence})
	}
	names := func(query string) []string {
		var out []string
		for _, a := range decode[listResponse](t, ts.do(t, "GET", "/api/applications?"+query, nil)).Data {
			out = append(out, a.CompanyName)
		}
		return out
	}
	check := func(query string, want ...string) {
		t.Helper()
		if got := names(query); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: got %v, want %v", query, got, want)
		}
	}
	// Case and accents do not push a name to the end of the alphabet.
	check("sort=company_name&dir=asc", "Alpha", "École 42", "iAdvize", "leboncoin", "Zenika")
	// Pipeline order, not the English names' alphabetical order.
	check("sort=status&dir=asc", "leboncoin", "iAdvize", "Alpha", "Zenika", "École 42")
	// Applications without a value come last in both directions.
	for dir, want := range map[string]string{"asc": "iAdvize|Zenika|École 42", "desc": "École 42|Zenika|iAdvize"} {
		got := names("sort=confidence&dir=" + dir)
		if len(got) != 5 || strings.Join(got[:3], "|") != want {
			t.Errorf("sort=confidence&dir=%s: got %v, want %s then the two without a value", dir, got, want)
		}
	}
}

func TestSearchAndDuplicates_IgnoreCaseAccentsAndSpaces(t *testing.T) {
	ts := newTestServer(t)
	createApp(t, ts, map[string]any{"company_name": "Société Générale ", "job_title": "Dev"})
	createApp(t, ts, map[string]any{"company_name": "École 42", "job_title": "Pédagogue", "source": " LinkedIn "})
	createApp(t, ts, map[string]any{"company_name": "Other", "job_title": "Dev", "source": "LinkedIn"})

	for _, q := range []string{"societe generale", "SOCIÉTÉ  GÉNÉRALE", "Société Générale", " société générale "} {
		var results []map[string]any
		json.NewDecoder(ts.do(t, "GET", "/api/applications/duplicates?company_name="+url.QueryEscape(q), nil).Body).Decode(&results)
		if len(results) != 1 {
			t.Errorf("duplicates for %q: got %d, want 1", q, len(results))
		}
	}
	for _, q := range []string{"école", "ECOLE", "pedagogue"} {
		if got := listTotal(t, ts, "search="+url.QueryEscape(q)); got != 1 {
			t.Errorf("search %q: got %d, want 1", q, got)
		}
	}

	var sources []string
	json.NewDecoder(ts.do(t, "GET", "/api/sources", nil).Body).Decode(&sources)
	if len(sources) != 1 || sources[0] != "LinkedIn" {
		t.Errorf("sources = %q, want the one trimmed LinkedIn", sources)
	}
}

func TestBulkUpdateStatus_SetsAppliedAtAndTimeline(t *testing.T) {
	ts := newTestServer(t)
	wish := createApp(t, ts, map[string]any{"company_name": "Wish", "status": "Wishlist"})
	screening := createApp(t, ts, map[string]any{"company_name": "Back", "status": "Screening", "applied_at": "2025-01-15T00:00:00Z"})

	w := ts.do(t, "PUT", "/api/applications/bulk/status", map[string]any{
		"ids": []string{wish.ID, screening.ID, "unknown-id"}, "status": "Applied",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk: %d %s", w.Code, w.Body.String())
	}
	if got := decode[map[string]int](t, w)["updated"]; got != 2 {
		t.Errorf("updated = %d, want 2 (the unknown id is skipped)", got)
	}
	got := getApp(t, ts, wish.ID)
	if got.AppliedAt == nil {
		t.Error("bulk move to Applied should stamp applied_at, as the single update does")
	}
	found := false
	for _, e := range got.TimelineEvents {
		if e.Description == "Status changed from Wishlist to Applied" {
			found = true
		}
	}
	if !found {
		t.Error("bulk move should write the status_change event")
	}
	if a := getApp(t, ts, screening.ID).AppliedAt; a == nil || a.Format("2006-01-02") != "2025-01-15" {
		t.Errorf("existing applied_at must be kept, got %v", a)
	}
	if p := getStats(t, ts, "?period=30").Period; p.Sent.Value != 1 {
		t.Errorf("period.sent = %v, want 1 (the bulk-applied Wish)", p.Sent.Value)
	}
}

// The list sends the user's local calendar day, so the sent date does not
// depend on the server clock around midnight.
func TestBulkUpdateStatus_UsesClientAppliedDate(t *testing.T) {
	ts := newTestServer(t)
	wish := createApp(t, ts, map[string]any{"company_name": "Wish", "status": "Wishlist"})

	w := ts.do(t, "PUT", "/api/applications/bulk/status", map[string]any{
		"ids": []string{wish.ID}, "status": "Applied", "applied_at": "2026-03-14T00:00:00Z",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk: %d %s", w.Code, w.Body.String())
	}
	if a := getApp(t, ts, wish.ID).AppliedAt; a == nil || !a.Equal(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("applied_at = %v, want the client's calendar day 2026-03-14", a)
	}
}

func TestBulkActions_AreAllOrNothing(t *testing.T) {
	ts := newTestServer(t)
	a := createApp(t, ts, map[string]any{"company_name": "A", "status": "Applied"})
	b := createApp(t, ts, map[string]any{"company_name": "B", "status": "Applied"})
	// The second application's timeline insert fails: the first status
	// change must be rolled back with it.
	rawExec(t, ts, fmt.Sprintf(`CREATE TRIGGER boom_status BEFORE INSERT ON timeline_events
		WHEN NEW.application_id = '%s' BEGIN SELECT RAISE(ABORT, 'boom'); END`, b.ID))

	w := ts.do(t, "PUT", "/api/applications/bulk/status", map[string]any{"ids": []string{a.ID, b.ID}, "status": "Rejected"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bulk status with a failing row: expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if got := getApp(t, ts, a.ID).Status; got != models.StatusApplied {
		t.Errorf("first application status = %s, want Applied (rolled back)", got)
	}

	// The same goes for the single update: no status change without its event.
	w = ts.do(t, "PUT", "/api/applications/"+b.ID, map[string]any{
		"company_name": "B", "job_title": "Dev", "contract_type": "CDI", "work_mode": "Remote", "status": "Screening",
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("update with a failing event: expected 500, got %d", w.Code)
	}
	if got := getApp(t, ts, b.ID).Status; got != models.StatusApplied {
		t.Errorf("status = %s, want Applied (rolled back with its event)", got)
	}

	rawExec(t, ts, fmt.Sprintf(`CREATE TRIGGER boom_delete BEFORE DELETE ON applications
		WHEN OLD.id = '%s' BEGIN SELECT RAISE(ABORT, 'boom'); END`, b.ID))
	w = ts.do(t, "DELETE", "/api/applications/bulk", map[string]any{"ids": []string{a.ID, b.ID}})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bulk delete with a failing row: expected 500, got %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/applications/"+a.ID, nil); w.Code != http.StatusOK {
		t.Errorf("first application should survive a failed bulk delete, got %d", w.Code)
	}
}

func TestMarkNoReply_SkipsApplicationsWithAnInterview(t *testing.T) {
	ts := newTestServer(t)
	// Any real interview, even undated and without outcome, is an answer.
	withInterview := createApp(t, ts, map[string]any{"company_name": "Call", "status": "Applied", "applied_at": daysAgo(40)})
	postInterview(t, ts, withInterview.ID, map[string]any{"round": 1, "type": "Phone"})
	// A cancelled interview is not.
	cancelledOnly := createApp(t, ts, map[string]any{"company_name": "Cancelled", "status": "Applied", "applied_at": daysAgo(40)})
	postInterview(t, ts, cancelledOnly.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAhead(2), "outcome": "Cancelled"})

	if n := markNoReply(t, ts, 30); n != 1 {
		t.Fatalf("MarkNoReply returned %d, want 1", n)
	}
	if got := getApp(t, ts, withInterview.ID).Status; got != models.StatusApplied {
		t.Errorf("application with an interview = %s, want Applied", got)
	}
	got := getApp(t, ts, cancelledOnly.ID)
	if got.Status != models.StatusNoReply {
		t.Errorf("application with only a cancelled interview = %s, want NoReply", got.Status)
	}
	// The job's own event is told apart from a manual change by its id.
	for _, e := range got.TimelineEvents {
		if e.Description == "Status changed from Applied to NoReply" && !strings.HasPrefix(e.ID, "auto-noreply-") {
			t.Errorf("automatic event id = %q, want the auto-noreply- prefix", e.ID)
		}
	}
}

// An interview added after the job picked its candidates still wins: the
// UPDATE checks for one itself instead of trusting the earlier SELECT.
func TestMarkNoReply_InterviewAddedMidRunIsRespected(t *testing.T) {
	ts := newTestServer(t)
	a := createApp(t, ts, map[string]any{"company_name": "A", "status": "Applied", "applied_at": daysAgo(40)})
	b := createApp(t, ts, map[string]any{"company_name": "B", "status": "Applied", "applied_at": daysAgo(40)})
	// Both are stale when the job reads them. As soon as it moves the first
	// one to NoReply, the other gets an interview, as if the user added one
	// between the job's SELECT and its UPDATE. Whichever comes first, the
	// second must be left alone.
	rawExec(t, ts, `CREATE TRIGGER race_interview AFTER UPDATE OF status ON applications
		WHEN NEW.status = 'NoReply' BEGIN
			INSERT INTO interviews (id, application_id)
			SELECT 'race-' || id, id FROM applications WHERE status = 'Applied';
		END`)

	if n := markNoReply(t, ts, 30); n != 1 {
		t.Fatalf("MarkNoReply returned %d, want 1", n)
	}
	statuses := map[models.ApplicationStatus]int{}
	for _, id := range []string{a.ID, b.ID} {
		statuses[getApp(t, ts, id).Status]++
	}
	if statuses[models.StatusNoReply] != 1 || statuses[models.StatusApplied] != 1 {
		t.Errorf("statuses = %v, want one NoReply and one Applied", statuses)
	}
	// The timeline event follows the row actually updated, never the skipped one.
	if got := rawCount(t, ts, `SELECT COUNT(*) FROM timeline_events WHERE id LIKE 'auto-noreply-%'`); got != 1 {
		t.Errorf("auto-noreply events = %d, want 1", got)
	}
}

func TestActivity_HeatmapMatchesDayPanel(t *testing.T) {
	ts := newTestServer(t)
	day := time.Now().UTC().AddDate(0, 0, -10)
	d := day.Format("2006-01-02")
	at := func(hm string) string { return d + "T" + hm + ":00Z" }

	w := importApps(t, ts, map[string]any{
		"id": "act-app", "company_name": "ActCo", "job_title": "Dev", "status": "Interviewing",
		"timeline_events": []map[string]any{
			{"event_type": "created", "description": "Application created", "created_at": at("09:00")},
			{"event_type": "status_change", "description": "Status changed from Applied to Interviewing", "created_at": at("10:00")},
			// Nobody typed these two: the no-reply job and migration 006.
			{"id": "auto-noreply-1", "event_type": "status_change", "description": "Status changed from Applied to NoReply", "created_at": at("11:00")},
			{"id": "migration-006-act-app", "event_type": "status_change", "description": "Status changed from Withdrawn to Applied", "created_at": at("12:00")},
		},
		"interviews": []map[string]any{
			{"round": 1, "type": "Phone", "scheduled_at": at("14:00")},
			{"round": 2, "type": "Video", "scheduled_at": at("15:00"), "outcome": "Cancelled"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	// An orphan left by a delete made before foreign keys were enforced.
	rawExec(t, ts, `INSERT INTO timeline_events (id, application_id, event_type, description, created_at)
		VALUES ('orphan', 'gone', 'created', 'Application created', ?)`, d+" 13:00:00")

	// Today: one application created through the API (one event, not two),
	// and an interview later today that has not been held yet.
	today := time.Now().UTC().Format("2006-01-02")
	created := createApp(t, ts, map[string]any{"company_name": "Today"})
	postInterview(t, ts, created.ID, map[string]any{"round": 1, "type": "HR", "scheduled_at": today + "T23:59:59Z"})

	heat := map[string]int{}
	for _, c := range getStats(t, ts, "").ActivityHeatmap {
		heat[c.Date] = c.Count
	}
	for _, tc := range []struct {
		day  string
		want int
	}{
		{d, 3},     // created, status change, interview held
		{today, 2}, // created, interview added
	} {
		items := decode[[]models.ActivityItem](t, ts.do(t, "GET", "/api/activity?date="+tc.day, nil))
		if heat[tc.day] != tc.want || len(items) != tc.want {
			t.Errorf("%s: heatmap %d, day panel %d items, want both %d: %+v", tc.day, heat[tc.day], len(items), tc.want, items)
		}
	}
}

func TestActivity_UsesTheViewersDays(t *testing.T) {
	ts := newTestServer(t)
	base := time.Now().UTC().AddDate(0, 0, -20)
	d := base.Format("2006-01-02")
	next := base.AddDate(0, 0, 1).Format("2006-01-02")
	prev := base.AddDate(0, 0, -1).Format("2006-01-02")

	w := importApps(t, ts, map[string]any{
		"id": "tz-app", "company_name": "TzCo", "job_title": "Dev", "status": "Interviewing",
		"timeline_events": []map[string]any{
			{"event_type": "note", "description": "late evening", "created_at": d + "T23:30:00Z"},
			{"event_type": "note", "description": "small hours", "created_at": next + "T01:00:00Z"},
		},
		// Interview times are the wall clock the user typed: this one is on d
		// whatever the viewer's zone.
		"interviews": []map[string]any{{"round": 1, "type": "Phone", "scheduled_at": d + "T23:30:00Z"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}

	for _, tc := range []struct {
		tz   string
		want map[string]int
	}{
		{"", map[string]int{d: 2, next: 1}},                   // UTC
		{"America/Toronto", map[string]int{d: 3}},             // 19:30 and 21:00 (or 18:30 and 20:00) on d
		{"Europe/Paris", map[string]int{d: 1, next: 2}},       // 00:30 and 02:00 (or 01:30 and 03:00) the next day
		{"Not/AZone", map[string]int{d: 2, next: 1}},          // unknown zone falls back to UTC
		{"Local", map[string]int{d: 2, next: 1}},              // the server's zone is never used
		{"../../etc/passwd", map[string]int{d: 2, next: 1}},   // rejected by LoadLocation
		{"Pacific/Kiritimati", map[string]int{d: 1, next: 2}}, // UTC+14
	} {
		q := ""
		if tc.tz != "" {
			q = "&tz=" + tc.tz
		}
		heat := map[string]int{}
		for _, c := range getStats(t, ts, "?period=30"+q).ActivityHeatmap {
			heat[c.Date] = c.Count
		}
		for _, day := range []string{prev, d, next} {
			items := decode[[]models.ActivityItem](t, ts.do(t, "GET", "/api/activity?date="+day+q, nil))
			if heat[day] != tc.want[day] || len(items) != tc.want[day] {
				t.Errorf("tz=%q %s: heatmap %d, day panel %d, want %d", tc.tz, day, heat[day], len(items), tc.want[day])
			}
		}
	}
}

func TestStats_OrphansAreIgnored(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Applied"})
	putApp(t, ts, app, map[string]any{"status": "Screening"})
	postInterview(t, ts, app.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAhead(2)})
	// Deleted the way older versions did, without the cascade.
	rawExec(t, ts, `DELETE FROM applications WHERE id = ?`, app.ID)

	stats := getStats(t, ts, "")
	if stats.Total != 0 || stats.UpcomingInterviews != 0 || len(stats.ActivityHeatmap) != 0 {
		t.Errorf("orphans still counted: total=%d upcoming=%d heatmap=%v", stats.Total, stats.UpcomingInterviews, stats.ActivityHeatmap)
	}
	if replies := stats.Weekly[len(stats.Weekly)-1].Replies; replies != 0 {
		t.Errorf("weekly replies = %d, want 0", replies)
	}
}

func TestStats_UpcomingIgnoresClosedApplications(t *testing.T) {
	ts := newTestServer(t)
	for _, st := range []string{"Rejected", "Wishlist", "NoReply", "Accepted", "Interviewing"} {
		app := createApp(t, ts, map[string]any{"company_name": st, "status": st})
		postInterview(t, ts, app.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAhead(2)})
	}
	if got := getStats(t, ts, "").UpcomingInterviews; got != 1 {
		t.Errorf("upcoming_interviews = %d, want 1 (the Interviewing one)", got)
	}
}

func TestStats_InterviewCountsAsReply(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(10)})
	postInterview(t, ts, app.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(1)})
	createApp(t, ts, map[string]any{"status": "Applied", "applied_at": daysAgo(10)})

	p := getStats(t, ts, "?period=30").Period
	want := models.FunnelStats{Sent: 2, Responded: 1, Interviewing: 1, Pending: 1}
	if p.Funnel != want {
		t.Errorf("funnel = %+v, want %+v (an interview is an answer)", p.Funnel, want)
	}
	if got := listTotal(t, ts, "has_reply=1"); got != 1 {
		t.Errorf("has_reply list = %d, want 1", got)
	}
}

// Every KPI tile of the dashboard links to the list: the list total must be
// the number on the tile, for every period.
func TestStats_TileTotalsMatchList(t *testing.T) {
	ts := newTestServer(t)
	mk := func(status string, applied any, interviews ...map[string]any) {
		fields := map[string]any{"status": status}
		if applied != nil {
			fields["applied_at"] = applied
		}
		app := createApp(t, ts, fields)
		for _, iv := range interviews {
			postInterview(t, ts, app.ID, iv)
		}
	}
	held := map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(3)}
	cancelled := map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(3), "outcome": "Cancelled"}
	for _, days := range []int{10, 50, 200, 400} {
		mk("Applied", daysAgo(days))
		mk("Applied", daysAgo(days), held)
		mk("Applied", daysAgo(days), cancelled)
		mk("NoReply", daysAgo(days))
		mk("NoReply", daysAgo(days), held)
		mk("Screening", daysAgo(days))
		mk("Interviewing", daysAgo(days), held)
		mk("Offer", daysAgo(days))
		mk("Accepted", daysAgo(days), held)
		mk("Rejected", daysAgo(days), held)
		mk("Wishlist", daysAgo(days), held)
	}
	mk("Screening", nil)        // sent "now": no applied_at, falls back to created_at
	mk("Applied", daysAhead(3)) // dated in the future by mistake
	mk("Wishlist", nil, held)   // never sent
	// Either side of the 30-day edge, an hour away from it.
	edge := time.Now().UTC().AddDate(0, 0, -30)
	mk("Applied", edge.Add(time.Hour).Format(time.RFC3339), held)
	mk("Rejected", edge.Add(-time.Hour).Format(time.RFC3339))

	for _, period := range []string{"30", "90", "365", "all"} {
		p := getStats(t, ts, "?period="+period).Period
		suffix := "&period=" + period
		if period == "all" {
			suffix = ""
		}
		for _, tile := range []struct {
			name   string
			value  float64
			params string
		}{
			{"sent", p.Sent.Value, "sent=1"},
			{"responded", p.Responded.Value, "has_reply=1&sent=1"},
			{"interviews", p.Interviews.Value, "has_interviews=1&sent=1"},
			{"offers", p.Offers.Value, "status=Offer,Accepted"},
			{"rejected", p.Rejected.Value, "status=Rejected"},
			{"no_reply", p.NoReply.Value, "status=NoReply"},
		} {
			if tile.value == 0 {
				t.Errorf("period %s: tile %s is 0, the fixture should fill it", period, tile.name)
			}
			if got := listTotal(t, ts, tile.params+suffix); float64(got) != tile.value {
				t.Errorf("period %s: tile %s = %v but the list (%s) has %d", period, tile.name, tile.value, tile.params+suffix, got)
			}
		}
		if float64(p.Funnel.Responded) != p.Responded.Value || float64(p.Funnel.Sent) != p.Sent.Value {
			t.Errorf("period %s: funnel %+v disagrees with the tiles", period, p.Funnel)
		}
		if p.Funnel.Interviewing > p.Funnel.Responded || p.Funnel.Responded > p.Funnel.Sent {
			t.Errorf("period %s: funnel is not monotonic: %+v", period, p.Funnel)
		}
		if f := p.Funnel; f.Responded+f.NoReply+f.Pending != f.Sent || f.NoReply < 0 || f.Pending < 0 {
			t.Errorf("period %s: funnel footer does not add up to sent: %+v", period, f)
		}
	}

	if w := ts.do(t, "GET", "/api/applications?status=Offer,Bogus", nil); w.Code != http.StatusBadRequest {
		t.Errorf("unknown status in the list: expected 400, got %d", w.Code)
	}
}

func TestStats_FollowUpsIgnoreCancelledInterviews(t *testing.T) {
	ts := newTestServer(t)
	onlyCancelled := createApp(t, ts, map[string]any{"company_name": "Cancelled", "status": "Interviewing"})
	postInterview(t, ts, onlyCancelled.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(15), "outcome": "Cancelled"})
	stale := createApp(t, ts, map[string]any{"company_name": "Stale", "status": "Interviewing"})
	postInterview(t, ts, stale.ID, map[string]any{"round": 1, "type": "Phone", "scheduled_at": daysAgo(20)})
	postInterview(t, ts, stale.ID, map[string]any{"round": 2, "type": "Video", "scheduled_at": daysAgo(5), "outcome": "Cancelled"})

	fu := getStats(t, ts, "").FollowUps
	if len(fu) != 1 || fu[0].ID != stale.ID {
		t.Errorf("follow-ups = %+v, want only Stale", fu)
	}
}

func TestStats_ActiveProcesses_SilenceIsTheCompanys(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "Quiet", "status": "Screening", "applied_at": daysAgo(20)})
	// Things the user does on their side are not news from the company.
	putApp(t, ts, app, map[string]any{"notes": "called them"})
	ts.do(t, "PUT", "/api/applications/"+app.ID+"/snooze", map[string]any{"until": "2099-01-01"})
	ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{"name": "Alice"})
	// An interview without a date has not been held as far as anyone knows.
	postInterview(t, ts, app.ID, map[string]any{"round": 1, "type": "HR"})

	ap := getStats(t, ts, "").ActiveProcesses
	if len(ap) != 1 {
		t.Fatalf("expected 1 active process, got %d", len(ap))
	}
	if ap[0].SilentDays != 20 {
		t.Errorf("silent_days = %d, want 20", ap[0].SilentDays)
	}
	if ap[0].LastInterview != nil || ap[0].Rounds != 1 {
		t.Errorf("undated interview: last=%+v rounds=%d, want no last interview and 1 round", ap[0].LastInterview, ap[0].Rounds)
	}

	// Moving to Interviewing is news: the company answered.
	putApp(t, ts, app, map[string]any{"status": "Interviewing"})
	if got := getStats(t, ts, "").ActiveProcesses[0].SilentDays; got != 0 {
		t.Errorf("silent_days after a reply = %d, want 0", got)
	}
}

var generatedID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func TestImport_MalformedIDsAreReplaced(t *testing.T) {
	ts := newTestServer(t)
	evil := `x" onmouseover="alert(1)`
	w := importApps(t, ts, map[string]any{
		"id": evil, "company_name": "EvilCo", "job_title": "Dev",
		"interviews":      []map[string]any{{"id": `<img src=x>`, "round": 1, "type": "Phone"}},
		"contacts":        []map[string]any{{"id": `a"b`, "name": "Eve"}},
		"timeline_events": []map[string]any{{"id": "migration-006-0b1c", "event_type": "status_change", "description": "Status changed from Withdrawn to Applied"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	if res := decode[map[string]int](t, w); res["imported"] != 1 {
		t.Fatalf("import result = %v, want 1 imported (never skipped for its id)", res)
	}
	list := decode[listResponse](t, ts.do(t, "GET", "/api/applications", nil))
	if len(list.Data) != 1 || !generatedID.MatchString(list.Data[0].ID) {
		t.Fatalf("stored id = %+v, want a clean one", list.Data)
	}
	app := getApp(t, ts, list.Data[0].ID)
	if len(app.Interviews) != 1 || !generatedID.MatchString(app.Interviews[0].ID) ||
		len(app.Contacts) != 1 || !generatedID.MatchString(app.Contacts[0].ID) {
		t.Errorf("children should follow the application under clean ids: %+v %+v", app.Interviews, app.Contacts)
	}
	if len(app.TimelineEvents) != 1 || app.TimelineEvents[0].ID != "migration-006-0b1c" {
		t.Errorf("a well-formed event id must be kept: %+v", app.TimelineEvents)
	}
}

func TestImport_ChildIDAlreadyTakenGetsANewOne(t *testing.T) {
	ts := newTestServer(t)
	importApps(t, ts, map[string]any{"id": "app-a", "company_name": "A", "job_title": "Dev",
		"interviews": []map[string]any{{"id": "iv-1", "round": 1, "type": "Phone", "notes": "A's"}}})
	w := importApps(t, ts, map[string]any{"id": "app-b", "company_name": "B", "job_title": "Dev",
		"interviews": []map[string]any{{"id": "iv-1", "round": 1, "type": "Video", "notes": "B's"}}})
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	a, b := getApp(t, ts, "app-a"), getApp(t, ts, "app-b")
	if len(a.Interviews) != 1 || a.Interviews[0].ID != "iv-1" || *a.Interviews[0].Notes != "A's" {
		t.Errorf("A's interview must be untouched: %+v", a.Interviews)
	}
	if len(b.Interviews) != 1 || b.Interviews[0].ID == "iv-1" || *b.Interviews[0].Notes != "B's" {
		t.Errorf("B's interview must be kept under a new id: %+v", b.Interviews)
	}
}

func TestImport_RestoreAfterDeleteReplacesOrphans(t *testing.T) {
	ts := newTestServer(t)
	app := createApp(t, ts, map[string]any{"company_name": "Restored"})
	iv := postInterview(t, ts, app.ID, map[string]any{"round": 1, "type": "Phone", "notes": "backup-version"})
	var backup map[string]any
	json.NewDecoder(ts.do(t, "GET", "/api/export", nil).Body).Decode(&backup)

	ts.do(t, "PUT", "/api/interviews/"+iv.ID, map[string]any{"round": 1, "type": "Phone", "notes": "post-backup-edit"})
	ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{"name": "Added after backup"})
	// Deleted the way older versions did, leaving the children behind.
	rawExec(t, ts, `DELETE FROM applications WHERE id = ?`, app.ID)

	w := ts.do(t, "POST", "/api/import", backup)
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	got := getApp(t, ts, app.ID)
	if len(got.Interviews) != 1 || got.Interviews[0].Notes == nil || *got.Interviews[0].Notes != "backup-version" {
		t.Errorf("interviews = %+v, want only the backup's version", got.Interviews)
	}
	if len(got.Contacts) != 0 {
		t.Errorf("contacts = %+v, want none (the backup had none)", got.Contacts)
	}
}

func TestImport_IsAllOrNothing(t *testing.T) {
	ts := newTestServer(t)
	rawExec(t, ts, `CREATE TRIGGER boom_import BEFORE INSERT ON contacts
		WHEN NEW.name = 'boom' BEGIN SELECT RAISE(ABORT, 'boom'); END`)

	w := importApps(t, ts,
		map[string]any{"company_name": "First", "job_title": "Dev",
			"interviews": []map[string]any{{"round": 1, "type": "Phone"}},
			"contacts":   []map[string]any{{"name": "Fine"}}},
		map[string]any{"company_name": "Second", "job_title": "Dev", "contacts": []map[string]any{{"name": "boom"}}},
		map[string]any{"company_name": "Third", "job_title": "Dev"},
	)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("import with a failing row: expected 500, got %d: %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"applications", "interviews", "contacts", "timeline_events"} {
		if n := rawCount(t, ts, "SELECT COUNT(*) FROM "+table); n != 0 {
			t.Errorf("%s has %d rows after a failed import, want 0", table, n)
		}
	}
}

func TestImport_KeepsLegacyValues(t *testing.T) {
	ts := newTestServer(t)
	w := importApps(t, ts, map[string]any{
		"id": "legacy", "company_name": "Legacy", "job_title": "Dev",
		"confidence": 9, "rating": 7, "salary": -5, "contract_duration": 0,
		"interviews": []map[string]any{{"round": 0, "type": ""}, {"round": -2, "type": "Video", "duration_minutes": -3}},
	})
	if w.Code != http.StatusOK || decode[map[string]int](t, w)["imported"] != 1 {
		t.Fatalf("legacy application must be imported, got %d", w.Code)
	}
	got := getApp(t, ts, "legacy")
	if got.Confidence != nil || got.Rating != nil || got.Salary != nil || got.ContractDuration != nil {
		t.Errorf("out-of-range values should be imported empty: %+v", got)
	}
	if len(got.Interviews) != 2 {
		t.Fatalf("both interviews must be kept, got %d", len(got.Interviews))
	}
	for _, iv := range got.Interviews {
		if iv.Round != 1 || iv.Type == "" || iv.DurationMinutes != nil {
			t.Errorf("legacy interview not normalised: %+v", iv)
		}
	}
}

func TestImport_LargerThanTheOldTenMegabyteLimit(t *testing.T) {
	ts := newTestServer(t)
	desc := strings.Repeat("x", 1<<20)
	var apps []map[string]any
	for i := 0; i < 12; i++ {
		apps = append(apps, map[string]any{"company_name": fmt.Sprintf("Big %d", i), "job_title": "Dev", "job_description": desc})
	}
	w := importApps(t, ts, apps...)
	if w.Code != http.StatusOK {
		t.Fatalf("12 MB import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if res := decode[map[string]int](t, w); res["imported"] != 12 {
		t.Errorf("imported = %d, want 12", res["imported"])
	}
}

func TestExport_BatchedChildrenStayWithTheirApplication(t *testing.T) {
	ts := newTestServer(t)
	a := createApp(t, ts, map[string]any{"company_name": "A"})
	b := createApp(t, ts, map[string]any{"company_name": "B"})
	postInterview(t, ts, a.ID, map[string]any{"round": 2, "type": "Video"})
	postInterview(t, ts, a.ID, map[string]any{"round": 1, "type": "Phone"})
	ts.do(t, "POST", "/api/applications/"+b.ID+"/contacts", map[string]any{"name": "Bob"})

	var export struct {
		Applications []models.Application `json:"applications"`
	}
	json.NewDecoder(ts.do(t, "GET", "/api/export", nil).Body).Decode(&export)
	byName := map[string]models.Application{}
	for _, app := range export.Applications {
		byName[app.CompanyName] = app
	}
	if ivs := byName["A"].Interviews; len(ivs) != 2 || ivs[0].Round != 1 || ivs[1].Round != 2 {
		t.Errorf("A's interviews = %+v, want rounds 1 then 2", ivs)
	}
	if len(byName["A"].Contacts) != 0 || len(byName["B"].Contacts) != 1 || len(byName["B"].Interviews) != 0 {
		t.Errorf("children attached to the wrong application: %+v", export.Applications)
	}
	if len(byName["A"].TimelineEvents) != 3 || len(byName["B"].TimelineEvents) != 2 {
		t.Errorf("timeline events: A %d, B %d, want 3 and 2", len(byName["A"].TimelineEvents), len(byName["B"].TimelineEvents))
	}
}
