# Job-Ctrl

Suivi de candidatures auto-hébergé. Un binaire Go, une base SQLite, zéro dépendance cloud.

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-003B57?logo=sqlite&logoColor=white)
![Docker](https://img.shields.io/github/actions/workflow/status/karl-cta/jobctrl/docker.yml?label=Docker&logo=docker&logoColor=white)

---

## Lancer avec Docker

```bash
docker compose up -d
```

Ouvrir [http://localhost:8080](http://localhost:8080).

Les données sont persistées dans un volume Docker (`job-ctrl-data`).

Pour une version spécifique :

```bash
docker pull ghcr.io/karl-cta/jobctrl:0.1.0
```

<details>
<summary>Build local</summary>

```bash
git clone https://github.com/karl-cta/jobctrl.git
cd jobctrl
docker compose up -d --build
```

Ajouter `build: .` au service dans `docker-compose.yml` pour builder depuis les sources.

</details>

## Lancer sans Docker

**Prérequis** : Go 1.26+, Node.js 22+

```bash
git clone https://github.com/karl-cta/jobctrl.git
cd jobctrl
make install   # npm install du frontend
make build     # compile frontend + binaire Go
./job-ctrl     # lance le serveur sur :8080
```

## Configuration

| Variable | Défaut | Description |
|----------|--------|-------------|
| `JOB_CTRL_DB_PATH` | `job-ctrl.db` | Chemin du fichier SQLite |
| `JOB_CTRL_ADDR` | `:8080` | Adresse d'écoute |

## Développement

```bash
make install   # deps frontend
make dev       # backend :8080 + frontend :5173 (hot reload)
go test ./...  # tests
```

## API

Toutes les routes sont sous `/api`, réponses en JSON.

| Méthode | Route | Description |
|---------|-------|-------------|
| `GET` | `/api/applications` | Liste (filtres: `status`, `search`, `sort`) |
| `POST` | `/api/applications` | Créer |
| `GET/PUT/DELETE` | `/api/applications/:id` | Détail / modifier / supprimer |
| `GET/POST` | `/api/applications/:id/interviews` | Entretiens |
| `GET/POST` | `/api/applications/:id/contacts` | Contacts |
| `GET` | `/api/stats` | Statistiques dashboard |
| `GET` | `/api/export` | Export JSON complet |
| `GET` | `/api/health` | Health check |

## Stack

Go + [chi](https://github.com/go-chi/chi) / SQLite (pure Go, sans CGO) / TypeScript vanilla + Vite + Tailwind / Docker

## Licence

MIT
