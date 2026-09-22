-- The "Withdrawn" status is gone. Existing rows go back to "Applied": they
-- were sent, and the no-reply job will move the old ones to "NoReply" on its
-- own. Each migrated application gets a timeline event so the change is
-- visible in its history, and the old "... to Withdrawn" events are kept.
INSERT INTO timeline_events (id, application_id, event_type, description, created_at)
SELECT 'migration-006-' || id, id, 'status_change', 'Status changed from Withdrawn to Applied', datetime('now')
FROM applications WHERE status = 'Withdrawn';

UPDATE applications SET status = 'Applied', updated_at = datetime('now') WHERE status = 'Withdrawn';
