package handlers

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"job-ctrl/internal/models"
)

// noReplyDesc is the exact timeline description written by MarkNoReply.
// It follows the "Status changed from X to Y" shape the stats code parses.
const noReplyDesc = "Status changed from Applied to NoReply"

type noReplyCandidate struct {
	id     string
	sentAt time.Time
}

// MarkNoReply moves every stale "Applied" application to "NoReply" and returns
// the number of transitions. days <= 0 disables the feature and returns 0.
//
// The countdown starts at the LATEST of COALESCE(applied_at, created_at) and
// the most recent "... to Applied" timeline event, so re-applying (or moving
// the application back to Applied by hand) restarts it.
func (h *Handler) MarkNoReply(ctx context.Context, now time.Time, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	now = now.UTC()
	cutoff := now.AddDate(0, 0, -days)

	candidates, err := h.noReplyCandidates(ctx)
	if err != nil {
		return 0, err
	}
	reapplied, err := h.lastAppliedEvents(ctx)
	if err != nil {
		return 0, err
	}

	var stale []string
	for _, c := range candidates {
		sentAt := c.sentAt
		if at, ok := reapplied[c.id]; ok && at.After(sentAt) {
			sentAt = at
		}
		if sentAt.Before(cutoff) {
			stale = append(stale, c.id)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stamp := sqliteTime(now)
	var n int
	for _, id := range stale {
		res, err := tx.ExecContext(ctx,
			`UPDATE applications SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
			models.StatusNoReply, stamp, id, models.StatusApplied)
		if err != nil {
			return n, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return n, err
		}
		if affected == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO timeline_events (id, application_id, event_type, description, created_at)
			 VALUES (?,?,?,?,?)`,
			uuid.New().String(), id, "status_change", noReplyDesc, stamp); err != nil {
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// noReplyCandidates returns every "Applied" application with its sent date
// (applied_at, falling back to created_at).
func (h *Handler) noReplyCandidates(ctx context.Context) ([]noReplyCandidate, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, COALESCE(applied_at, created_at) FROM applications WHERE status = ?`,
		models.StatusApplied)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []noReplyCandidate
	for rows.Next() {
		var c noReplyCandidate
		var raw any
		if err := rows.Scan(&c.id, &raw); err != nil {
			return nil, err
		}
		t, ok := scanTime(raw)
		if !ok {
			// No usable sent date: never guess an application is dead.
			continue
		}
		c.sentAt = t
		out = append(out, c)
	}
	return out, rows.Err()
}

// lastAppliedEvents returns, per application, the most recent status change
// whose target status is "Applied".
func (h *Handler) lastAppliedEvents(ctx context.Context) (map[string]time.Time, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT application_id, description, created_at FROM timeline_events
		 WHERE event_type = 'status_change' AND description LIKE '% to Applied'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]time.Time{}
	for rows.Next() {
		var appID, desc string
		var raw any
		if err := rows.Scan(&appID, &desc, &raw); err != nil {
			return nil, err
		}
		if statusChangeTarget(desc) != string(models.StatusApplied) {
			continue
		}
		t, ok := scanTime(raw)
		if !ok {
			continue
		}
		if cur, seen := out[appID]; !seen || t.After(cur) {
			out[appID] = t
		}
	}
	return out, rows.Err()
}

// RunNoReplyJob runs MarkNoReply immediately, then every `every` until ctx is
// done. Errors are logged and the loop keeps going: a transient DB error must
// not kill the background job.
func (h *Handler) RunNoReplyJob(ctx context.Context, days int, every time.Duration) {
	if days <= 0 || every <= 0 {
		return
	}
	run := func() {
		n, err := h.MarkNoReply(ctx, time.Now().UTC(), days)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("no-reply: %v", err)
			}
			return
		}
		if n > 0 {
			log.Printf("no-reply: marked %d application(s)", n)
		}
	}

	run()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
