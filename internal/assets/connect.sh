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

# docker_name_matching prints the first running container whose name matches $1 and
# returns non-zero when there is none. The status has to be set explicitly: the exit
# code of the pipeline is head's, and head succeeds on empty input, so the previous
# version reported "found" on every host that merely had docker installed - which
# made detect_kind pick vktp for a 3x-ui node.
docker_name_matching() {
  have docker || return 1
  match="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -i "$1" | head -1)"
  [ -n "$match" ] || return 1
  printf '%s\n' "$match"
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

# xui_exec runs the real x-ui Go binary in the given container and reports success
# only when grpc-connect actually wired the token.
#
# Two traps, both of which used to report a false success while configuring nothing:
#   - The "x-ui" on PATH is the management wrapper (a shell menu script), NOT the
#     Go binary. It swallows an unknown subcommand like grpc-connect, prints its
#     menu, and exits 0. So the Go binary paths are tried FIRST, and a run counts
#     as success only when it prints the grpc-connect marker; a bare exit 0 with no
#     marker is the wrapper no-op and falls through to the next path.
#   - An x-ui older than the release that added grpc-connect answers "Invalid
#     subcommands"; that is a hard "build too old", not a retry.
xui_exec() {
  container="$1"
  shift
  saw_binary=0
  for binary in /app/x-ui /usr/local/x-ui/x-ui x-ui; do
    # The assignment must not be a bare simple command: under `set -e` a non-zero
    # docker exec would abort the script before the status is inspected.
    if out="$(docker exec "$container" "$binary" "$@" 2>&1)"; then
      status=0
    else
      status=$?
    fi
    # 126/127 = not executable / not found: try the next path.
    if [ "$status" -eq 126 ] || [ "$status" -eq 127 ]; then
      continue
    fi
    saw_binary=1
    # The Go binary prints this line only after it actually registered the token.
    case "$out" in
      *"token registered"*)
        printf '%s\n' "$out"
        return 0
        ;;
    esac
    # A real binary too old to know the subcommand: stop, no other path will help.
    case "$out" in
      *"Invalid subcommands"*|*"usage: x-ui grpc-connect"*)
        [ -n "$out" ] && printf '%s\n' "$out"
        warn "$binary in $container does not support grpc-connect - the 3x-ui build is too old"
        return 1
        ;;
    esac
    # A hard error from a real binary: surface it instead of masking it with the
    # next candidate (typically the no-op wrapper).
    if [ "$status" -ne 0 ]; then
      [ -n "$out" ] && printf '%s\n' "$out"
      return "$status"
    fi
    # Exit 0 with no marker: this was the management wrapper's menu, not the Go
    # binary doing the work. Keep looking.
  done
  if [ "$saw_binary" -eq 1 ]; then
    warn "grpc-connect ran in $container but no x-ui binary confirmed it (only the management wrapper?)"
  fi
  return 127
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
    # Fatal, not a warning: the DB is wired but the panel still cannot reach the
    # listener, so reporting success here is the same silent no-op as above. Uses
    # the host network namespace check too - a container on --network=host
    # publishes nothing yet is reachable.
    if ! docker inspect -f '{{.HostConfig.NetworkMode}}' "$container" 2>/dev/null | grep -q '^host$' \
      && ! docker port "$container" 2>/dev/null | grep -q ":${XUI_GRPC_PORT}"; then
      die "container $container does not publish port ${XUI_GRPC_PORT}; recreate it with -p ${XUI_GRPC_PORT}:${XUI_GRPC_PORT} so the panel can reach the gRPC (the DB is already wired, so just recreating the container is enough)."
    fi
    log "3x-ui in container $container connected to panel $PANEL_GRPC"
  elif have x-ui || [ -x /usr/local/x-ui/x-ui ]; then
    # Same wrapper trap as the container path: the "x-ui" on PATH may be the shell
    # management menu, which swallows grpc-connect and exits 0. Try the Go binary
    # first and require the success marker; a bare exit 0 with no marker is a no-op.
    xui_ok=0
    for binary in /usr/local/x-ui/x-ui x-ui; do
      command -v "$binary" >/dev/null 2>&1 || [ -x "$binary" ] || continue
      out="$("$binary" grpc-connect "$PANEL_GRPC" "$TOKEN" "$NODE_ID" "0.0.0.0:${XUI_GRPC_PORT}" 2>&1)" || true
      [ -n "$out" ] && printf '%s\n' "$out"
      case "$out" in
        *"token registered"*) xui_ok=1; break ;;
      esac
    done
    [ "$xui_ok" -eq 1 ] || die "grpc-connect did not register the token; the 'x-ui' found may be only the management wrapper, not the Go binary"
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
