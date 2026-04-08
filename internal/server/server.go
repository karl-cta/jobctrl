package server

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"job-ctrl/internal/handlers"
)

func New(db *sql.DB, frontendFS embed.FS, version string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	h := handlers.New(db)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})

		r.Get("/applications", h.ListApplications)
		r.Post("/applications", h.CreateApplication)
		r.Get("/applications/{id}", h.GetApplication)
		r.Put("/applications/{id}", h.UpdateApplication)
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
		r.Get("/export", h.Export)
		r.Post("/import", h.Import)
	})

	sub, _ := fs.Sub(frontendFS, "frontend/dist")
	fileServer := http.FileServer(http.FS(sub))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Serve static file if it exists, otherwise SPA fallback to index.html
		if f, err := sub.Open(r.URL.Path[1:]); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return r
}
