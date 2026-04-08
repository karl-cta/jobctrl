package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"job-ctrl/internal/db"
	"job-ctrl/internal/handlers"
	"job-ctrl/internal/models"
)

// testServer creates an isolated server with a temp DB for each test.
type testServer struct {
	handler http.Handler
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
	r.Get("/api/applications/{id}", h.GetApplication)
	r.Put("/api/applications/{id}", h.UpdateApplication)
	r.Delete("/api/applications/{id}", h.DeleteApplication)
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
	r.Get("/api/export", h.Export)
	r.Post("/api/import", h.Import)
	r.Get("/api/export/csv", h.ExportCSV)

	ts := &testServer{handler: r, dbPath: f.Name()}
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

	// 1. Create application
	app := createApp(t, ts, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer", "status": "Wishlist",
	})

	// 2. Add interview
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

	// 3. Update status to Applied
	ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer",
		"contract_type": "CDI", "work_mode": "Hybrid", "status": "Applied",
	})

	// 4. Update status to Interviewing
	ts.do(t, "PUT", "/api/applications/"+app.ID, map[string]any{
		"company_name": "LifeCycleCo", "job_title": "Engineer",
		"contract_type": "CDI", "work_mode": "Hybrid", "status": "Interviewing",
	})

	// 5. Check stats
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
	if stats.ActiveInterviews != 1 {
		t.Errorf("expected 1 active interview, got %d", stats.ActiveInterviews)
	}

	// 6. Verify full app detail includes interview and timeline events
	w = ts.do(t, "GET", "/api/applications/"+app.ID, nil)
	full := decode[models.Application](t, w)
	if len(full.Interviews) != 1 {
		t.Errorf("expected 1 interview in detail, got %d", len(full.Interviews))
	}
	// created + interview_added + status_change(Wishlist->Applied) + status_change(Applied->Interviewing)
	if len(full.TimelineEvents) != 4 {
		t.Errorf("expected 4 timeline events, got %d", len(full.TimelineEvents))
	}

	// 7. Delete application cascades
	w = ts.do(t, "DELETE", "/api/applications/"+app.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// 8. Verify empty DB
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

	// Create
	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/interviews", map[string]any{
		"round": 1, "type": "Technical",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	iv := decode[models.Interview](t, w)

	// List
	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/interviews", nil)
	ivs := decode[[]models.Interview](t, w)
	if len(ivs) != 1 {
		t.Errorf("expected 1 interview, got %d", len(ivs))
	}

	// Update
	w = ts.do(t, "PUT", "/api/interviews/"+iv.ID, map[string]any{
		"round": 1, "type": "Technical", "outcome": "Passed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update interview: expected 200, got %d", w.Code)
	}

	// Delete
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

	// Create
	w := ts.do(t, "POST", "/api/applications/"+app.ID+"/contacts", map[string]any{
		"name": "Alice Martin", "role": "HR Manager", "email": "alice@contactco.com",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	c := decode[models.Contact](t, w)

	// List
	w = ts.do(t, "GET", "/api/applications/"+app.ID+"/contacts", nil)
	contacts := decode[[]models.Contact](t, w)
	if len(contacts) != 1 {
		t.Errorf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Name != "Alice Martin" {
		t.Errorf("expected Alice Martin, got %q", contacts[0].Name)
	}

	// Update
	w = ts.do(t, "PUT", "/api/contacts/"+c.ID, map[string]any{
		"name": "Alice Dupont", "role": "Recruiter",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update contact: expected 200, got %d", w.Code)
	}

	// Delete
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

func TestStats_OfferRate(t *testing.T) {
	ts := newTestServer(t)

	for _, s := range []string{"Applied", "Screening", "Offer", "Accepted", "Rejected"} {
		createApp(t, ts, map[string]any{"status": s})
	}

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	// applied=1, responded=4 (Screening+Offer+Accepted+Rejected), total applied pool=5
	// offers (Offer+Accepted) = 2, offer rate = 2/5*100 = 40%
	if stats.OfferRate != 40.0 {
		t.Errorf("expected offer rate 40.0, got %.1f", stats.OfferRate)
	}
}

func TestStats_SalaryDistribution(t *testing.T) {
	ts := newTestServer(t)

	createApp(t, ts, map[string]any{"salary": 35000})
	createApp(t, ts, map[string]any{"salary": 55000})
	createApp(t, ts, map[string]any{"salary": 55000})

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	if len(stats.SalaryDistribution) == 0 {
		t.Fatal("expected salary distribution buckets")
	}
	// Should have 2 buckets: 30-40k (1) and 50-60k (2)
	total := 0
	for _, b := range stats.SalaryDistribution {
		total += b.Count
	}
	if total != 3 {
		t.Errorf("expected 3 total in salary distribution, got %d", total)
	}
}

func TestStats_OverTime(t *testing.T) {
	ts := newTestServer(t)

	createApp(t, ts, map[string]any{})
	createApp(t, ts, map[string]any{})

	w := ts.do(t, "GET", "/api/stats", nil)
	stats := decode[models.Stats](t, w)
	if len(stats.OverTime) == 0 {
		t.Error("expected at least one over_time data point for current week")
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

	// Verify it exists
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

	// Verify relations
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

	// First import
	w1 := ts.do(t, "POST", "/api/import", payload)
	r1 := decode[map[string]any](t, w1)
	if int(r1["imported"].(float64)) != 1 {
		t.Errorf("first import: expected 1 imported, got %v", r1["imported"])
	}

	// Second import same ID
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

	// Create an application with relations
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

	// Export
	exportW := ts.do(t, "GET", "/api/export", nil)
	var exportData map[string]any
	json.NewDecoder(exportW.Body).Decode(&exportData)

	// Import into fresh server
	ts2 := newTestServer(t)
	importW := ts2.do(t, "POST", "/api/import", exportData)
	if importW.Code != 200 {
		t.Fatalf("import failed: %d %s", importW.Code, importW.Body.String())
	}
	result := decode[map[string]any](t, importW)
	if int(result["imported"].(float64)) != 1 {
		t.Errorf("expected 1 imported, got %v", result["imported"])
	}

	// Verify data in new server
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
