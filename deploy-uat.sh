#!/usr/bin/env bash
# UAT deploy: same steps as the prod script, but only touches docker-compose.uat.yml
set -euo pipefail

F="docker-compose.uat.yml"

# backend folder only (has .env.uat): refuse to run against prod's database or port
if [ -f .env.uat ]; then
  grep -Eq '^DATABASE_URL=.*/erp_db_uat([?]|$)' .env.uat \
    || { echo "!! DATABASE_URL in .env.uat does not point to erp_db_uat. Stopping."; exit 1; }
  grep -Eq '^PORT=8081$' .env.uat \
    || { echo "!! PORT in .env.uat must be 8081 (prod uses 8080). Stopping."; exit 1; }
fi

echo "==> [1/3] Pulling latest code from origin/main..."
git pull origin main

echo "==> [2/3] Rebuilding UAT docker image (no cache)..."
docker compose -f "$F" build --no-cache

echo "==> [3/3] Recreating UAT container..."
docker compose -f "$F" up -d --force-recreate

echo "==> Done. Current UAT container status:"
docker compose -f "$F" ps
