package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"job-ctrl/internal/db"
	"job-ctrl/internal/server"
)

const indexHTML = "<!doctype html><title>JobCtrl test index</title>"

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	frontend := fstest.MapFS{
		"frontend/dist/index.html":              {Data: []byte(indexHTML)},
		"frontend/dist/favicon.svg":             {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
		"frontend/dist/assets/app-abc123.js":    {Data: []byte("console.log('app')")},
		"frontend/dist/assets/style-abc123.css": {Data: []byte("body{}")},
	}
	return server.New(database, frontend, "test")
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newAppBody(t *testing.T) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(map[string]string{"company_name": "Acme", "job_title": "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func TestCrossOriginWrites(t *testing.T) {
	h := newHandler(t)

	tests := []struct {
		name       string
		method     string
		path       string
		headers    map[string]string
		wantDenied bool
	}{
		{"cross-site POST", http.MethodPost, "/api/applications", map[string]string{"Sec-Fetch-Site": "cross-site"}, true},
		{"same-site POST from another subdomain", http.MethodPost, "/api/applications", map[string]string{"Sec-Fetch-Site": "same-site"}, true},
		{"foreign Origin without fetch metadata", http.MethodPost, "/api/applications", map[string]string{"Origin": "https://evil.example"}, true},
		{"cross-site POST to import", http.MethodPost, "/api/import", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "text/plain"}, true},
		{"same-origin POST", http.MethodPost, "/api/applications", map[string]string{"Sec-Fetch-Site": "same-origin"}, false},
		{"Origin matching Host", http.MethodPost, "/api/applications", map[string]string{"Origin": "http://example.com"}, false},
		{"POST without browser headers (curl)", http.MethodPost, "/api/applications", nil, false},
		{"cross-site GET", http.MethodGet, "/api/health", map[string]string{"Sec-Fetch-Site": "cross-site"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Reader
			if tt.method == http.MethodPost {
				body = newAppBody(t)
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := serve(h, req)
			if tt.wantDenied {
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403", rec.Code)
				}
				if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
					t.Errorf("rejection Content-Type = %q, want JSON", ct)
				}
				return
			}
			if rec.Code == http.StatusForbidden || rec.Code >= 500 {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHandler(t)
	for _, path := range []string{"/", "/api/health", "/api/nope", "/assets/app-abc123.js"} {
		rec := serve(h, httptest.NewRequest(http.MethodGet, path, nil))
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: Content-Security-Policy = %q", path, csp)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing X-Content-Type-Options", path)
		}
	}
}

func TestHealth(t *testing.T) {
	h := newHandler(t)
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("health is not JSON: %v (%s)", err, rec.Body)
	}
	if rec.Code != http.StatusOK || got["status"] != "ok" || got["version"] != "test" {
		t.Errorf("health = %d %v", rec.Code, got)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	h := newHandler(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/nope"},
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/applications/1/nope"},
		{http.MethodPost, "/api/nothing"},
	} {
		rec := serve(h, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tc.method, tc.path, rec.Code)
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["error"] != "not found" {
			t.Errorf("%s %s: body = %q, want {\"error\":\"not found\"}", tc.method, tc.path, rec.Body)
		}
	}
}

func TestStaticFilesAndSPAFallback(t *testing.T) {
	h := newHandler(t)

	rec := serve(h, httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('app')" {
		t.Fatalf("asset: %d %q", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q", cc)
	}

	// An asset from a previous build must 404, never come back as index.html
	// (and certainly not with a year-long cache).
	for _, path := range []string{"/assets/dashboard-oldhash.js", "/assets/", "/assets/nested/missing.css"} {
		rec = serve(h, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), indexHTML) {
			t.Errorf("%s: served index.html", path)
		}
		if strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
			t.Errorf("%s: 404 marked immutable", path)
		}
	}

	for _, path := range []string{"/", "/applications", "/applications/123", "/applications/123/edit"} {
		rec = serve(h, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), indexHTML) {
			t.Errorf("%s: %d %q, want index.html", path, rec.Code, rec.Body)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, cc)
		}
	}

	rec = serve(h, httptest.NewRequest(http.MethodGet, "/favicon.svg", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("favicon: %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestOddPathsDoNotPanic(t *testing.T) {
	h := newHandler(t)

	// A raw client can send a request whose path is empty.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = ""
	rec := serve(h, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), indexHTML) {
		t.Errorf("empty path: %d %q, want index.html", rec.Code, rec.Body)
	}

	for _, path := range []string{"//", "/.", "/assets", "/index.html", "/a/../../b", "/%00"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		if rec := serve(h, req); rec.Code >= 500 {
			t.Errorf("%q: status = %d", path, rec.Code)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	h := newHandler(t)
	big := strings.Repeat("x", 2<<20)

	payload, _ := json.Marshal(map[string]string{"company_name": "Acme", "job_title": "Dev", "notes": big})
	req := httptest.NewRequest(http.MethodPost, "/api/applications", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := serve(h, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized create: status = %d, want 413", rec.Code)
	}

	// Import takes whole backups: a body over the general cap still goes through.
	payload, _ = json.Marshal(map[string]any{"applications": []map[string]string{
		{"company_name": "Acme", "job_title": "Dev", "notes": big},
	}})
	req = httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec = serve(h, req)
	if rec.Code != http.StatusOK {
		t.Errorf("large import: status = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	// A normal-sized body is untouched.
	req = httptest.NewRequest(http.MethodPost, "/api/applications", newAppBody(t))
	req.Header.Set("Content-Type", "application/json")
	if rec := serve(h, req); rec.Code != http.StatusCreated {
		t.Errorf("normal create: status = %d, want 201 (%s)", rec.Code, rec.Body)
	}
}

func TestCompression(t *testing.T) {
	h := newHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	if rec := serve(h, req); rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("JavaScript not gzipped: Content-Encoding = %q", rec.Header().Get("Content-Encoding"))
	}
}
