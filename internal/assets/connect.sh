#!/bin/sh
# WINGS panel node connector.
#
#   curl -fsSL https://v.wingsnet.org/connect.sh | sh -s -- \
#     grpc connect <panel_grpc> <token> <node-id> [vktp|xui|auto]
#
# It detects a local vk-turn-proxy or 3x-ui (host binary, systemd, or docker) and
# wires it to the panel in both directions: it enables the node's management gRPC
# with the given token (so the panel can drive it) and sets panel-grpc/node-id so
# apps can self-enroll their wg config over DTLS PROVISION.
#
# vk-turn is fully automated through /etc/wings/vktp/config.toml. 3x-ui needs its
# XUI_GRPC_LISTEN env and an API token, which live in the container/service config
# and its database; the script does what it safely can and prints the rest.
set -eu

PANEL_GRPC=""
TOKEN=""
NODE_ID=""
KIND="auto"
VKTP_GRPC_PORT="${VKTP_GRPC_PORT:-25612}"
XUI_GRPC_PORT="${XUI_GRPC_PORT:-25613}"
VKTP_CONFIG="/etc/wings/vktp/config.toml"

log() { printf '\033[1;34m[connect]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[connect]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[connect]\033[0m %s\n' "$*" >&2; exit 1; }

parse_args() {
  [ "${1:-}" = "grpc" ] || die "usage: connect.sh grpc connect <panel_grpc> <token> <node-id> [vktp|xui|auto]"
  [ "${2:-}" = "connect" ] || die "usage: connect.sh grpc connect <panel_grpc> <token> <node-id> [vktp|xui|auto]"
  PANEL_GRPC="${3:-}"
  TOKEN="${4:-}"
  NODE_ID="${5:-}"
  KIND="${6:-auto}"
  [ -n "$PANEL_GRPC" ] || die "missing <panel_grpc> (e.g. v.wingsnet.org:443)"
  [ -n "$TOKEN" ] || die "missing <token>"
  [ -n "$NODE_ID" ] || die "missing <node-id> (copy it from the panel node list)"
}

have() { command -v "$1" >/dev/null 2>&1; }

# docker_name_matching prints the first running container whose name matches $1.
docker_name_matching() {
  have docker || return 1
  docker ps --format '{{.Names}}' 2>/dev/null | grep -i "$1" | head -1
}

detect_kind() {
  [ "$KIND" != "auto" ] && return 0
  if systemctl list-units --type=service 2>/dev/null | grep -qi 'vk-turn-server' \
    || [ -x /root/vk-turn-proxy/vk-turn-server ] || docker_name_matching 'vk-turn' >/dev/null 2>&1; then
    KIND="vktp"
  elif have x-ui || [ -d /etc/x-ui ] || docker_name_matching '3x-ui' >/dev/null 2>&1 || docker_name_matching 'x-ui' >/dev/null 2>&1; then
    KIND="xui"
  else
    die "could not detect a local vk-turn-proxy or 3x-ui; pass the kind explicitly (vktp|xui)"
  fi
  log "detected node kind: $KIND"
}

write_vktp_config() {
  target_dir="$1"
  target_file="$target_dir/config.toml"
  mkdir -p "$target_dir"
  # Preserve any listen/udp-connect the operator already relies on by only writing
  # the panel wiring keys; vk-turn merges this file under the command-line flags,
  # so a unit that still passes explicit flags will keep overriding these.
  cat > "$target_file" <<EOF
# Written by connect.sh - panel wiring for this vk-turn-proxy node.
grpc-listen = "0.0.0.0:${VKTP_GRPC_PORT}"
grpc-token = "${TOKEN}"
panel-grpc = "${PANEL_GRPC}"
node-id = "${NODE_ID}"
panel-token = "${TOKEN}"
EOF
  log "wrote $target_file"
}

connect_vktp() {
  container="$(docker_name_matching 'vk-turn' || true)"
  if systemctl list-units --type=service 2>/dev/null | grep -qi 'vk-turn-server'; then
    write_vktp_config "$(dirname "$VKTP_CONFIG")"
    if grep -qE '\-grpc-listen|\-panel-grpc' "$(systemctl show -p FragmentPath --value vk-turn-server.service 2>/dev/null || echo /dev/null)" 2>/dev/null; then
      warn "the vk-turn-server unit passes explicit gRPC/panel flags that override the config file;"
      warn "remove them from ExecStart so $VKTP_CONFIG takes effect, then: systemctl restart vk-turn-server"
    else
      systemctl restart vk-turn-server.service
      log "restarted vk-turn-server.service"
    fi
  elif [ -n "$container" ]; then
    # Write the config inside the container at the default path, then restart it.
    docker exec "$container" sh -c "mkdir -p /etc/wings/vktp" 2>/dev/null || die "cannot exec into container $container"
    write_vktp_config "/tmp/wings-vktp"
    docker cp /tmp/wings-vktp/config.toml "$container":/etc/wings/vktp/config.toml
    rm -rf /tmp/wings-vktp
    docker restart "$container" >/dev/null
    log "wrote config into container $container and restarted it"
  else
    write_vktp_config "$(dirname "$VKTP_CONFIG")"
    warn "no systemd unit or container found; wrote $VKTP_CONFIG - start vk-turn-server so it reads it."
  fi
  log "vk-turn-proxy node $NODE_ID connected to panel $PANEL_GRPC"
}

connect_xui() {
  container="$(docker_name_matching '3x-ui' || docker_name_matching 'x-ui' || true)"
  log "3x-ui management gRPC needs XUI_GRPC_LISTEN set and the token registered as an API token."
  if [ -n "$container" ]; then
    warn "3x-ui runs in container '$container'. Recreate it with the gRPC port exposed and enabled:"
    printf '    docker rm -f %s\n' "$container"
    printf '    docker run -d --name %s ... -p %s:%s -e XUI_GRPC_LISTEN=0.0.0.0:%s <image>\n' \
      "$container" "$XUI_GRPC_PORT" "$XUI_GRPC_PORT" "$XUI_GRPC_PORT"
  elif have x-ui || [ -d /etc/x-ui ]; then
    warn "host 3x-ui detected. Set XUI_GRPC_LISTEN=0.0.0.0:$XUI_GRPC_PORT in its service environment and restart x-ui."
  else
    die "3x-ui not found locally"
  fi
  warn "Then in the 3x-ui panel: create an API token equal to <token>, and on the vk-turn inbound set"
  warn "panel-grpc=$PANEL_GRPC and node-id=$NODE_ID (Panel provisioning section) to enable DTLS PROVISION."
  log "3x-ui node $NODE_ID: management port $XUI_GRPC_PORT, panel $PANEL_GRPC"
}

main() {
  parse_args "$@"
  detect_kind
  case "$KIND" in
    vktp) connect_vktp ;;
    xui) connect_xui ;;
    *) die "unknown kind: $KIND (use vktp|xui|auto)" ;;
  esac
}

main "$@"
