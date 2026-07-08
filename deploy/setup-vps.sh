#!/usr/bin/env bash
# Bootstrap VPS: generate coturn.conf from .env, open firewall hints, start stack.
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created deploy/.env — set DOMAIN, VPS_PUBLIC_IP, TURN_SECRET, then re-run."
  exit 1
fi

# shellcheck disable=SC1091
source .env

for var in DOMAIN VPS_PUBLIC_IP TURN_SECRET; do
  if [[ -z "${!var:-}" ]]; then
    echo "Missing $var in deploy/.env"
    exit 1
  fi
done

if [[ "$TURN_SECRET" == *"generate-a-long"* ]] || [[ ${#TURN_SECRET} -lt 16 ]]; then
  echo "Set a strong TURN_SECRET in deploy/.env (16+ chars)"
  exit 1
fi

sed \
  -e "s/CHANGE_ME_TURN_SECRET/${TURN_SECRET}/g" \
  -e "s/CHANGE_ME_PUBLIC_IP/${VPS_PUBLIC_IP}/g" \
  coturn.conf.template > coturn.conf

echo "Wrote coturn.conf (realm=connect, external-ip=${VPS_PUBLIC_IP})"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found — install Docker Engine + Compose plugin first."
  exit 1
fi

docker compose up -d --build

echo ""
echo "VPS stack started."
echo "  Dashboard: https://${DOMAIN}/dashboard/"
echo "  Health:    https://${DOMAIN}/api/health  (turnExternal should be true)"
echo "  ICE:       https://${DOMAIN}/api/ice"
echo ""
echo "Firewall (on VPS):"
echo "  TCP 80, 443  — HTTPS / WSS"
echo "  UDP 3478     — TURN/STUN"
echo "  UDP 49152-65535 — TURN relay ports"
echo ""
echo "Windows agent:"
echo "  .\\deploy\\start-vps-agent.ps1 -Server wss://${DOMAIN}/ws"
