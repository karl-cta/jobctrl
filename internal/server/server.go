package server

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"job-ctrl/internal/handlers"
)

// contentSecurityPolicy allows no inline script. Inline styles stay allowed
// for the style="..." attributes in the templates; fonts are served by the
// app and company favicons come from any https host.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"font-src 'self'; img-src 'self' data: https:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; " +
	"form-action 'self'; object-src 'none'"

// maxBodyBytes caps API request bodies. POST /api/import takes whole backups
// and enforces its own, larger limit.
const maxBodyBytes = 1 << 20

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/import" {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > maxBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func New(db *sql.DB, frontendFS fs.FS, version string) http.Handler {
	// Rejects cross-site POST/PUT/DELETE (Sec-Fetch-Site, or Origin compared
	// with Host), so another web page cannot write to the local instance.
	// Behind a reverse proxy the original Host header must be forwarded.
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
	}))

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(cop.Handler)
	r.Use(middleware.Compress(5, "text/html", "text/css", "text/javascript", "application/javascript", "application/json", "image/svg+xml"))

	h := handlers.New(db)

	r.Route("/api", func(r chi.Router) {
		r.Use(limitBody)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})

		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
		})

		r.Get("/applications", h.ListApplications)
		r.Post("/applications", h.CreateApplication)
		r.Get("/applications/duplicates", h.CheckDuplicates)
		r.Put("/applications/bulk/status", h.BulkUpdateStatus)
		r.Delete("/applications/bulk", h.BulkDelete)
		r.Get("/applications/{id}", h.GetApplication)
		r.Put("/applications/{id}", h.UpdateApplication)
		r.Put("/applications/{id}/snooze", h.SnoozeFollowUp)
		r.Delete("/applications/{id}", h.DeleteApplication)

		r.Get("/applications/{id}/interviews", h.ListInterviews)
		r.Post("/applications/{id}/interviews", h.CreateInterview)
		r.Get("/interviews/{id}", h.GetInterview)
		r.Put("/interviews/{id}", h.UpdateInterview)
		r.Delete("/interviews/{id}", h.DeleteInterview)

		r.Get("/applications/{id}/contacts", h.ListContacts)
		r.Post("/applications/{id}/contacts", h.CreateContact)
		r.Get("/contacts/{id}", h.GetContact)
		r.Put("/contacts/{id}", h.UpdateContact)
		r.Delete("/contacts/{id}", h.DeleteContact)

		r.Post("/extract", h.ExtractFromURL)

		r.Get("/sources", h.ListSources)
		r.Get("/stats", h.GetStats)
		r.Get("/activity", h.GetActivityByDay)
		r.Get("/export", h.Export)
		r.Post("/import", h.Import)
		r.Get("/export/csv", h.ExportCSV)
	})

	sub, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to open embedded frontend: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Serve the static file if it exists. Vite content-hashes everything
		// under assets/, so those can be cached for good; anything else (the
		// index.html that points at them) must be revalidated after upgrades.
		name := strings.TrimPrefix(r.URL.Path, "/")
		if f, err := sub.Open(name); err == nil {
			f.Close()
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// A missing chunk (an open tab asking for an asset from the previous
		// build) must fail as such, not load index.html as JavaScript.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		// SPA fallback: client-side routes all get index.html.
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return r
}
