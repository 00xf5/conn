#!/usr/bin/env bash
# Bootstrap VPS: generate coturn.conf from .env, start stack, verify health.
set -euo pipefail
cd "$(dirname "$0")"

STEP=0
FAILURES=0

step() {
  STEP=$((STEP + 1))
  echo ""
  echo "==> [$STEP] $*"
}

ok() {
  echo "    OK: $*"
}

fail() {
  FAILURES=$((FAILURES + 1))
  echo ""
  echo "========================================"
  echo " SETUP VPS: FAILED"
  echo "========================================"
  echo "  $*"
  echo ""
  echo "Debug:"
  echo "  docker compose ps"
  echo "  docker compose logs connectd --tail 30"
  echo "  docker compose logs caddy --tail 30"
  echo "  docker compose logs coturn --tail 30"
  exit 1
}

banner_success() {
  echo ""
  echo "========================================"
  echo " SETUP VPS: SUCCESS"
  echo "========================================"
  echo "  Domain:    https://${DOMAIN}"
  echo "  Dashboard: https://${DOMAIN}/dashboard/"
  echo "  Health:    https://${DOMAIN}/api/health"
  echo "  Agent WSS: wss://${DOMAIN}/ws"
  if [[ -n "${NEW_ADMIN_TOKEN:-}" ]]; then
    echo ""
    echo "  Admin login token (save this): ${NEW_ADMIN_TOKEN}"
  fi
  echo ""
  echo "From Windows PC:"
  echo "  curl.exe -sS https://${DOMAIN}/api/health"
  echo "  .\\deploy\\start-vps-agent.ps1 -Server wss://${DOMAIN}/ws"
  echo ""
  echo "If PC curl still fails, open TCP 80/443 in your cloud firewall."
  echo "========================================"
}

detect_public_ip() {
  local ip=""
  ip=$(curl -sf --max-time 6 https://api.ipify.org 2>/dev/null || true)
  if [[ -z "$ip" ]]; then
    ip=$(curl -sf --max-time 6 https://ifconfig.me/ip 2>/dev/null || true)
  fi
  if [[ -z "$ip" ]]; then
    ip=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
  fi
  echo "$ip"
}

env_needs_write() {
  if [[ ! -f .env ]]; then
    return 0
  fi
  # shellcheck disable=SC1091
  source .env 2>/dev/null || return 0
  [[ -z "${DOMAIN:-}" ]] && return 0
  [[ -z "${VPS_PUBLIC_IP:-}" ]] && return 0
  [[ -z "${TURN_SECRET:-}" ]] && return 0
  [[ "$TURN_SECRET" == *"generate-a-long"* ]] && return 0
  [[ ${#TURN_SECRET} -lt 16 ]] && return 0
  return 1
}

write_env_auto() {
  local domain ip turn admin old_turn old_admin
  domain="worthyjoin.online"
  ip=""
  turn=""
  admin=""
  old_turn=""
  old_admin=""

  if [[ -f .env ]]; then
    old_turn=$(grep -E '^TURN_SECRET=' .env 2>/dev/null | cut -d= -f2- || true)
    old_admin=$(grep -E '^CONNECT_ADMIN_TOKEN=' .env 2>/dev/null | cut -d= -f2- || true)
    domain=$(grep -E '^DOMAIN=' .env 2>/dev/null | cut -d= -f2- || true)
    ip=$(grep -E '^VPS_PUBLIC_IP=' .env 2>/dev/null | cut -d= -f2- || true)
  fi
  if [[ -z "$old_turn" ]] && [[ -f coturn.conf ]]; then
    old_turn=$(grep -E '^static-auth-secret=' coturn.conf 2>/dev/null | cut -d= -f2- || true)
  fi

  [[ -z "$domain" ]] && domain="worthyjoin.online"
  [[ -z "$ip" ]] && ip=$(detect_public_ip)
  [[ -z "$ip" ]] && fail "Could not detect VPS public IP — set VPS_PUBLIC_IP in .env manually"

  if [[ -n "$old_turn" ]] && [[ ${#old_turn} -ge 16 ]] && [[ "$old_turn" != *"generate"* ]] && [[ "$old_turn" != "CHANGE_ME"* ]]; then
    turn="$old_turn"
  else
    turn=$(openssl rand -hex 24)
  fi

  if [[ -n "$old_admin" ]] && [[ ${#old_admin} -ge 16 ]] && [[ "$old_admin" != *"generate"* ]]; then
    admin="$old_admin"
  else
    admin=$(openssl rand -hex 24)
    NEW_ADMIN_TOKEN="$admin"
  fi

  cat > .env <<EOF
DOMAIN=${domain}
VPS_PUBLIC_IP=${ip}
TURN_SECRET=${turn}
CONNECT_ADMIN_TOKEN=${admin}
EOF
  ok "AUTO wrote .env (DOMAIN=${domain} IP=${ip})"
}

step "Ensure deploy/.env"
if env_needs_write; then
  write_env_auto
else
  ok ".env already configured"
fi

# shellcheck disable=SC1091
source .env

for var in DOMAIN VPS_PUBLIC_IP TURN_SECRET; do
  if [[ -z "${!var:-}" ]]; then
    fail "Missing $var in deploy/.env after auto-setup"
  fi
done

if [[ "$TURN_SECRET" == *"generate-a-long"* ]] || [[ ${#TURN_SECRET} -lt 16 ]]; then
  fail "TURN_SECRET still invalid after auto-setup"
fi
ok "DOMAIN=$DOMAIN VPS_PUBLIC_IP=$VPS_PUBLIC_IP"

step "Write coturn.conf from template"
if [[ ! -f coturn.conf.template ]]; then
  fail "Missing coturn.conf.template"
fi
sed \
  -e "s/CHANGE_ME_TURN_SECRET/${TURN_SECRET}/g" \
  -e "s/CHANGE_ME_PUBLIC_IP/${VPS_PUBLIC_IP}/g" \
  coturn.conf.template > coturn.conf
ok "coturn.conf written (external-ip=${VPS_PUBLIC_IP})"

step "Check Docker"
if ! command -v docker >/dev/null 2>&1; then
  fail "Docker not installed. Run: curl -fsSL https://get.docker.com | sh"
fi
if ! docker compose version >/dev/null 2>&1; then
  fail "Docker Compose plugin missing. Install docker-compose-plugin."
fi
ok "docker $(docker --version | cut -d' ' -f3- | tr -d ',')"

step "Build and start containers"
if ! docker compose up -d --build; then
  fail "docker compose up failed"
fi
ok "docker compose up finished"

step "Wait for containers to be running"
sleep 3
missing=""
while read -r name state; do
  if [[ "$state" != "running" ]]; then
    missing="${missing} ${name}=${state}"
  fi
done < <(docker compose ps --format '{{.Name}} {{.State}}' 2>/dev/null || true)

if [[ -n "$missing" ]]; then
  docker compose ps
  fail "Containers not running:${missing}"
fi
docker compose ps
ok "connectd, caddy, coturn running"

step "Wait for HTTPS health (Caddy + TLS cert + connectd)"
health_body=""
health_ok=0
for i in $(seq 1 45); do
  if health_body=$(curl -sf --max-time 5 "https://${DOMAIN}/api/health" 2>/dev/null); then
    health_ok=1
    break
  fi
  echo "    waiting for https://${DOMAIN}/api/health (${i}/45)..."
  sleep 2
done

if [[ "$health_ok" -ne 1 ]]; then
  echo ""
  echo "HTTPS health check failed. Trying HTTP redirect probe..."
  curl -sv --max-time 5 "http://${DOMAIN}/api/health" 2>&1 | tail -5 || true
  fail "Could not reach https://${DOMAIN}/api/health — check Caddy logs and cloud firewall TCP 80/443"
fi

echo "    health: $health_body"

if ! echo "$health_body" | grep -q '"ok":true'; then
  fail "Health returned but ok!=true: $health_body"
fi
ok "health ok=true"

if ! echo "$health_body" | grep -q '"turnExternal":true'; then
  fail "turnExternal is not true — check CONNECT_TURN_URL/SECRET in docker-compose.yml: $health_body"
fi
ok "turnExternal=true (coturn wired to connectd)"

step "Check /api/ice has TURN credentials"
ice_body=""
if ! ice_body=$(curl -sf --max-time 5 "https://${DOMAIN}/api/ice" 2>/dev/null); then
  fail "Could not fetch https://${DOMAIN}/api/ice"
fi
if ! echo "$ice_body" | grep -q 'turn:'; then
  fail "ICE config missing turn: URL: $ice_body"
fi
if ! echo "$ice_body" | grep -q '"username"'; then
  fail "ICE config missing TURN username: $ice_body"
fi
ok "TURN credentials present in /api/ice"

banner_success
exit 0
