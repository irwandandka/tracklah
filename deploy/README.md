# Deploying TrackLah to the homelab

Production runs on **vm-4** (192.168.100.163) as the `tracklah` compose project in `/opt/tracklah`. The data services are the homelab's shared infra LXCs, not containers here:

| Need | Where |
|---|---|
| Postgres | `postgres.lan` (192.168.100.150), database `tracklah`, role `tracklah` |
| RabbitMQ | `messaging.lan` (192.168.100.155), vhost `tracklah`, user `tracklah_mq` |
| Redis | `redis.lan` (192.168.100.152), shared `requirepass` |
| Public HTTPS | Cloudflare Tunnel (`cloudflared` already on vm-4) → `http://127.0.0.1:3300` |

## How a deploy happens

On every push to `main`, the root `Jenkinsfile` on jenkins.lan tests everything, pushes images to GHCR tagged with the short commit SHA, then runs:

```
ssh deploy@192.168.100.163 <sha> < deploy/docker-compose.yml
```

The `deploy` user's key is locked to `/opt/tracklah/deploy.sh` (forced command), so it can't do anything else. `deploy.sh` validates the compose file, pulls that tag, waits for health checks, and rolls back to the previous tag if they fail.

Manual deploy or rollback on vm-4: `sudo -u deploy /opt/tracklah/deploy.sh <sha>`.

## Files on vm-4 (not in git)

```
/opt/tracklah/                    owner deploy, 750
├── deploy.sh                     copy of deploy/deploy.sh, owner root, 755
├── docker-compose.yml            written by deploy.sh
├── .env                          secrets, owner deploy, 600 (see below)
├── .tag                          TRACKLAH_TAG=<sha> currently running
└── secrets/firebase-adminsdk.json  owner 10001 (the containers' uid), 400
```

`.env` holds: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `RABBITMQ_URL` (with `/tracklah` vhost), `REDIS_ADDR`, `REDIS_PASSWORD`, `FIRESTORE_PROJECT_ID`.

`deploy`'s `~/.docker/config.json` has a GHCR login made with a classic PAT that has **only** `read:packages` (the packages are private). This is a different token from the one Jenkins pushes with.

## Updating deploy.sh

Jenkins never changes `deploy.sh` itself (that would let the deploy key rewrite its own restriction). After editing `deploy/deploy.sh`, copy it over as root:

```
scp deploy/deploy.sh root@192.168.100.163:/opt/tracklah/deploy.sh
```

`deploy/known_hosts` pins vm-4's SSH host key. If vm-4 is rebuilt, update it from `/etc/ssh/ssh_host_ed25519_key.pub` on the server.

## Metrics (Phase 4)

Every service serves Prometheus metrics on `:9464` in its container. `docker-compose.yml` publishes them on vm-4's LAN address only (not loopback, so the Cloudflare Tunnel can't reach them):

| Port on vm-4 | Service | Highlights |
|---|---|---|
| 9464 | api | `tracklah_http_request_duration_seconds{method,route,status}`, `tracklah_trips_created_total`, `tracklah_trip_transitions_total{status,reason}`, Node.js process metrics |
| 9465 | location | `tracklah_location_pings_consumed_total`, `tracklah_location_sink_errors_total{sink}`, `tracklah_location_firestore_write_seconds` |
| 9466 | driver-simulator | `tracklah_sim_drivers_online`, `tracklah_sim_pending_trips`, `tracklah_sim_trips_total{outcome}`, `tracklah_sim_location_pings_total` |
| 9467 | trip-events-consumer | `tracklah_trip_events_consumed_total{consumer,status}` |

Prometheus on observability.lan scrapes them with the job in `observability/prometheus-scrape.yml` (append it to `/root/observability/prometheus/prometheus.yml`, then `docker compose restart prometheus` in `/root/observability`).
The Grafana dashboard is `observability/grafana-tracklah-dashboard.json` (uid `tracklah`). Import it through Dashboards → New → Import, or POST it to `/api/dashboards/db`. Logs already reach Loki through promtail on vm-4 (`{host="vm-4", container=~"tracklah-.*"}`).
