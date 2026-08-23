#!/usr/bin/env bash
# One command on VPS:  cd ~/connect/deploy && git pull && bash up.sh
exec "$(dirname "$0")/setup-vps.sh" "$@"
