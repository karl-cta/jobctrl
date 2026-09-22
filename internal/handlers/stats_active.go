package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"job-ctrl/internal/models"
)

type activeInterview struct {
	round     int
	typ       string
	at        time.Time
	scheduled bool
	outcome   string
}

// loadActiveProcesses lists the applications currently in Screening or
// Interviewing. Silence is measured from the latest of: last interview date,
// last timeline event, and the application's own updated_at.
func (h *Handler) loadActiveProcesses(ctx context.Context, now time.Time) []models.ActiveProcess {
	rows, err := h.db.QueryContext(ctx, `SELECT id, company_name, job_title, status, updated_at
		FROM applications WHERE status IN (?, ?)`,
		models.StatusScreening, models.StatusInterviewing)
	if err != nil {
		log.Printf("GetStats activeProcesses: %v", err)
		return nil
	}
	defer rows.Close()

	var out []models.ActiveProcess
	lastTouch := map[string]time.Time{}
	for rows.Next() {
		var p models.ActiveProcess
		var rawUpdated any
		if err := rows.Scan(&p.ID, &p.CompanyName, &p.JobTitle, &p.Status, &rawUpdated); err != nil {
			log.Printf("GetStats activeProcesses scan: %v", err)
			continue
		}
		if t, ok := scanTime(rawUpdated); ok {
			lastTouch[p.ID] = t
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}

	ivByApp := map[string][]activeInterview{}
	if ivRows, err := h.db.QueryContext(ctx, `SELECT application_id, round, type, scheduled_at, created_at, COALESCE(outcome, '')
		FROM interviews`); err != nil {
		log.Printf("GetStats activeProcesses interviews: %v", err)
	} else {
		defer ivRows.Close()
		for ivRows.Next() {
			var appID string
			var iv activeInterview
			var rawSched, rawCreated any
			if err := ivRows.Scan(&appID, &iv.round, &iv.typ, &rawSched, &rawCreated, &iv.outcome); err != nil {
				continue
			}
			if t, ok := scanTime(rawSched); ok {
				iv.at, iv.scheduled = t, true
			} else if t, ok := scanTime(rawCreated); ok {
				iv.at = t
			} else {
				continue
			}
			ivByApp[appID] = append(ivByApp[appID], iv)
		}
	}

	if evRows, err := h.db.QueryContext(ctx, `SELECT application_id, created_at FROM timeline_events`); err != nil {
		log.Printf("GetStats activeProcesses events: %v", err)
	} else {
		defer evRows.Close()
		for evRows.Next() {
			var appID string
			var raw any
			if err := evRows.Scan(&appID, &raw); err != nil {
				continue
			}
			if t, ok := scanTime(raw); ok {
				if cur, seen := lastTouch[appID]; !seen || t.After(cur) {
					lastTouch[appID] = t
				}
			}
		}
	}

	for i := range out {
		p := &out[i]
		ivs := ivByApp[p.ID]
		sort.Slice(ivs, func(a, b int) bool { return ivs[a].at.Before(ivs[b].at) })
		p.Rounds = len(ivs)
		for _, iv := range ivs {
			if iv.outcome == string(models.OutcomeCancelled) {
				continue
			}
			if !iv.at.After(now) {
				// Latest past interview wins (slice is sorted ascending).
				p.LastInterview = &models.InterviewStep{
					Round: iv.round, Type: iv.typ, At: iv.at.Format("2006-01-02 15:04:05"), Outcome: iv.outcome,
				}
			} else if iv.scheduled && p.NextInterview == nil {
				p.NextInterview = &models.InterviewStep{
					Round: iv.round, Type: iv.typ, At: iv.at.Format("2006-01-02 15:04:05"), Outcome: iv.outcome,
				}
			}
		}
		// Silence: days since the last thing that happened on this application.
		touch, ok := lastTouch[p.ID]
		if p.LastInterview != nil {
			if t, ok2 := parseDBTime(p.LastInterview.At); ok2 && (!ok || t.After(touch)) {
				touch, ok = t, true
			}
		}
		if ok {
			p.SilentDays = int(now.Sub(touch).Hours() / 24)
			if p.SilentDays < 0 {
				p.SilentDays = 0
			}
		}
	}

	// Most advanced / most recently active first.
	sort.Slice(out, func(a, b int) bool {
		if out[a].Status != out[b].Status {
			return out[a].Status == string(models.StatusInterviewing)
		}
		return out[a].SilentDays < out[b].SilentDays
	})
	return out
}

// GetActivityByDay returns every timeline event of one day (UTC, the same
// key the heatmap is built on), newest first, joined with its application.
// GET /api/activity?date=YYYY-MM-DD
func (h *Handler) GetActivityByDay(w http.ResponseWriter, r *http.Request) {
	day := strings.TrimSpace(r.URL.Query().Get("date"))
	if _, err := time.Parse("2006-01-02", day); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT te.created_at, te.event_type, te.description,
		a.id, a.company_name, a.job_title, a.status
		FROM timeline_events te
		JOIN applications a ON a.id = te.application_id
		WHERE date(replace(replace(te.created_at, 'T', ' '), 'Z', '')) = ?
		ORDER BY replace(replace(te.created_at, 'T', ' '), 'Z', '') DESC`, day)
	if err != nil {
		log.Printf("GetActivityByDay: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load activity")
		return
	}
	defer rows.Close()
	items := []models.ActivityItem{}
	for rows.Next() {
		var it models.ActivityItem
		if err := rows.Scan(&it.Time, &it.EventType, &it.Description, &it.ApplicationID, &it.CompanyName, &it.JobTitle, &it.Status); err != nil {
			log.Printf("GetActivityByDay scan: %v", err)
			continue
		}
		items = append(items, it)
	}

	// Interviews held that day count as activity too: an interview is the
	// most significant thing that can happen to an application, and holding
	// it leaves no timeline row of its own.
	ivRows, err := h.db.QueryContext(r.Context(), `SELECT i.scheduled_at, i.type, i.round,
		a.id, a.company_name, a.job_title, a.status
		FROM interviews i
		JOIN applications a ON a.id = i.application_id
		WHERE i.scheduled_at IS NOT NULL
		  AND COALESCE(i.outcome, '') != 'Cancelled'
		  AND date(replace(replace(i.scheduled_at, 'T', ' '), 'Z', '')) = ?`, day)
	if err != nil {
		log.Printf("GetActivityByDay interviews: %v", err)
	} else {
		defer ivRows.Close()
		for ivRows.Next() {
			var it models.ActivityItem
			var typ string
			var round int
			if err := ivRows.Scan(&it.Time, &typ, &round, &it.ApplicationID, &it.CompanyName, &it.JobTitle, &it.Status); err != nil {
				log.Printf("GetActivityByDay interview scan: %v", err)
				continue
			}
			it.EventType = "interview_held"
			it.Description = fmt.Sprintf("%s · round %d", typ, round)
			items = append(items, it)
		}
	}

	sort.SliceStable(items, func(a, b int) bool {
		ta, _ := parseDBTime(items[a].Time)
		tb, _ := parseDBTime(items[b].Time)
		return ta.After(tb)
	})
	writeJSON(w, http.StatusOK, items)
}
