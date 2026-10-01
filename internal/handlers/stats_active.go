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
// Interviewing. Silence counts the days since the company last gave news:
// its last interview that took place, or the last move to a status that means
// it answered (replies), else the day the application was sent. What the user
// does on their side (editing notes, snoozing a follow-up, adding a contact)
// is not news and does not reset it.
func (h *Handler) loadActiveProcesses(ctx context.Context, now time.Time, replies []replyEvent) []models.ActiveProcess {
	rows, err := h.db.QueryContext(ctx, `SELECT id, company_name, job_title, status, COALESCE(applied_at, created_at)
		FROM applications WHERE status IN (?, ?)`,
		models.StatusScreening, models.StatusInterviewing)
	if err != nil {
		log.Printf("GetStats activeProcesses: %v", err)
		return nil
	}
	defer rows.Close()

	var out []models.ActiveProcess
	active := map[string]bool{}
	lastNews := map[string]time.Time{}
	for rows.Next() {
		var p models.ActiveProcess
		var rawSent any
		if err := rows.Scan(&p.ID, &p.CompanyName, &p.JobTitle, &p.Status, &rawSent); err != nil {
			log.Printf("GetStats activeProcesses scan: %v", err)
			continue
		}
		if t, ok := scanTime(rawSent); ok {
			lastNews[p.ID] = t
		}
		active[p.ID] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	for _, e := range replies {
		if !active[e.appID] {
			continue
		}
		if cur, seen := lastNews[e.appID]; !seen || e.at.After(cur) {
			lastNews[e.appID] = e.at
		}
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

	for i := range out {
		p := &out[i]
		ivs := ivByApp[p.ID]
		sort.Slice(ivs, func(a, b int) bool { return ivs[a].at.Before(ivs[b].at) })
		p.Rounds = len(ivs)
		for _, iv := range ivs {
			if iv.outcome == string(models.OutcomeCancelled) {
				continue
			}
			// Without a date, nothing says the interview took place: it counts
			// in Rounds but is neither the last nor the next one.
			if !iv.scheduled {
				continue
			}
			if !iv.at.After(now) {
				// Latest past interview wins (slice is sorted ascending).
				p.LastInterview = &models.InterviewStep{
					Round: iv.round, Type: iv.typ, At: iv.at.Format("2006-01-02 15:04:05"), Outcome: iv.outcome,
				}
			} else if p.NextInterview == nil {
				p.NextInterview = &models.InterviewStep{
					Round: iv.round, Type: iv.typ, At: iv.at.Format("2006-01-02 15:04:05"), Outcome: iv.outcome,
				}
			}
		}
		touch, ok := lastNews[p.ID]
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

// heatmapDays is how far back the activity heatmap reaches. Its grid is 26
// weeks ending with the current one, so its first cell is at most 181 days
// ago (when today is a Sunday); one more day of margin.
const heatmapDays = 182

// userEventSQL leaves out of the activity views the timeline rows nobody
// typed: those migration 006 wrote, and those of the no-reply job.
const userEventSQL = `te.id NOT LIKE 'migration-%' AND te.id NOT LIKE '` + autoEventPrefix + `%'`

// requestLocation returns the viewer's time zone from ?tz= (an IANA name such
// as Europe/Paris), or UTC when it is missing or unknown. The value only ever
// reaches time.LoadLocation, never SQL. "Local" would be the server's zone,
// not the viewer's, so it is refused too.
func requestLocation(r *http.Request) *time.Location {
	name := strings.TrimSpace(r.URL.Query().Get("tz"))
	if name == "" || name == "Local" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// activityClock places stored times on the viewer's calendar. Timeline events
// are real instants: their day is the viewer's day at that instant. Interview
// times are floating: the wall-clock time the user typed, stored labelled
// UTC, so their day is the stored date as is, and one counts as held once
// the viewer's own wall clock has passed it.
type activityClock struct {
	loc     *time.Location
	wallNow time.Time // the viewer's wall clock, labelled UTC like interview times
}

func newActivityClock(now time.Time, loc *time.Location) activityClock {
	l := now.In(loc)
	return activityClock{
		loc:     loc,
		wallNow: time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second(), 0, time.UTC),
	}
}

// today is the viewer's current day, as a UTC-labelled midnight.
func (c activityClock) today() time.Time {
	return time.Date(c.wallNow.Year(), c.wallNow.Month(), c.wallNow.Day(), 0, 0, 0, 0, time.UTC)
}

type activityEntry struct {
	day  string    // the viewer's day, "2006-01-02"
	wall time.Time // the viewer's wall-clock time, for ordering
	item models.ActivityItem
}

// loadActivity returns the user's activity on the viewer's days from..to
// (inclusive, UTC-labelled midnights): the timeline events of existing
// applications, automatic ones excepted, and the non-cancelled interviews
// held so far. The heatmap counts it per day and the day panel lists one day,
// both from here, so a cell's count is always the length of its list.
func (h *Handler) loadActivity(ctx context.Context, clock activityClock, from, to time.Time) ([]activityEntry, error) {
	const dayFmt = "2006-01-02"
	fromKey, toKey := from.Format(dayFmt), to.Format(dayFmt)
	var out []activityEntry

	// The SQL bounds only narrow the scan, a day wider than needed on each
	// side; the exact cut is made in Go on the viewer's calendar.
	lo := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, clock.loc).AddDate(0, 0, -1)
	hi := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, clock.loc).AddDate(0, 0, 2)
	err := func() error {
		rows, err := h.db.QueryContext(ctx, `SELECT te.created_at, te.event_type, te.description,
			a.id, a.company_name, a.job_title, a.status
			FROM timeline_events te
			JOIN applications a ON a.id = te.application_id
			WHERE replace(replace(te.created_at, 'T', ' '), 'Z', '') >= ?
			  AND replace(replace(te.created_at, 'T', ' '), 'Z', '') < ?
			  AND `+userEventSQL, sqliteTime(lo), sqliteTime(hi))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e activityEntry
			var raw any
			if err := rows.Scan(&raw, &e.item.EventType, &e.item.Description,
				&e.item.ApplicationID, &e.item.CompanyName, &e.item.JobTitle, &e.item.Status); err != nil {
				log.Printf("activity scan: %v", err)
				continue
			}
			t, text, ok := scanTimeText(raw)
			if !ok {
				continue
			}
			l := t.In(clock.loc)
			e.day = l.Format(dayFmt)
			if e.day < fromKey || e.day > toKey {
				continue
			}
			e.wall = time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second(), l.Nanosecond(), time.UTC)
			e.item.Time = text
			out = append(out, e)
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}

	// Interviews held count as activity too: an interview is the most
	// significant thing that can happen to an application, and holding it
	// leaves no timeline row of its own.
	err = func() error {
		rows, err := h.db.QueryContext(ctx, `SELECT i.scheduled_at, i.type, i.round,
			a.id, a.company_name, a.job_title, a.status
			FROM interviews i
			JOIN applications a ON a.id = i.application_id
			WHERE i.scheduled_at IS NOT NULL
			  AND COALESCE(i.outcome, '') != 'Cancelled'
			  AND replace(replace(i.scheduled_at, 'T', ' '), 'Z', '') >= ?
			  AND replace(replace(i.scheduled_at, 'T', ' '), 'Z', '') < ?`,
			sqliteTime(from.AddDate(0, 0, -1)), sqliteTime(to.AddDate(0, 0, 2)))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e activityEntry
			var raw any
			var typ string
			var round int
			if err := rows.Scan(&raw, &typ, &round,
				&e.item.ApplicationID, &e.item.CompanyName, &e.item.JobTitle, &e.item.Status); err != nil {
				log.Printf("activity interview scan: %v", err)
				continue
			}
			t, text, ok := scanTimeText(raw)
			if !ok || t.After(clock.wallNow) {
				continue
			}
			e.day = t.Format(dayFmt)
			if e.day < fromKey || e.day > toKey {
				continue
			}
			e.wall = t
			e.item.Time = text
			e.item.EventType = "interview_held"
			e.item.Description = fmt.Sprintf("%s · round %d", typ, round)
			out = append(out, e)
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanTimeText is scanTime that also returns the text to send to the client:
// the stored string as is, or RFC 3339 when the driver parsed it already.
func scanTimeText(v any) (time.Time, string, bool) {
	t, ok := scanTime(v)
	if !ok {
		return time.Time{}, "", false
	}
	switch x := v.(type) {
	case string:
		return t, x, true
	case []byte:
		return t, string(x), true
	}
	return t, t.Format(time.RFC3339Nano), true
}

// loadActivityHeatmap counts the activity of each of the viewer's last
// heatmapDays days (see loadActivity), oldest first.
func (h *Handler) loadActivityHeatmap(ctx context.Context, now time.Time, loc *time.Location) ([]models.ActivityDay, error) {
	clock := newActivityClock(now, loc)
	today := clock.today()
	entries, err := h.loadActivity(ctx, clock, today.AddDate(0, 0, -heatmapDays), today)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.day]++
	}
	var days []models.ActivityDay
	for d, c := range counts {
		days = append(days, models.ActivityDay{Date: d, Count: c})
	}
	sort.Slice(days, func(a, b int) bool { return days[a].Date < days[b].Date })
	return days, nil
}

// GetActivityByDay lists the activity of one of the viewer's days (?tz=, as
// for the heatmap), newest first, each item joined with its application.
// GET /api/activity?date=YYYY-MM-DD&tz=Europe/Paris
func (h *Handler) GetActivityByDay(w http.ResponseWriter, r *http.Request) {
	day, err := time.Parse("2006-01-02", strings.TrimSpace(r.URL.Query().Get("date")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	clock := newActivityClock(time.Now(), requestLocation(r))
	entries, err := h.loadActivity(r.Context(), clock, day, day)
	if err != nil {
		log.Printf("GetActivityByDay: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load activity")
		return
	}
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].wall.After(entries[b].wall) })
	items := make([]models.ActivityItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, e.item)
	}
	writeJSON(w, http.StatusOK, items)
}
