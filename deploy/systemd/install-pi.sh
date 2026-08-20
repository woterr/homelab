#!/usr/bin/env bash
# install-pi.sh — install homelab on the Raspberry Pi as native systemd services.
#
# Run with sudo. Idempotent: safe to re-run after a `git push`.
#
# Everything lives in ~/homelab (/home/user/homelab): the repo clone AND the
# SQLite data dir. A leftover /var/lib/homelab from an earlier layout is
# migrated into ~/homelab/data and then removed.
#
# It deliberately does NOT touch the NAS: Docker, the filestash stack, Samba,
# and the NAS's own Caddy on :80 are all left as-is. Only the homelab backend
# and a dedicated homelab Caddy (:8090) are managed here.
#
# Usage: sudo ./deploy/systemd/install-pi.sh

set -euo pipefail

REPO="git@github.com:woterr/homelab.git"
DEST="/home/user/homelab"
DATA_DIR="$DEST/data"
RUN_USER="user"
SYS_CONF="/etc/homelab.env"
OLD_DATA="/var/lib/homelab"

log() { printf '\n\033[1;32m==> %s\033[0m\n' "$*"; }

if [[ $EUID -ne 0 ]]; then
  echo "run with sudo: sudo $0" >&2
  exit 1
fi

log "Installing prerequisites (git)"
apt-get update -qq
apt-get install -y -qq git >/dev/null || true

log "Ensuring '$RUN_USER' is in the docker + video groups"
usermod -a -G docker "$RUN_USER" || true
usermod -a -G video "$RUN_USER" || true

# As root, ssh-agent is usually empty; fall back to the user's GitHub key.
if [[ -f /home/user/.ssh/id_ed25519 ]] && [[ ! -f /root/.ssh/id_ed25519 ]]; then
  export GIT_SSH_COMMAND="ssh -i /home/user/.ssh/id_ed25519 -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"
fi

log "Fetching repo -> $DEST"
if [[ -d "$DEST/.git" ]]; then
  git -C "$DEST" pull --ff-only
else
  if [[ -d "$DEST" ]]; then
    mv "$DEST" "$DEST.old"
    log "Moved non-git $DEST aside -> $DEST.old"
  fi
  git clone "$REPO" "$DEST"
fi

log "Preparing data dir $DATA_DIR"
install -d -o "$RUN_USER" -g "$RUN_USER" -m 0755 "$DATA_DIR"
if [[ -f "$OLD_DATA/homelab.db" ]]; then
  cp -f "$OLD_DATA"/homelab.db* "$DATA_DIR"/ 2>/dev/null || true
  chown -R "$RUN_USER:$RUN_USER" "$DATA_DIR"
  log "Migrated data from old $OLD_DATA layout"
  rm -rf "$OLD_DATA"
fi

log "Installing systemd units"
install -o root -g root -m 0644 "$DEST/deploy/systemd/homelab-backend.service" /etc/systemd/system/
install -o root -g root -m 0644 "$DEST/deploy/systemd/homelab-caddy.service" /etc/systemd/system/

log "Seeding $SYS_CONF"
if [[ ! -f "$SYS_CONF" ]]; then
  install -o root -g root -m 0600 "$DEST/deploy/systemd/homelab.env.example" "$SYS_CONF"
  log "Created $SYS_CONF — edit it for ntfy/discord/smtp/phone, then 'sudo systemctl restart homelab-backend'"
fi

log "Enabling + starting services"
systemctl daemon-reload
systemctl enable --now homelab-backend.service
systemctl enable --now homelab-caddy.service

log "Verifying"
systemctl --no-pager --no-legend status homelab-backend.service || true
sleep 1
http=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8090 || echo fail)
echo "frontend :8090 -> HTTP $http"
curl -s http://localhost:8090/api/v1/system | head -c 200; echo
echo
echo "Done. Dashboard: http://<pi-ip>:8090  (backend :8000, logs: journalctl -fu homelab-backend)"