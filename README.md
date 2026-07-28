# ScrumPoker

ScrumPoker is a lightweight, real-time planning poker application for agile teams. Create a room, invite participants with a room code or link, vote privately, reveal estimates together, and start a new round without creating accounts.

## Features

- Real-time rooms over WebSockets
- Fibonacci-style estimates, unknown, and coffee-break votes
- Private votes until the host reveals the round
- Online participant indicators
- Shareable room codes and links
- Persistent rooms and rounds
- SQLite for simple local use
- PostgreSQL and Supabase support
- Hexagonal backend architecture

## Technology

- Frontend: React, TypeScript, Vite, Tailwind CSS
- Backend: Go, `net/http`, WebSockets
- Databases: SQLite or PostgreSQL

## Project Structure

```text
ScrumPoker/
|-- backend/
|   |-- application/              # Use cases and repository ports
|   |-- domain/                   # Entities, rules, and domain errors
|   |-- infrastructure/
|   |   |-- config/               # Environment loading
|   |   |-- httpapi/              # HTTP and WebSocket adapter
|   |   |-- postgres/             # PostgreSQL adapter
|   |   `-- sqlite/               # SQLite adapter
|   `-- main.go                   # Composition root
`-- frontend/
    `-- src/                       # React application
```

The backend follows hexagonal architecture. The domain has no dependency on HTTP or a database. The application layer defines the `RoomRepository` port, and the infrastructure layer supplies SQLite, PostgreSQL, HTTP, and WebSocket adapters.

## Prerequisites

- Go 1.25 or newer
- Node.js 20 or newer
- npm
- Optional: a PostgreSQL or Supabase database

## Quick Start

### 1. Configure the backend

Run these commands from the repository root.

macOS or Linux:

```bash
cp backend/.env.example backend/.env
```

Windows PowerShell:

```powershell
Copy-Item backend/.env.example backend/.env
```

The template uses SQLite by default, so no external database is required.

### 2. Start the backend

```bash
cd backend
go mod download
go run .
```

The API starts at `http://localhost:8080`.

### 3. Start the frontend

In another terminal:

```bash
cd frontend
npm install
npm run dev
```

Open the URL printed by Vite, normally `http://localhost:5173`.

## Environment Variables

The backend loads `.env` from its current working directory. Running it from `backend/` therefore loads `backend/.env`. Existing process environment variables take precedence over values in the file.

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_DRIVER` | `sqlite` | Database adapter: `sqlite`, `postgres`, or `postgresql` |
| `DATABASE_PATH` | `poker.db` | SQLite database path |
| `DATABASE_URL` | none | PostgreSQL connection URI; required for PostgreSQL |
| `ADDR` | `:8080` | Backend listen address |
| `FRONTEND_DIR` | empty | Optional directory containing a built frontend to serve |

The frontend supports one optional build-time variable:

| Variable | Default | Description |
| --- | --- | --- |
| `VITE_WS_URL` | inferred | WebSocket base URL, such as `wss://api.example.com` |

The frontend container supports one runtime variable:

| Variable | Default | Description |
| --- | --- | --- |
| `BACKEND_URL` | `http://backend:8080` | Internal backend URL used by Nginx for `/api` and `/ws` proxying |

## Database Configuration

### SQLite

SQLite is the default and is suitable for local development or a small single-instance deployment.

```env
DATABASE_DRIVER=sqlite
DATABASE_PATH=poker.db
```

The backend creates and migrates the database automatically.

### PostgreSQL

```env
DATABASE_DRIVER=postgres
DATABASE_URL=postgresql://user:password@localhost:5432/scrumpoker?sslmode=disable
```

The configured user must be able to create tables and indexes. Schema migration runs automatically when the backend starts.

### Supabase

No Supabase SDK is required for database access. Use the PostgreSQL **Session pooler** URI from the Supabase dashboard under **Connect**:

```env
DATABASE_DRIVER=postgres
DATABASE_URL=postgresql://postgres.PROJECT_REF:URL_ENCODED_PASSWORD@SESSION_POOLER_HOST:5432/postgres?sslmode=require
```

Use the exact URI supplied by Supabase. The direct database hostname is often IPv6-only, so the Session pooler is recommended on IPv4 networks. URL-encode special characters in the password, and never expose this URI to the frontend.

## Production Build

Build the frontend:

```bash
cd frontend
npm ci
npm run build
```

To serve that build from Go, set `FRONTEND_DIR` relative to the directory where the backend starts:

```env
FRONTEND_DIR=../frontend/dist
```

Then start the backend from `backend/` and open `http://localhost:8080`.

For a separate frontend deployment, configure its hosting platform to proxy `/api` to the backend and set `VITE_WS_URL` before building.

## Docker

### Backend image

The backend image contains only the API and WebSocket server. Build it from the repository root:

```bash
docker build -t scrumpoker-backend ./backend
```

Run it with SQLite and a persistent volume:

```bash
docker run --rm -p 8080:8080 \
  -v scrumpoker-data:/data \
  scrumpoker-backend
```

Run it with PostgreSQL or Supabase:

```bash
docker run --rm -p 8080:8080 \
  -e DATABASE_DRIVER=postgres \
  -e "DATABASE_URL=postgresql://USER:PASSWORD@HOST:5432/postgres?sslmode=require" \
  scrumpoker-backend
```

Alternatively, pass the backend environment file:

```bash
docker run --rm -p 8080:8080 --env-file backend/.env scrumpoker-backend
```

The container runs as a non-root user, exposes port `8080`, and has an HTTP health check at `/api/health`. Do not bake `.env` or credentials into the image.

### Frontend image

The frontend image builds the Vite application and serves it from unprivileged Nginx on port `8080`:

```bash
docker build -t scrumpoker-frontend ./frontend
```

At runtime, `BACKEND_URL` tells Nginx where to proxy `/api` and `/ws`. The URL is resolved inside the frontend container and is not a browser-facing URL.

```bash
docker run --rm -p 3000:8080 \
  -e BACKEND_URL=http://host.docker.internal:8080 \
  scrumpoker-frontend
```

Open `http://localhost:3000`. On Linux, use the backend container's network name or add an appropriate host mapping instead of `host.docker.internal`.

To run both images on one Docker network with SQLite:

```bash
docker network create scrumpoker

docker run -d --name backend --network scrumpoker \
  -v scrumpoker-data:/data \
  scrumpoker-backend

docker run --rm --name frontend --network scrumpoker \
  -p 3000:8080 \
  -e BACKEND_URL=http://backend:8080 \
  scrumpoker-frontend
```

The frontend container provides SPA route fallback, long-lived caching for hashed assets, API proxying, WebSocket upgrades, and a container health check. `VITE_WS_URL` remains available as an optional Docker build argument when the browser must connect directly to a different WebSocket endpoint:

```bash
docker build \
  --build-arg VITE_WS_URL=wss://api.example.com \
  -t scrumpoker-frontend ./frontend
```

## GitHub Container Registry

The backend and frontend workflows test their applications and build `linux/amd64` and `linux/arm64` images:

```text
.github/workflows/backend-image.yml
.github/workflows/frontend-image.yml
```

- Pull requests build and test the image without publishing it.
- Pushes to `main` publish `main`, `sha-*`, and `latest` tags.
- Git tags such as `v1.2.3` publish `1.2.3`, `1.2`, and `sha-*` tags.
- Manual workflow runs publish tags generated from the selected ref.

The image names are generated in lowercase as:

```text
ghcr.io/OWNER/REPOSITORY-backend:TAG
ghcr.io/OWNER/REPOSITORY-frontend:TAG
```

For example:

```bash
docker pull ghcr.io/example/scrumpoker-backend:latest
docker pull ghcr.io/example/scrumpoker-frontend:latest
```

The workflow authenticates with the built-in `GITHUB_TOKEN`; no registry password secret is required. The repository workflow permission must allow package writes. After the first publication, set the package visibility to public in the package settings if anonymous pulls are required.

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/health` | Health check |
| `POST` | `/api/rooms` | Create a room and host session |
| `POST` | `/api/rooms/{code}/join` | Join an existing room |
| `GET` | `/api/rooms/{code}?participantId=...` | Load room state |
| `GET` | `/ws?roomCode=...&participantId=...` | Open the real-time connection |

WebSocket client message types are `vote`, `reveal`, and `reset`. The server emits `state` and `error` messages.

## Validation

Backend:

```bash
cd backend
go test ./...
go vet ./...
```

Frontend:

```bash
cd frontend
npm run lint
npm run build
```

## Troubleshooting

### Supabase reports `no such host`

The direct Supabase database endpoint may resolve only over IPv6. Copy the Session pooler connection URI from the Supabase **Connect** panel instead of using `db.PROJECT_REF.supabase.co`.

### PostgreSQL requires `DATABASE_URL`

Set `DATABASE_URL` in `backend/.env`, confirm that `DATABASE_DRIVER=postgres`, and start the backend from the `backend/` directory.

### The frontend cannot reconnect

Confirm that the backend is listening on port `8080`. For a non-default or remote backend, set `VITE_WS_URL` and restart or rebuild Vite.

## Security Notes

- Never commit `.env` or database credentials.
- Keep `DATABASE_URL` on the backend only.
- Participant sessions are stored in browser local storage; this application does not currently provide user accounts or authentication.
- Configure TLS, trusted WebSocket origins, and network access controls before exposing the application publicly.
