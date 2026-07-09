# TrackLah

TrackLah is a polyglot ride/live-location tracking backend, built as a learning project: the goal is breadth-first exposure to real-world infra and messaging tools (self-hosted infra, reverse proxies, message brokers, CI/CD, observability, multi-cloud), not a rushed product. Everything is configured raw and manually — no PaaS black boxes — so each piece is understood, not just copy-pasted.

This doc is the "start here" for anyone opening the repo: what exists today, how the pieces fit together, and where to look in the code for each concern.

## 1. Where this project is headed

The intended architecture splits backend responsibilities across three languages, each picked for the problem it's actually good at:

| Service | Language | Responsibility | Status |
|---|---|---|---|
| `services/api` | NestJS | Core REST API, trip lifecycle, Postgres, auth | ✅ built |
| `services/location` | Go | Real-time location ingestion, WebSocket broadcast, Redis pub/sub | 🔜 not started |
| `services/edge` | Rust (Cloudflare Workers/WASM) | Geofence validation, rate-limiting, edge ETA | 🔜 not started |

Only the NestJS API exists so far. The rest of this document describes what's actually in the repo right now.

## 2. Request flow (how a request gets to your code)

```
                    ┌──────────┐
  curl / browser ──▶│ Traefik  │  (reverse proxy, Docker-label based routing)
                    └────┬─────┘
                         │ Host(`api.tracklah.localhost`)
                         ▼
                    ┌──────────┐
                    │ NestJS   │  services/api
                    │   API    │
                    └────┬─────┘
                         │ TypeORM
                         ▼
                    ┌──────────┐
                    │ Postgres │
                    └──────────┘
```

Traefik has no static routing config — it watches the Docker socket and auto-discovers containers via labels (see `docker-compose.yml`, the `api` service's `labels:` block). This is the pattern that will let more services (Go, later) join the same proxy without touching Traefik's own config.

## 3. Repo layout

```
tracklah/
├── docker-compose.yml       # the whole local stack: traefik, postgres, api
└── services/
    └── api/                 # NestJS core API
        ├── Dockerfile       # multi-stage build (build → slim runtime)
        ├── src/
        │   ├── main.ts          # bootstrap, global ValidationPipe
        │   ├── app.module.ts    # wires ConfigModule + TypeOrmModule + feature modules
        │   └── trips/            # first (and so far only) feature module
        │       ├── trip.entity.ts        # Postgres table shape
        │       ├── trip-status.enum.ts   # lifecycle states
        │       ├── dto/                  # request validation shapes
        │       ├── trips.service.ts      # business logic + repository access
        │       ├── trips.controller.ts   # HTTP routes
        │       └── trips.module.ts       # ties the above together
        └── test/
```

As Go and Rust services get added, they'll live as siblings under `services/`.

## 4. Reading the code: the `trips` module, in order

If you want to understand how a feature is built here, read `services/api/src/trips/` in this order — it mirrors how a request actually flows through the layers:

1. **`trip-status.enum.ts`** — the trip lifecycle: `requested → assigned → in_progress → completed` (or `cancelled`). This is the vocabulary everything else uses.
2. **`trip.entity.ts`** — the Postgres table via TypeORM decorators: a trip has a rider, an optional driver, a status, origin/destination coordinates, and timestamps.
3. **`dto/create-trip.dto.ts`** and **`dto/update-trip-status.dto.ts`** — what's accepted over HTTP, validated with `class-validator` (e.g. `@IsLatitude()`) before it ever reaches the service.
4. **`trips.service.ts`** — the actual logic: create, list, find one (throws `NotFoundException` if missing), update status. Talks to Postgres through TypeORM's `Repository<Trip>`.
5. **`trips.controller.ts`** — maps HTTP verbs to service methods: `POST /trips`, `GET /trips`, `GET /trips/:id`, `PATCH /trips/:id/status`.
6. **`trips.module.ts`** — registers the entity with TypeORM and wires controller + service together as a Nest module.
7. **`app.module.ts`** (one level up) — this is where `TripsModule` gets imported into the app, alongside `ConfigModule` (reads `.env`) and `TypeOrmModule.forRootAsync` (builds the DB connection from env vars).

This is the template to copy when the next feature module (e.g. drivers, or trip history) gets added.

## 5. Running it locally

Prerequisites: Docker Desktop running.

```bash
git clone git@github.com:irwandandka/tracklah.git
cd tracklah
docker compose up -d --build
```

This brings up three containers: `traefik`, `postgres`, `api`. First boot creates the `trips` table automatically (TypeORM `synchronize: true` — fine for this learning phase, would need real migrations before anything resembling production).

Try it:

```bash
curl -X POST http://api.tracklah.localhost:8000/trips \
  -H "Content-Type: application/json" \
  -d '{"riderId":"rider-1","originLat":-6.2,"originLng":106.8,"destinationLat":-6.21,"destinationLng":106.81}'

curl http://api.tracklah.localhost:8000/trips
```

Traefik's dashboard (routers, discovered services) is at `http://localhost:8080`.

### Running the API outside Docker (faster iteration)

```bash
cd services/api
nvm use 23            # this repo targets Node ^22/^23
cp .env.example .env  # DB_HOST=localhost, DB_PORT=5433
npm install
npm run start:dev
```

Note this still needs Postgres running (`docker compose up -d postgres` from the repo root works standalone).

## 6. Local environment quirks (and why)

- **Postgres runs on host port `5433`, not `5432`.** This dev machine already has a native PostgreSQL 16 service bound to 5432 for an unrelated project. If you're on a clean machine, this remap is harmless — just note it when connecting external tools (DBeaver, psql, etc.).
- **Traefik's web entrypoint runs on host port `8000`, not `80`.** Same reason — this machine has an nginx already on 80. `8080` (Traefik's dashboard) was free and left as-is.
- **Traefik is pinned to `v3.7`.** `traefik:v3.5` and `v3.6` have a bug where the Docker provider hardcodes API version `1.24` when talking to the Docker daemon, which newer Docker Engines (this machine: Engine 29.x, `MinAPIVersion` 1.40) reject outright with an unhelpful `400 Bad Request` and no discovered routers. Fixed upstream in Traefik's `v3.7` line — don't downgrade the image without checking this first.

## 7. Roadmap

Full rationale and tool-by-tool notes live in the project brainstorm doc (not checked into this repo). Short version of what's next, in order:

- **Phase 1 — Foundation** ✅ done: Docker Compose, Traefik, NestJS CRUD, Postgres, all wired end-to-end.
- **Phase 2 — Real-time core**: Go service for location ping ingestion, RabbitMQ command pattern, Redis pub/sub, Firestore writes.
- **Phase 3 — CI/CD**: Jenkins controller + build agent, Docker build/push, SSH deploy.
- **Phase 4 — Observability**: Prometheus/Grafana, OpenTelemetry tracing across services.
- **Phase 5 — Storage & cloud**: MinIO, Cloudflare Tunnel + R2, AWS SES/S3, the Rust/WASM edge worker.
- **Phase 6 — Advanced**: RabbitMQ DLQ + MQTT, Kafka (replacing RabbitMQ for location ingestion), Firebase Auth, Sentry.
