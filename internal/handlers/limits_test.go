package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFoldText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"École 42", "ecole 42"},
		{"SOCIÉTÉ GÉNÉRALE", "societe generale"},
		{"  Société  Générale ", "societe generale"},
		{"Œuvre Ærø Straße", "oeuvre aero strasse"},
		{"Ñandú Çà", "nandu ca"},
		{"100%_off\\", "100%_off\\"}, // LIKE metacharacters are left for escapeLike
		{"", ""},
	}
	for _, tc := range tests {
		if got := foldText(tc.in); got != tc.want {
			t.Errorf("foldText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestImport_TooLargeIs413(t *testing.T) {
	old := maxImportBytes
	maxImportBytes = 1 << 10
	defer func() { maxImportBytes = old }()

	body := `{"applications":[` + strings.Repeat(" ", 2<<10) + `]}`
	w := httptest.NewRecorder()
	// No database: the handler must answer before it needs one.
	(&Handler{}).Import(w, httptest.NewRequest("POST", "/api/import", strings.NewReader(body)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized import: expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDecodeJSON_BodyOverTheLimitIs413(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/applications", strings.NewReader(`{"company_name":"`+strings.Repeat("x", 64)+`"}`))
	req.Body = http.MaxBytesReader(w, req.Body, 16) // what the /api middleware does
	(&Handler{}).CreateApplication(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExtract_WaitsForASlotNoLongerThanTheClient(t *testing.T) {
	for i := 0; i < cap(extractSlots); i++ {
		extractSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(extractSlots); i++ {
			<-extractSlots
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/api/extract", strings.NewReader(`{"url":"https://example.com/job"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	(&Handler{}).ExtractFromURL(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("all slots busy and client gone: expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
