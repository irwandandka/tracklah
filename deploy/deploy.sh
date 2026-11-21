#!/usr/bin/env bash
# Deploys one TrackLah release on vm-4. Installed as /opt/tracklah/deploy.sh.
#
# Jenkins runs it through the deploy user's SSH key, whose authorized_keys
# entry forces this command: the key can do nothing but deploy. The git
# SHA comes from the SSH command line, the compose file on stdin:
#
#   ssh deploy@vm-4 <sha> < deploy/docker-compose.yml
#
# By hand on the server: /opt/tracklah/deploy.sh <sha>  (keeps the
# current compose file).
#
# If the new release doesn't come up healthy, it rolls back to the
# previous tag and exits non-zero, so the Jenkins build fails.
set -euo pipefail
cd /opt/tracklah

tag="${SSH_ORIGINAL_COMMAND:-${1:-}}"
if [[ ! "$tag" =~ ^[0-9a-f]{7,40}$ ]]; then
  echo "deploy.sh: expected a git SHA, got '${tag}'" >&2
  exit 2
fi

compose() { docker compose --env-file .env --env-file "$1" -f "$2" "${@:3}"; }

echo "TRACKLAH_TAG=$tag" > .tag.new
file=docker-compose.yml
if [ ! -t 0 ]; then
  cat > docker-compose.yml.new
  if [ -s docker-compose.yml.new ]; then
    compose .tag.new docker-compose.yml.new config --quiet
    file=docker-compose.yml.new
  fi
fi

echo "deploy.sh: deploying $tag"
compose .tag.new "$file" pull --quiet
if compose .tag.new "$file" up -d --remove-orphans --wait --wait-timeout 180; then
  [ "$file" = docker-compose.yml.new ] && mv docker-compose.yml.new docker-compose.yml
  mv .tag.new .tag
  docker image prune -f >/dev/null
  echo "deploy.sh: $tag is up"
  exit 0
fi

echo "deploy.sh: $tag did not become healthy" >&2
compose .tag.new "$file" ps >&2 || true
compose .tag.new "$file" logs --tail 50 >&2 || true
rm -f .tag.new docker-compose.yml.new
if [ -f .tag ]; then
  echo "deploy.sh: rolling back to $(cut -d= -f2 .tag)" >&2
  compose .tag docker-compose.yml up -d --remove-orphans --wait --wait-timeout 180 >&2 || true
fi
exit 1
