package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"job-ctrl/internal/models"
)

// The dashboard period metrics are computed in Go from a few small queries
// rather than in SQL: a personal tracker holds hundreds of rows at most, and
// the windowing and bucketing are far clearer here than in SQLite.

const (
	defaultPeriodDays = 90
	sparkBuckets      = 8
	timelineWeeks     = 12
)

// parsePeriod turns the ?period= query value into a number of days (0 = all).
func parsePeriod(raw string) int {
	switch strings.TrimSpace(raw) {
	case "30":
		return 30
	case "90", "":
		return defaultPeriodDays
	case "365":
		return 365
	case "all":
		return 0
	}
	return defaultPeriodDays
}

// scanTime converts a value scanned from a DATETIME column into a time.Time.
// modernc/sqlite may hand back either time.Time or the raw string, and stored
// strings come in the "2006-01-02 15:04:05" form or RFC3339 (import paths).
func scanTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return x.UTC(), true
	case []byte:
		return parseDBTime(string(x))
	case string:
		return parseDBTime(x)
	}
	return time.Time{}, false
}

func parseDBTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// statusChangeTarget extracts "Y" from "Status changed from X to Y".
func statusChangeTarget(desc string) string {
	if idx := strings.LastIndex(desc, " to "); idx >= 0 {
		return strings.TrimSpace(desc[idx+4:])
	}
	return ""
}

func isRespondedStatus(s string) bool {
	switch models.ApplicationStatus(s) {
	case models.StatusScreening, models.StatusInterviewing, models.StatusOffer,
		models.StatusAccepted, models.StatusRejected:
		return true
	}
	return false
}

// isSentStatus reports whether the application was actually sent. NoReply is
// a sent application that never got an answer, so it counts here but never in
// isRespondedStatus.
func isSentStatus(s string) bool {
	return s == string(models.StatusApplied) ||
		s == string(models.StatusNoReply) ||
		isRespondedStatus(s)
}

type sentApp struct {
	id     string
	status string
	sentAt time.Time
}

type replyEvent struct {
	appID string
	at    time.Time
}

// loadSentApps returns every application with the moment it was "sent"
// (applied_at, falling back to created_at).
func (h *Handler) loadSentApps(ctx context.Context) []sentApp {
	rows, err := h.db.QueryContext(ctx, `SELECT id, status, COALESCE(applied_at, created_at) FROM applications`)
	if err != nil {
		log.Printf("GetStats sentApps: %v", err)
		return nil
	}
	defer rows.Close()
	var out []sentApp
	for rows.Next() {
		var a sentApp
		var raw any
		if err := rows.Scan(&a.id, &a.status, &raw); err != nil {
			log.Printf("GetStats sentApps scan: %v", err)
			continue
		}
		t, ok := scanTime(raw)
		if !ok {
			continue
		}
		a.sentAt = t
		out = append(out, a)
	}
	return out
}

// loadReplyEvents returns status_change events whose target status counts as
// a reply from the company.
func (h *Handler) loadReplyEvents(ctx context.Context) []replyEvent {
	rows, err := h.db.QueryContext(ctx, `SELECT application_id, description, created_at
		FROM timeline_events WHERE event_type = 'status_change'`)
	if err != nil {
		log.Printf("GetStats replyEvents: %v", err)
		return nil
	}
	defer rows.Close()
	var out []replyEvent
	for rows.Next() {
		var e replyEvent
		var desc string
		var raw any
		if err := rows.Scan(&e.appID, &desc, &raw); err != nil {
			log.Printf("GetStats replyEvents scan: %v", err)
			continue
		}
		if !isRespondedStatus(statusChangeTarget(desc)) {
			continue
		}
		t, ok := scanTime(raw)
		if !ok {
			continue
		}
		e.at = t
		out = append(out, e)
	}
	return out
}

type interviewTime struct {
	appID     string
	at        time.Time
	scheduled bool
	cancelled bool
}

// loadInterviewTimes returns the scheduled time of every interview
// (falling back to its creation time when unscheduled).
func (h *Handler) loadInterviewTimes(ctx context.Context) []interviewTime {
	rows, err := h.db.QueryContext(ctx, `SELECT application_id, scheduled_at, created_at, COALESCE(outcome, '') FROM interviews`)
	if err != nil {
		log.Printf("GetStats interviewTimes: %v", err)
		return nil
	}
	defer rows.Close()
	var out []interviewTime
	for rows.Next() {
		var appID, outcome string
		var rawSched, rawCreated any
		if err := rows.Scan(&appID, &rawSched, &rawCreated, &outcome); err != nil {
			continue
		}
		cancelled := outcome == string(models.OutcomeCancelled)
		if t, ok := scanTime(rawSched); ok {
			out = append(out, interviewTime{appID: appID, at: t, scheduled: true, cancelled: cancelled})
		} else if t, ok := scanTime(rawCreated); ok {
			out = append(out, interviewTime{appID: appID, at: t, cancelled: cancelled})
		}
	}
	return out
}

// bucketIndex maps a time into one of n equal buckets spanning [start, end).
// Returns -1 when out of range.
func bucketIndex(t, start, end time.Time, n int) int {
	if n <= 0 || !t.Before(end) || t.Before(start) {
		return -1
	}
	span := end.Sub(start)
	if span <= 0 {
		return -1
	}
	idx := int(float64(n) * float64(t.Sub(start)) / float64(span))
	if idx >= n {
		idx = n - 1
	}
	return idx
}

func floatPtr(v float64) *float64 { return &v }

// computePeriodStats builds the KPI row + funnel for the selected period.
func computePeriodStats(now time.Time, days int, apps []sentApp, interviews []interviewTime) models.PeriodStats {
	ps := models.PeriodStats{Days: days}

	var start, prevStart time.Time
	hasPrev := days > 0
	if days > 0 {
		start = now.AddDate(0, 0, -days)
		prevStart = start.AddDate(0, 0, -days)
	} else {
		// All time: start at the earliest sent application (or now when empty).
		start = now
		for _, a := range apps {
			if a.sentAt.Before(start) {
				start = a.sentAt
			}
		}
		for _, iv := range interviews {
			if iv.at.Before(start) {
				start = iv.at
			}
		}
		if start.Equal(now) {
			start = now.AddDate(0, 0, -1)
		}
	}

	inCurrent := func(t time.Time) bool { return !t.Before(start) && t.Before(now.Add(time.Second)) }
	inPrev := func(t time.Time) bool { return hasPrev && !t.Before(prevStart) && t.Before(start) }

	// Applications that got at least one real interview, whatever the round.
	withInterview := map[string]bool{}
	for _, iv := range interviews {
		if !iv.cancelled {
			withInterview[iv.appID] = true
		}
	}

	type counters struct{ sent, responded, interviewing, offers, accepted, rejected, noReply, interviewed int }
	var cur, prev counters
	sentSeries := make([]float64, sparkBuckets)
	respondedSeries := make([]float64, sparkBuckets)
	rejectedSeries := make([]float64, sparkBuckets)
	noReplySeries := make([]float64, sparkBuckets)
	offerSeries := make([]float64, sparkBuckets)
	interviewSeries := make([]float64, sparkBuckets)

	tally := func(c *counters, a sentApp) {
		c.sent++
		if isRespondedStatus(a.status) {
			c.responded++
		}
		// "Reached the interview stage" = had at least one interview, or sits
		// at Interviewing or beyond (older data may have no interview rows).
		if withInterview[a.id] {
			c.interviewed++
			c.interviewing++
		}
		switch models.ApplicationStatus(a.status) {
		case models.StatusInterviewing:
			if !withInterview[a.id] {
				c.interviewing++
			}
		case models.StatusOffer:
			if !withInterview[a.id] {
				c.interviewing++
			}
			c.offers++
		case models.StatusAccepted:
			if !withInterview[a.id] {
				c.interviewing++
			}
			c.offers++
			c.accepted++
		case models.StatusRejected:
			c.rejected++
		case models.StatusNoReply:
			c.noReply++
		}
	}

	for _, a := range apps {
		if !isSentStatus(a.status) {
			continue
		}
		switch {
		case inCurrent(a.sentAt):
			tally(&cur, a)
			if b := bucketIndex(a.sentAt, start, now.Add(time.Second), sparkBuckets); b >= 0 {
				sentSeries[b]++
				if isRespondedStatus(a.status) {
					respondedSeries[b]++
				}
				if withInterview[a.id] {
					interviewSeries[b]++
				}
				switch models.ApplicationStatus(a.status) {
				case models.StatusRejected:
					rejectedSeries[b]++
				case models.StatusNoReply:
					noReplySeries[b]++
				case models.StatusOffer, models.StatusAccepted:
					offerSeries[b]++
				}
			}
		case inPrev(a.sentAt):
			tally(&prev, a)
		}
	}

	rate := func(c counters) float64 {
		if c.sent == 0 {
			return 0
		}
		return float64(c.responded) / float64(c.sent) * 100
	}
	rateSeries := make([]float64, sparkBuckets)
	for i := range rateSeries {
		if sentSeries[i] > 0 {
			rateSeries[i] = respondedSeries[i] / sentSeries[i] * 100
		}
	}

	ps.Sent = models.KPI{Value: float64(cur.sent), Series: sentSeries}
	ps.Responded = models.KPI{Value: float64(cur.responded), Series: respondedSeries}
	ps.ResponseRate = models.KPI{Value: rate(cur), Series: rateSeries}
	ps.Interviews = models.KPI{Value: float64(cur.interviewed), Series: interviewSeries}
	ps.Rejected = models.KPI{Value: float64(cur.rejected), Series: rejectedSeries}
	ps.NoReply = models.KPI{Value: float64(cur.noReply), Series: noReplySeries}
	ps.Offers = models.KPI{Value: float64(cur.offers), Series: offerSeries}
	if hasPrev {
		ps.Sent.Prev = floatPtr(float64(prev.sent))
		ps.Responded.Prev = floatPtr(float64(prev.responded))
		ps.ResponseRate.Prev = floatPtr(rate(prev))
		ps.Interviews.Prev = floatPtr(float64(prev.interviewed))
		ps.Rejected.Prev = floatPtr(float64(prev.rejected))
		ps.NoReply.Prev = floatPtr(float64(prev.noReply))
		ps.Offers.Prev = floatPtr(float64(prev.offers))
	}
	ps.Funnel = models.FunnelStats{
		Sent:         cur.sent,
		Responded:    cur.responded,
		Interviewing: cur.interviewing,
		Offers:       cur.offers,
		Accepted:     cur.accepted,
	}
	return ps
}

// mondayOf returns the Monday (00:00 UTC) of the week containing t.
func mondayOf(t time.Time) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(day.Weekday()) + 6) % 7 // Monday=0 ... Sunday=6
	return day.AddDate(0, 0, -offset)
}

// computeWeekly builds the zero-filled 12-week sent/replies series ending
// with the current week.
func computeWeekly(now time.Time, apps []sentApp, replies []replyEvent) []models.WeeklyPoint {
	thisMonday := mondayOf(now)
	first := thisMonday.AddDate(0, 0, -7*(timelineWeeks-1))
	points := make([]models.WeeklyPoint, timelineWeeks)
	for i := range points {
		points[i].WeekStart = first.AddDate(0, 0, 7*i).Format("2006-01-02")
	}
	weekIdx := func(t time.Time) int {
		m := mondayOf(t)
		if m.Before(first) || m.After(thisMonday) {
			return -1
		}
		return int(m.Sub(first).Hours() / 24 / 7)
	}
	for _, a := range apps {
		if !isSentStatus(a.status) {
			continue
		}
		if i := weekIdx(a.sentAt); i >= 0 {
			points[i].Sent++
		}
	}
	// One reply per application per week at most: a Screening -> Interviewing
	// move the same week is one conversation, not two replies.
	seen := map[string]bool{}
	for _, r := range replies {
		i := weekIdx(r.at)
		if i < 0 {
			continue
		}
		key := r.appID + "#" + points[i].WeekStart
		if seen[key] {
			continue
		}
		seen[key] = true
		points[i].Replies++
	}
	return points
}

// countUpcomingInterviews counts interviews scheduled within the next 7 days,
// ignoring cancelled ones.
func countUpcomingInterviews(now time.Time, interviews []interviewTime) int {
	end := now.AddDate(0, 0, 7)
	n := 0
	for _, iv := range interviews {
		if iv.scheduled && !iv.cancelled && !iv.at.Before(now) && iv.at.Before(end) {
			n++
		}
	}
	return n
}
