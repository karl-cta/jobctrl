# Changelog

## 1.0.1 — 2026-09-22

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
