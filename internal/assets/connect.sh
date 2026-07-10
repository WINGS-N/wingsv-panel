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
  # The panel renders "<token>" as a placeholder when the node has no token yet.
  # Running with it would wire a garbage credential, so refuse it explicitly.
  case "$TOKEN" in
    '<token>' | '<TOKEN>')
      die "token is a placeholder ('<token>') - the node has no token in the panel yet; generate/set it in the panel first, then copy the real command"
      ;;
  esac
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

# upsert_key sets KEY = "VAL" in a flat TOML file, editing an existing active line
# in place or appending it, and never touching any other line. This is what keeps
# the operator's listen/udp-connect/wrap/cert settings intact.
upsert_key() {
  f="$1"
  key="$2"
  val="$3"
  if grep -qE "^[[:space:]]*${key}[[:space:]]*=" "$f" 2>/dev/null; then
    tmp="$f.tmp.$$"
    awk -v k="$key" -v v="$val" '
      !done && $0 ~ ("^[[:space:]]*" k "[[:space:]]*=") && $0 !~ /^[[:space:]]*#/ {
        print k " = \"" v "\""; done = 1; next
      }
      { print }
    ' "$f" > "$tmp" && mv "$tmp" "$f"
  else
    printf '%s = "%s"\n' "$key" "$val" >> "$f"
  fi
}

# write_vktp_config MERGES the panel wiring keys into an existing config.toml,
# preserving every other line, and backs the file up first. It NEVER truncates the
# file - an earlier version did, which wiped operators' full relay configuration.
write_vktp_config() {
  target_dir="$1"
  target_file="$target_dir/config.toml"
  mkdir -p "$target_dir"
  if [ -f "$target_file" ]; then
    backup="$target_file.bak.$(date +%Y%m%d%H%M%S 2>/dev/null || echo bak)"
    cp "$target_file" "$backup" && log "backed up existing config to $backup"
  else
    : > "$target_file"
    log "no existing $target_file; creating one with panel wiring only"
  fi
  upsert_key "$target_file" grpc-listen "0.0.0.0:${VKTP_GRPC_PORT}"
  upsert_key "$target_file" grpc-token "${TOKEN}"
  upsert_key "$target_file" panel-grpc "${PANEL_GRPC}"
  upsert_key "$target_file" node-id "${NODE_ID}"
  # One token wires the node both ways: the relay uses grpc-token to authenticate
  # the panel's inbound management calls and, when panel-token is unset (as here),
  # falls back to the same value for its outbound provisioning bearer. No second
  # copy of the credential.
  log "merged panel wiring into $target_file"
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
    # Merge the panel wiring into the container's config at the default path, then
    # restart it. Seed the local copy from the container so its existing settings
    # are preserved (a docker cp back would otherwise replace the whole file).
    docker exec "$container" sh -c "mkdir -p /etc/wings/vktp" 2>/dev/null || die "cannot exec into container $container"
    mkdir -p /tmp/wings-vktp
    docker cp "$container":/etc/wings/vktp/config.toml /tmp/wings-vktp/config.toml 2>/dev/null || true
    write_vktp_config "/tmp/wings-vktp"
    docker cp /tmp/wings-vktp/config.toml "$container":/etc/wings/vktp/config.toml
    rm -rf /tmp/wings-vktp
    docker restart "$container" >/dev/null
    log "merged config into container $container and restarted it"
  else
    write_vktp_config "$(dirname "$VKTP_CONFIG")"
    warn "no systemd unit or container found; wrote $VKTP_CONFIG - start vk-turn-server so it reads it."
  fi
  log "vk-turn-proxy node $NODE_ID connected to panel $PANEL_GRPC"
}

# xui_exec runs the x-ui binary in the given container, trying common paths.
xui_exec() {
  container="$1"
  shift
  docker exec "$container" x-ui "$@" 2>/dev/null && return 0
  docker exec "$container" /app/x-ui "$@" 2>/dev/null && return 0
  docker exec "$container" /usr/local/x-ui/x-ui "$@"
}

connect_xui() {
  # x-ui grpc-connect enables the management gRPC (persisted in the DB), registers
  # the token, and points vk-turn inbounds at the panel - covering both plain 3x-ui
  # and vk-turn running as a 3x-ui inbound.
  container="$(docker_name_matching '3x-ui' || docker_name_matching 'x-ui' || true)"
  if [ -n "$container" ]; then
    xui_exec "$container" grpc-connect "$PANEL_GRPC" "$TOKEN" "$NODE_ID" "0.0.0.0:${XUI_GRPC_PORT}" \
      || die "x-ui grpc-connect failed in container $container"
    docker restart "$container" >/dev/null
    if ! docker port "$container" 2>/dev/null | grep -q ":${XUI_GRPC_PORT}"; then
      warn "container $container does not publish port ${XUI_GRPC_PORT}; recreate it with -p ${XUI_GRPC_PORT}:${XUI_GRPC_PORT} so the panel can reach the gRPC."
    fi
    log "3x-ui in container $container connected to panel $PANEL_GRPC"
  elif have x-ui; then
    x-ui grpc-connect "$PANEL_GRPC" "$TOKEN" "$NODE_ID" "0.0.0.0:${XUI_GRPC_PORT}" \
      || die "x-ui grpc-connect failed"
    systemctl restart x-ui 2>/dev/null || warn "restart x-ui so the gRPC listener comes up"
    log "3x-ui connected to panel $PANEL_GRPC"
  else
    die "3x-ui not found locally"
  fi
  log "3x-ui node $NODE_ID: management gRPC :$XUI_GRPC_PORT, panel $PANEL_GRPC"
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
