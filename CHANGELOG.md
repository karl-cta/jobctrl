# Changelog

## Unreleased

Your data is safe across this upgrade: the database is snapshotted before the one new migration runs. That migration removes rows that earlier versions left behind when you deleted an application (its interviews, contacts and timeline events, which were no longer visible anywhere). See "Upgrading" in the README.

### Data
- The SQLite settings the app asked for were silently ignored by the driver: foreign keys, write-ahead logging and the busy timeout are now actually on. Deleting an application now deletes its interviews, contacts and history with it.
- The database now runs in WAL mode: `job-ctrl.db-wal` and `job-ctrl.db-shm` sit next to it while the app runs. The rollback steps in the README changed accordingly.
- Each migration runs in a transaction, so an interrupted upgrade can be retried.
- Restoring a backup is all or nothing, accepts files up to 64 MiB, and brings back applications deleted since the backup with all their interviews and contacts.
- The app shuts down cleanly on `docker stop` (SIGTERM) and closes the database.

### Security
- Write requests from other websites are rejected (cross-site request forgery). Behind a reverse proxy, forward the original `Host` header.
- A Content-Security-Policy is sent, and several places where imported or fetched data could inject HTML or script are escaped (application ids, favicons, the status filter in the URL, the search box, email links).
- URL auto-fill checks every connection it opens, including redirects, against private and reserved networks, and ignores `HTTP_PROXY`/`HTTPS_PROXY`.
- Request bodies are capped at 1 MiB (64 MiB for imports) and the HTTP server has timeouts.
- Fonts are served by the app: pages no longer load anything from Google Fonts.

### Dashboard
- Every indicator opens a list that contains exactly the number shown, for the selected period.
- The activity heatmap and its day details use your local days instead of UTC days; creations are no longer counted twice, and automatic "No reply" moves are not counted as activity.
- An application with an interview counts as a reply in the journey.
- Loading errors show a retry instead of an empty dashboard; switching period quickly no longer shows stale numbers.

### Applications
- Interview times show the time you entered, whatever your time zone. Application dates no longer shift by one day.
- Changing a status from the detail page no longer erases the application date on the next save; editing an interview keeps its preparation notes.
- Salaries that are not a multiple of 1000 can be saved, and the currency found by auto-fill is kept.
- Unsaved notes, preparation and offer text trigger a warning before leaving the page.
- The status filter and search are kept in the URL and survive going back, switching language or theme.
- Duplicate detection, search and A-Z sort ignore accents and case.
- Applications with an interview are no longer moved to "No reply".

### API
- `GET /api/applications` accepts `sent=1`, `period=30|90|365|all` and a comma-separated `status`.
- `GET /api/stats` and `GET /api/activity` accept `tz` (IANA time zone).
- `PUT /api/applications/bulk/status` accepts an optional `applied_at`.
- Unknown `/api/*` routes answer 404 JSON; creating or listing interviews or contacts of an unknown application answers 404; updates return the stored row.

## 1.1.0 — 2026-09-22

Your data is safe across this upgrade: the app snapshots the database next to itself before applying its one migration, and the migration only remaps a removed status while keeping a visible trace in each affected timeline. See "Upgrading" in the README.

### Dashboard
- Redesigned around a period selector (all time, 30, 90 or 365 days): six clickable indicators with trend sparklines, an "in progress" card for applications in screening or interview, a 12-week sent-vs-replies timeline with per-week details, a six-month activity heatmap with per-day event lists, a journey funnel, a full status breakdown, and an expandable sources list.
- Panels can be reordered from the options menu; the order is kept locally.
- New caption typeface (Bricolage Grotesque); JetBrains Mono is no longer loaded.

### Statuses
- New "No reply" status. Applications left in "Applied" for more than 30 days move there automatically (`JOB_CTRL_NO_REPLY_DAYS`, `0` disables).
- "Withdrawn" is removed. Existing rows go back to "Applied" with a timeline entry, then follow the no-reply rule.

### Lists
- Filters "with reply" and "with interview", reachable from the dashboard indicators.

### Fixes
- Importing a backup no longer drops interviews of type "Screening" or with outcome "Rejected", and no longer skips applications with a legacy status; everything the importer refuses is logged.
- Interview types, outcomes, contract types and work modes are translated in the UI.
- Clearing one list filter no longer wipes the others from the URL.

### API
- `GET /api/stats?period=…` gains `period`, `weekly`, `active_processes`, `upcoming_interviews`; the unused `over_time`, `offer_rate`, `active_interviews`, `avg_salary`, `salary_distribution` and `avg_days_in_status` fields are gone.
- `GET /api/activity?date=YYYY-MM-DD` lists the events of one day.
- `GET /api/applications` accepts `has_reply=1` and `has_interviews=1`.

## 1.0.0

Initial release.
