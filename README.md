<p align="center">
  <img src="frontend/public/favicon.svg" width="64" height="64" alt="JobCtrl" />
</p>

<h1 align="center">JobCtrl</h1>

<p align="center">
  Self-hosted job application tracker. Single Go binary, SQLite database, zero cloud dependencies.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/SQLite-003B57?logo=sqlite&logoColor=white" alt="SQLite" />
  <img src="https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/License-MIT-green" alt="License" />
</p>

<p align="center">
  <img src=".github/jobctrl-demo.gif" alt="JobCtrl Demo" />
</p>

## Features

- **Full pipeline tracking** with statuses from Wishlist to Accepted, interviews, contacts, and timeline
- **Auto-fill from URL** by pasting a job listing link (extracts company, title, salary via JSON-LD / Open Graph)
- **Dashboard** with period-based indicators, an "in progress" card for live interview processes, a 12-week sent-vs-replies timeline, a six-month activity heatmap with per-day details, a journey funnel, status breakdown and sources
- **Follow-up reminders** for applications with no response after an interview (snooze or dismiss)
- **Automatic "no reply"** moves applications with no answer after 30 days out of your pending pile
- **Duplicate detection** warns you when applying to a company you've already contacted
- **Bulk actions** to change status or delete multiple applications at once
- **Sort and filter** by date, company, status, confidence, interest, and source
- **Table and Kanban views** with pagination
- **JSON backup and restore** plus CSV export for spreadsheets
- **Bilingual** French and English
- **Dark mode** with warm stone palette
- **Self-hosted** with your data on your machine, no accounts, no cloud

## Quick start

### Docker (recommended)

```bash
docker compose up -d
```

Open [http://localhost:8080](http://localhost:8080). Data is persisted in a Docker volume.

### From source

Requires Go 1.26+ and Node.js 22+.

```bash
git clone https://github.com/karl-cta/jobctrl.git
cd jobctrl
make install   # frontend deps
make build     # frontend + Go binary
./job-ctrl     # starts on :8080
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `JOB_CTRL_DB_PATH` | `job-ctrl.db` | SQLite database path |
| `JOB_CTRL_ADDR` | `:8080` | Listen address |
| `JOB_CTRL_NO_REPLY_DAYS` | `30` | Days before an unanswered application becomes "No reply" (`0` disables) |

## Upgrading

Your data lives in a single SQLite file and every upgrade is designed to keep it intact:

- **Automatic backup.** Before a schema migration runs on an existing database, the app writes a consistent snapshot next to it (`job-ctrl.db.backup-<timestamp>`) and logs its path. Keep or delete these files as you like.
- **Migrations are additive.** Nothing is deleted from your history. The one status that was removed in this version, "Withdrawn", is mapped back to "Applied" with a visible entry in each affected application's timeline. Applications left unanswered for more than 30 days then move to "No reply" on their own (see `JOB_CTRL_NO_REPLY_DAYS`).
- **Old exports still import.** A JSON backup made with a previous version restores cleanly: legacy statuses are mapped, and interviews that older versions silently dropped ("Screening" type, "Rejected" outcome) are now kept. Anything the importer cannot take is logged rather than lost.

To roll back an upgrade, stop the app and copy the backup file over `job-ctrl.db`.

## Development

```bash
make dev       # Go + Vite hot reload
go test ./...  # backend tests
```

## Stack

Go + [chi](https://github.com/go-chi/chi) · SQLite (pure Go, no CGO) · Vanilla TypeScript + Vite + Tailwind · Docker

## License

MIT
