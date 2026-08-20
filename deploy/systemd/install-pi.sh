#!/usr/bin/env bash
# install-pi.sh — install homelab on the Raspberry Pi as native systemd services.
#
# Run with sudo. Idempotent: safe to re-run after a `git push`.
#
# It deliberately does NOT touch the NAS: Docker, the filestash stack, Samba,
# and the NAS's own Caddy on :80 are all left as-is. Only the homelab backend
# and a dedicated homelab Caddy (:8090) are managed here.
#
# Usage: sudo ./deploy/systemd/install-pi.sh

set -euo pipefail

REPO="git@github.com:woterr/homelab.git"
DEST="/opt/homelab"
DATA_DIR="/var/lib/homelab"
RUN_USER="homelab"
SYS_CONF="/etc/homelab.env"
OLD_DATA="/home/user/homelab/data"

log() { printf '\n\033[1;32m==> %s\033[0m\n' "$*"; }

if [[ $EUID -ne 0 ]]; then
  echo "run with sudo: sudo $0" >&2
  exit 1
fi

log "Installing prerequisites (git)"
apt-get update -qq
apt-get install -y -qq git >/dev/null || true

# As root, ssh-agent is usually empty; fall back to the user's GitHub key.
if [[ $EUID -eq 0 ]] && [[ -f /home/user/.ssh/id_ed25519 ]] && [[ ! -f /root/.ssh/id_ed25519 ]]; then
  export GIT_SSH_COMMAND="ssh -i /home/user/.ssh/id_ed25519 -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"
fi

log "Creating '$RUN_USER' user (groups: docker, video)"
if ! id -u "$RUN_USER" &>/dev/null; then
  useradd -r -s /usr/sbin/nologin "$RUN_USER"
fi
usermod -a -G docker "$RUN_USER" || true
usermod -a -G video "$RUN_USER" || true

log "Fetching repo -> $DEST"
if [[ -d "$DEST/.git" ]]; then
  git -C "$DEST" pull --ff-only
else
  git clone "$REPO" "$DEST"
fi

log "Preparing data dir $DATA_DIR"
install -d -o "$RUN_USER" -g "$RUN_USER" -m 0755 "$DATA_DIR"
if [[ -f "$OLD_DATA/homelab.db" ]]; then
  cp -n "$OLD_DATA"/homelab.db* "$DATA_DIR"/ 2>/dev/null || true
  chown -R "$RUN_USER:$RUN_USER" "$DATA_DIR"
  log "Preserved existing metrics from $OLD_DATA"
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