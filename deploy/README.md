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
