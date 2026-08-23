#!/usr/bin/env bash
# Recover after missing deploy/.env (DOMAIN variable is not set).
# On VPS:  cd ~/connect/deploy && bash recover-vps.sh
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo ""
  echo "Created .env from .env.example — EDIT IT NOW:"
  echo "  nano .env"
  echo ""
  echo "Set at least:"
  echo "  DOMAIN=worthyjoin.online"
  echo "  VPS_PUBLIC_IP=<your VPS public IP>"
  echo "  TURN_SECRET=<32+ char secret>"
  echo "  CONNECT_ADMIN_TOKEN=<admin login token>"
  echo ""
  echo "Then run:  bash recover-vps.sh"
  exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

for var in DOMAIN VPS_PUBLIC_IP TURN_SECRET; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: $var is empty in .env — run: nano .env"
    exit 1
  fi
done

if [[ "$TURN_SECRET" == *"generate-a-long"* ]] || [[ ${#TURN_SECRET} -lt 16 ]]; then
  echo "ERROR: Set a real TURN_SECRET in .env (16+ chars)"
  exit 1
fi

echo "Regenerating coturn.conf..."
sed -e "s/CHANGE_ME_VPS_PUBLIC_IP/${VPS_PUBLIC_IP}/g" \
    -e "s/CHANGE_ME_TURN_SECRET/${TURN_SECRET}/g" \
    coturn.conf.template > coturn.conf

echo "Starting stack..."
docker compose up -d --build
docker compose ps

echo ""
echo "Health check (local):"
curl -sf --max-time 8 "https://${DOMAIN}/api/health" && echo "" || echo "HTTPS not ready yet — check: docker compose logs caddy --tail 30"
