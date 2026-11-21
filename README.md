# TrackLah

TrackLah is a polyglot ride/live-location tracking backend, built as a learning project: the goal is breadth-first exposure to real-world infra and messaging tools (self-hosted infra, reverse proxies, message brokers, CI/CD, observability, multi-cloud), not a rushed product. Everything is configured raw and manually — no PaaS black boxes — so each piece is understood, not just copy-pasted.

This doc is the "start here" for anyone opening the repo: what exists today, how the pieces fit together, and where to look in the code for each concern.

## 1. Where this project is headed

The intended architecture splits backend responsibilities across three languages, each picked for the problem it's actually good at:

| Service | Language | Responsibility | Status |
|---|---|---|---|
| `services/api` | NestJS | Core REST API, trip lifecycle, Postgres, RabbitMQ/Redis producers | ✅ built |
| `services/location` | Go | Location ping ingestion (RabbitMQ), Redis pub/sub, Firestore writes | ✅ built |
| `services/driver-simulator` | Go | Fakes a fleet of drivers: roams, publishes location pings, picks up and drives requested trips, consumes commands | ✅ built |
| `services/trip-events-consumer` | Go | Three independent consumers proving the fanout pattern | ✅ built |
| `services/edge` | Rust (Cloudflare Workers/WASM) | Geofence validation, rate-limiting, edge ETA | 🔜 not started |

Phase 1 (foundation) and Phase 2 (real-time core) are both done — see §7. The rest of this document describes what's actually in the repo right now.

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

Traefik has no static routing config — it watches the Docker socket and auto-discovers containers via labels (see `docker-compose.yml`, the `api` service's `labels:` block). `location` (Go) is wired into Traefik the same way, at `location.tracklah.localhost`.

That covers direct HTTP requests. The other half of the system is message-driven and doesn't go through Traefik at all:

```
driver-simulator ──[RabbitMQ: direct]──▶ api            (Case 1: commands, e.g. "cancel trip")
driver-simulator ──[RabbitMQ: topic]───▶ location        (Case 2: location pings, wildcard-bound)
api (trips.service) ──[RabbitMQ: fanout]▶ trip-events-consumer   (Case 3: lifecycle broadcast, 3 independent queues)
api (trips.service) ──[RabbitMQ: fanout]▶ driver-simulator       (same fanout, 4th queue: picks up "requested" trips)
driver-simulator ──[HTTP: PATCH /trips/:id/status]──▶ api        (assigned → in_progress → completed)
location ──[Redis pub/sub]──▶ api (LocationUpdatesService)        (fire-and-forget, no persistence)
location ──[Firestore]──▶ driverLocations/{driverId}               (durable "current position" snapshot)
```

Each arrow is a deliberately different messaging pattern — see `documentation/` (gitignored, local-only notes) for the full breakdown of why each one was chosen where it was.

## 3. Repo layout

```
tracklah/
├── docker-compose.yml            # the whole local stack (10 containers)
├── .env.example                  # FIRESTORE_CREDENTIALS_PATH, FIRESTORE_PROJECT_ID
└── services/
    ├── api/                      # NestJS core API
    │   ├── src/
    │   │   ├── main.ts               # bootstrap, global ValidationPipe
    │   │   ├── app.module.ts         # wires ConfigModule + TypeOrmModule + feature modules
    │   │   ├── trips/                 # trip CRUD + lifecycle (see §4)
    │   │   ├── commands/              # RabbitMQ Case 1: direct exchange, publishes to drivers
    │   │   ├── rabbitmq/              # shared connection/channel, used by commands + trips
    │   │   ├── location/              # Redis subscriber, receives updates from Go
    │   │   └── health/                # GET /health (checks Postgres)
    │   └── test/
    ├── location/                 # Go: consumes RabbitMQ pings, publishes Redis + Firestore
    ├── driver-simulator/         # Go: fake driver - publishes pings, consumes commands
    ├── trip-events-consumer/     # Go: 3 goroutines proving RabbitMQ fanout (Case 3)
    ├── jenkins/                  # Custom jenkins/jenkins:lts image + Configuration-as-Code
    ├── jenkins-agent/            # Permanent inbound Build Agent (jenkins/inbound-agent + Go)
    └── edge/                     # 🔜 Rust/WASM, not started
```

As the Rust edge service gets added, it'll live as a sibling under `services/`.

## 4. Reading the code: the `trips` module, in order

If you want to understand how a feature is built here, read `services/api/src/trips/` in this order — it mirrors how a request actually flows through the layers:

1. **`trip-status.enum.ts`** — the trip lifecycle: `requested → assigned → in_progress → completed` (or `cancelled`). This is the vocabulary everything else uses, plus `ALLOWED_TRANSITIONS`, the state machine `PATCH /trips/:id/status` is checked against.
2. **`trip.entity.ts`** — the Postgres table via TypeORM decorators: a trip has a rider, an optional driver, a status, origin/destination coordinates, and timestamps.
3. **`dto/create-trip.dto.ts`** and **`dto/update-trip-status.dto.ts`** — what's accepted over HTTP, validated with `class-validator` (e.g. `@IsLatitude()`) before it ever reaches the service.
4. **`trips.service.ts`** — the actual logic: create, list, find one (throws `NotFoundException` if missing), update status. Talks to Postgres through TypeORM's `Repository<Trip>`, and calls `TripEventsService.publish()` after every create/update.
5. **`trip-events.service.ts`** — broadcasts a `trip.lifecycle` fanout event (RabbitMQ Case 3) every time a trip is created or changes status. Doesn't know or care who's listening — see `trip-events-consumer`.
6. **`trips.controller.ts`** — maps HTTP verbs to service methods: `POST /trips`, `GET /trips?riderId=`, `GET /trips/:id`, `PATCH /trips/:id/status`.
7. **`trips.module.ts`** — registers the entity with TypeORM, imports `RabbitmqModule`, wires controller + both services together.
8. **`app.module.ts`** (one level up) — this is where `TripsModule` gets imported into the app, alongside `ConfigModule` (reads `.env`), `TypeOrmModule.forRootAsync`, `CommandsModule`, and `LocationModule`.

This is the template to copy when the next feature module (e.g. drivers, or trip history) gets added.

## 5. Running it locally

Prerequisites:
- Docker Desktop running
- A Firebase project with Firestore enabled, and a service account key (JSON) downloaded — see [Firebase Console](https://console.firebase.google.com) → your project → ⚙️ Project Settings → Service accounts → Generate new private key. `location` writes to Firestore and won't start without this.

```bash
git clone git@github.com:irwandandka/tracklah.git
cd tracklah
cp .env.example .env
# edit .env: point FIRESTORE_CREDENTIALS_PATH at your downloaded key file
# (keep it OUTSIDE this repo - it's a secret and must never be committed)
docker compose up -d --build
```

This brings up ten containers: `traefik`, `postgres`, `rabbitmq`, `redis`, `api`, `location`, `driver-simulator`, `trip-events-consumer`, `jenkins`, `jenkins-agent`. The schema is managed by TypeORM migrations in `services/api/src/database/migrations/`, applied automatically when the API boots (`DB_MIGRATIONS_RUN=false` turns that off). `synchronize` is off everywhere.

Upgrading a local database created before migrations existed (when `synchronize: true` made the tables): the first migration will fail with "already exists". It only holds test trips, so recreate it: `docker compose exec postgres psql -U tracklah -d postgres -c 'DROP DATABASE tracklah WITH (FORCE)' -c 'CREATE DATABASE tracklah OWNER tracklah'`.

Schema changes: edit the entity, then `npm run migration:generate -- src/database/migrations/<Name>` in `services/api` (against an up-to-date database), review the generated SQL, and commit it.

Note: `jenkins-agent` needs a connection secret Jenkins only generates after the controller's first boot - see `.env.example` for the one-time `curl` command to fetch it into your `.env` before that container will connect successfully.

Try it:

```bash
curl http://api.tracklah.localhost:8000/health   # 200 {"status":"ok","db":"up"}, 503 if Postgres is unreachable

RIDER=$(uuidgen | tr A-Z a-z)   # riderId must be a UUID (the mobile app generates one per install)
curl -X POST http://api.tracklah.localhost:8000/trips \
  -H "Content-Type: application/json" \
  -d '{"riderId":"'$RIDER'","originLat":-6.2,"originLng":106.8,"destinationLat":-6.21,"destinationLng":106.81}'

curl "http://api.tracklah.localhost:8000/trips?riderId=$RIDER"
```

Trip API rules worth knowing:
- `GET /trips` requires `riderId` (there's no auth yet, so it never lists everyone's trips). Results are `createdAt DESC`, paginated with `limit` (default 20, max 100) and `offset`, optionally filtered with `status=completed,cancelled`. Response: `{ data, limit, offset, nextOffset }`, where `nextOffset` is `null` on the last page.
- A rider can have only one active trip (`requested`/`assigned`/`in_progress`). A second `POST /trips` gets `409`. This is enforced by a partial unique index (`UQ_trips_rider_active`), not just an app-level check. If an old local database already has duplicate active trips per rider, the API fails to create that index on boot: cancel or delete the extras first.
- `PATCH /trips/:id/status` only allows `requested→assigned|cancelled`, `assigned→in_progress|cancelled`, `in_progress→completed`. Anything else is `409`. `driverId` is required when assigning and rejected otherwise. The update is a compare-and-set on the previous status, so a concurrent change also loses with `409` instead of being overwritten.
- Malformed trip ids return `400`, not a Postgres `500`.
- Rate limits, per client IP: 120 requests/minute overall (`THROTTLE_LIMIT_PER_MINUTE`), 10/minute for `POST /trips`. Over the limit is `429`. Behind Cloudflare Tunnel set `TRUST_CF_CONNECTING_IP=true` so the limit uses the real client IP, and only when the API is reachable through the tunnel alone (otherwise the header can be spoofed).
- A trip nobody picks up within `TRIP_REQUEST_TIMEOUT_MINUTES` (30) is cancelled automatically by a once-a-minute sweep in `TripsService`. Every cancelled trip carries `cancelReason`: `no_driver_found` for a timeout, `rider_cancelled` for `PATCH .../status` with `cancelled`. The sweep is a single conditional `UPDATE`, so it can't race a driver's assign and is safe with several API replicas.

`driver-simulator` runs every driver in `DRIVER_IDS` (`driver-1,driver-2,driver-3` by default) in one process. Like real drivers, they aren't online 24/7: each one alternates an online shift (`ONLINE_DURATION`, 20–40 min) and an offline break (`OFFLINE_DURATION`, 10–30 min). Offline drivers send no pings at all, so they drop off the app's map (`updatedAt > now-30s`) and take no trips. A shift that ends mid-trip waits until the trip is completed. Set `ALWAYS_ONLINE=true` to switch shifts off while testing.

While online and waiting for an order, a driver roams in straight lines between random waypoints and pings every `IDLE_PING_INTERVAL` (20s). When you create a trip, the next idle online driver takes it: `PATCH assigned` with its `driverId`, drives to the origin, `PATCH in_progress`, drives to the destination, `PATCH completed`, pinging every `TRIP_PING_INTERVAL` (10s) at `SPEED_MPS` (12 m/s). If the rider cancels while the driver is on the way, the driver drops the trip and goes back to roaming. Any `409` from the API (trip already taken or cancelled) just means "skip this one". If every driver is offline, a new trip waits in `requested` until one comes back, for at most 30 minutes (then the API cancels it as `no_driver_found`). Watch `docker logs -f tracklah-driver-simulator-1` for the trip side and `docker logs -f tracklah-location-1` to see pings flow through RabbitMQ, Redis and into Firestore.

Known limitation: if the simulator restarts mid-trip, that trip stays `assigned`/`in_progress` (it isn't resumed). `assigned` can still be cancelled by the rider, but `in_progress` has to be finished by hand: `PATCH /trips/<id>/status` with `{"status":"completed"}`.

Firestore cost: every ping is one Firestore write in `location`. With the defaults (20s idle, 10s on a trip, online about 60% of the time), three drivers come to roughly 10k writes/day, inside the Spark plan's free 20k/day.

Other useful dashboards:
- Traefik (routers, discovered services): `http://localhost:8080`
- RabbitMQ management UI: `http://localhost:15672` (login `tracklah` / `tracklah`)
- Jenkins: `http://jenkins.tracklah.localhost:8000` (login from `JENKINS_ADMIN_USER`/`JENKINS_ADMIN_PASSWORD` in your `.env`) — a `location-build` pipeline job is seeded automatically via Configuration-as-Code (`services/jenkins/casc.yaml`), no setup wizard.

### Running the API outside Docker (faster iteration)

```bash
cd services/api
nvm use 23            # this repo targets Node ^22/^23
cp .env.example .env  # DB_HOST=localhost, DB_PORT=5433, RABBITMQ_URL, REDIS_ADDR
npm install
npm run start:dev
```

Note this still needs Postgres, RabbitMQ and Redis running (`docker compose up -d postgres rabbitmq redis` from the repo root works standalone).

## 6. Local environment quirks (and why)

- **Postgres runs on host port `5433`, not `5432`.** This dev machine already has a native PostgreSQL 16 service bound to 5432 for an unrelated project. If you're on a clean machine, this remap is harmless — just note it when connecting external tools (DBeaver, psql, etc.).
- **Redis runs on host port `6380`, not `6379`.** Same reason — a native Redis (via Homebrew) already owns 6379 on this machine.
- **Traefik's web entrypoint runs on host port `8000`, not `80`.** Same reason again — this machine has an nginx already on 80. `8080` (Traefik's dashboard) was free and left as-is.
- **Traefik is pinned to `v3.7`.** `traefik:v3.5` and `v3.6` have a bug where the Docker provider hardcodes API version `1.24` when talking to the Docker daemon, which newer Docker Engines (this machine: Engine 29.x, `MinAPIVersion` 1.40) reject outright with an unhelpful `400 Bad Request` and no discovered routers. Fixed upstream in Traefik's `v3.7` line — don't downgrade the image without checking this first.
- **The Firestore service account key is bind-mounted, never baked into the image.** `docker-compose.yml` mounts `${FIRESTORE_CREDENTIALS_PATH}` (from your local, gitignored `.env`) read-only into the `location` container. If `location` won't start, check that path is set and the file exists.

## 7. Roadmap

Full rationale and tool-by-tool notes live in the project brainstorm doc (not checked into this repo). Short version of what's next, in order:

- **Phase 1 — Foundation** ✅ done: Docker Compose, Traefik, NestJS CRUD, Postgres, all wired end-to-end.
- **Phase 2 — Real-time core** ✅ done: Go location ingestion, RabbitMQ (direct/topic/fanout — commands, location pings, trip lifecycle events), Redis pub/sub bridging Go → NestJS, Firestore last-known-location writes.
- **Phase 3 — CI/CD** 🚧 in progress: Jenkins controller ✅, Build Agent ✅ (`services/jenkins-agent`, permanent inbound node — `location-build` actually compiles on the agent, not the controller). The root `Jenkinsfile` (run by the homelab `jenkins.lan`) lints, builds and tests every service, then on `main` builds and pushes all four images to GHCR as `ghcr.io/irwandandka/tracklah-<service>:<short-sha>` plus `:latest` (needs a `ghcr-credentials` username/password credential on `jenkins.lan`: GitHub username + classic PAT with `write:packages`). It then deploys that tag to vm-4 over SSH with automatic rollback, see [`deploy/README.md`](deploy/README.md).
- **Phase 4 — Observability**: Prometheus/Grafana, OpenTelemetry tracing across services.
- **Phase 5 — Storage & cloud**: MinIO, Cloudflare Tunnel + R2, AWS SES/S3, the Rust/WASM edge worker.
- **Phase 6 — Advanced**: RabbitMQ DLQ + MQTT, Kafka (replacing RabbitMQ for location ingestion), Firebase Auth, Sentry.
