#!/usr/bin/env bash
#
# wingsv-panel turnkey installer.
#
#   curl -fsSL https://raw.githubusercontent.com/WINGS-N/wingsv-panel/main/install.sh | bash
#
# Installs the panel as a standalone binary (default) or a container (--docker),
# asks about certificates / port / an optional local vk-turn-proxy node / an
# optional local 3x-ui wiring, and leaves a working HTTPS panel that can mint
# client configs. See ./install.sh --help.
#
set -euo pipefail

# --------------------------------------------------------------------------- #
# Configurable knobs (env-overridable)
# --------------------------------------------------------------------------- #
PANEL_REPO="${PANEL_REPO:-WINGS-N/wingsv-panel}"
VKTP_REPO="${VKTP_REPO:-WINGS-N/vk-turn-proxy}"
PANEL_IMAGE="${PANEL_IMAGE:-ghcr.io/wings-n/wingsv-panel:latest}"

BIN_DIR=/usr/local/bin
PANEL_BIN="$BIN_DIR/wingsv-panel"
VKTP_BIN="$BIN_DIR/wings-vktp"
PANEL_CFG_DIR=/etc/wings/panel
VKTP_CFG_DIR=/etc/wings/vktp
PANEL_CFG="$PANEL_CFG_DIR/config.toml"
VKTP_CFG="$VKTP_CFG_DIR/config.toml"
PANEL_DATA_DIR=/var/lib/wings/panel
PANEL_DB="$PANEL_DATA_DIR/v-wingsnet.db"
CA_DIR="$PANEL_CFG_DIR/certs"
VKTP_KEY_FILE="$VKTP_CFG_DIR/wg-server.key"
SVC_USER=wings

PANEL_SVC=wingsv-panel
VKTP_SVC=wings-vktp

# Ports (defaults; panel port is prompted)
PANEL_PORT=8443
PROV_PORT=9091          # provisioning gRPC (node -> panel)
VKTP_DTLS_PORT=56000    # vk-turn-proxy data plane (UDP)
VKTP_GRPC_PORT=25612    # panel -> vktp management gRPC (loopback)
VKTP_WG_PORT=51820      # WireGuard listen (UDP)
VKTP_WG_CIDR=10.66.66.0/24
VKTP_WG_ADDR=10.66.66.1/24
XUI_GRPC_PORT=25613     # panel -> 3x-ui management gRPC

MODE=bin
ASSUME_YES=0
DO_UNINSTALL=0

# --------------------------------------------------------------------------- #
# Output helpers
# --------------------------------------------------------------------------- #
c_reset=$'\033[0m'; c_blue=$'\033[1;34m'; c_yellow=$'\033[1;33m'; c_red=$'\033[1;31m'; c_green=$'\033[1;32m'
log()  { printf '%s==>%s %s\n' "$c_blue" "$c_reset" "$*"; }
ok()   { printf '%s  ok%s %s\n' "$c_green" "$c_reset" "$*"; }
warn() { printf '%s warn%s %s\n' "$c_yellow" "$c_reset" "$*" >&2; }
die()  { printf '%serror%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

usage() {
  cat <<EOF
wingsv-panel installer

Usage: install.sh [--docker] [--yes] [--uninstall]

  --docker      run the panel as a container (default: standalone binary)
  --yes         accept defaults, do not prompt (non-interactive)
  --uninstall   stop and remove the panel + local node services

Env overrides: PANEL_REPO, VKTP_REPO, PANEL_IMAGE.
EOF
}

# --------------------------------------------------------------------------- #
# Prompt helpers (respect --yes)
# --------------------------------------------------------------------------- #
# When the installer is piped (curl ... | bash) its stdin IS the script, so a
# plain `read` hits EOF and every prompt silently takes its default (that is how
# "choose 1/2/3" fell through to self-signed and then died on an empty host).
# Read prompts from the controlling terminal instead so `curl | bash` stays
# interactive; only when there is no terminal at all do we require --yes.
if [ -r /dev/tty ]; then TTY_IN=/dev/tty; else TTY_IN=; fi
no_tty() { die "no terminal for prompts; re-run with --yes for defaults, or download install.sh and run it directly"; }

ask() { # ask "Question" "default" -> echoes answer
  local q="$1" def="${2:-}" ans=""
  if [ "$ASSUME_YES" = 1 ]; then printf '%s' "$def"; return; fi
  [ -n "$TTY_IN" ] || no_tty
  if [ -n "$def" ]; then read -r -p "$q [$def]: " ans <"$TTY_IN" || true; else read -r -p "$q: " ans <"$TTY_IN" || true; fi
  printf '%s' "${ans:-$def}"
}
ask_secret() { # ask_secret "Question" -> echoes secret (no echo)
  local q="$1" ans=""
  if [ "$ASSUME_YES" = 1 ]; then printf ''; return; fi
  [ -n "$TTY_IN" ] || no_tty
  read -r -s -p "$q: " ans <"$TTY_IN" || true; echo >&2
  printf '%s' "$ans"
}
yesno() { # yesno "Question" "y|n" -> returns 0 for yes
  local q="$1" def="${2:-y}" ans
  if [ "$ASSUME_YES" = 1 ]; then [ "$def" = y ]; return; fi
  ans=$(ask "$q (y/n)" "$def")
  case "${ans,,}" in y|yes) return 0;; *) return 1;; esac
}

gen_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
gen_pass()  { head -c 18 /dev/urandom | base64 | tr -d '/+=' | cut -c1-24; }
have()      { command -v "$1" >/dev/null 2>&1; }

# --------------------------------------------------------------------------- #
# Preflight
# --------------------------------------------------------------------------- #
require_root() { [ "$(id -u)" = 0 ] || die "run as root (sudo)"; }
require_systemd() { have systemctl || die "systemd is required for the binary install; use --docker or install manually"; }

pkg_install() {
  if   have apt-get; then apt-get update -y >/dev/null && DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" >/dev/null
  elif have dnf;     then dnf install -y "$@" >/dev/null
  elif have yum;     then yum install -y "$@" >/dev/null
  elif have pacman;  then pacman -Sy --noconfirm "$@" >/dev/null
  elif have apk;     then apk add --no-cache "$@" >/dev/null
  else warn "unknown package manager; ensure these are installed: $*"; fi
}

ensure_deps() {
  local need=()
  have curl || need+=(curl); have jq || need+=(jq); have tar || need+=(tar); have openssl || need+=(openssl)
  [ "${#need[@]}" -gt 0 ] && { log "installing dependencies: ${need[*]}"; pkg_install "${need[@]}"; }
}

arch_tag() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64;;
    aarch64|arm64) echo arm64;;
    armv7l|armv6l|arm) echo arm;;
    riscv64) echo riscv64;;
    *) die "unsupported arch $(uname -m); set PANEL_REPO asset manually";;
  esac
}

download() { # download <repo> <asset> <out>
  local repo="$1" asset="$2" out="$3"
  local url="https://github.com/$repo/releases/latest/download/$asset"
  log "downloading $asset from $repo"
  curl -fsSL "$url" -o "$out" || die "download failed: $url (override with the repo's release asset or build from source)"
  chmod +x "$out"
}

ensure_user() { id "$SVC_USER" >/dev/null 2>&1 || useradd --system --home /var/lib/wings --shell /usr/sbin/nologin "$SVC_USER" 2>/dev/null || useradd --system "$SVC_USER"; }

# WAN interface for NAT (default route egress)
wan_iface() { ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit}}'; }

open_ports() { # open_ports "port/proto" ...
  local p
  for p in "$@"; do
    local port="${p%/*}" proto="${p#*/}"
    if have ufw && ufw status 2>/dev/null | grep -q active; then ufw allow "$port/$proto" >/dev/null 2>&1 || true
    elif have firewall-cmd && firewall-cmd --state >/dev/null 2>&1; then firewall-cmd --permanent --add-port="$port/$proto" >/dev/null 2>&1 || true; firewall-cmd --reload >/dev/null 2>&1 || true
    fi
  done
}

# --------------------------------------------------------------------------- #
# panel_cli: run the panel CLI (binary or docker exec)
# --------------------------------------------------------------------------- #
panel_cli() {
  if [ "$MODE" = docker ]; then docker exec -e WINGS_PANEL_CONFIG=/etc/wings/panel/config.toml "$PANEL_SVC" /app/wingsv-panel "$@"
  else WINGS_PANEL_CONFIG="$PANEL_CFG" "$PANEL_BIN" "$@"; fi
}

# --------------------------------------------------------------------------- #
# Uninstall
# --------------------------------------------------------------------------- #
do_uninstall() {
  log "uninstalling"
  for svc in "$PANEL_SVC" "$VKTP_SVC"; do
    systemctl disable --now "$svc" 2>/dev/null || true
    rm -f "/etc/systemd/system/$svc.service"
  done
  systemctl daemon-reload 2>/dev/null || true
  [ "$MODE" = docker ] && { docker rm -f "$PANEL_SVC" 2>/dev/null || true; }
  rm -f "$PANEL_BIN" "$VKTP_BIN"
  if yesno "remove config and data ($PANEL_CFG_DIR, $VKTP_CFG_DIR, $PANEL_DATA_DIR)?" n; then
    rm -rf "$PANEL_CFG_DIR" "$VKTP_CFG_DIR" "$PANEL_DATA_DIR"
  fi
  ok "uninstalled"
}

# --------------------------------------------------------------------------- #
# Panel install
# --------------------------------------------------------------------------- #
PUBLIC_BASE_URL=""
TLS_CERT=""; TLS_KEY=""; TLS_SELF_SIGNED=false
ADMIN_USER=""; ADMIN_PASS=""; CA_PIN=""
NEED_LE_PORT80=0

configure_cert() {
  echo "Certificate:"
  echo "  1) Let's Encrypt (acme.sh, HTTP-01 standalone) - needs a domain and free :80"
  echo "  2) Existing certificate (provide cert + key paths)"
  echo "  3) Self-signed (own CA + SPKI pin) - domain or bare IP, no external CA needed"
  local choice; choice=$(ask "choose 1/2/3" 3)
  case "$choice" in
    1)
      local domain; domain=$(ask "domain (A record must point here)")
      [ -n "$domain" ] || die "domain required for Let's Encrypt"
      install_acme "$domain"
      TLS_CERT="$CA_DIR/fullchain.pem"; TLS_KEY="$CA_DIR/key.pem"
      PUBLIC_BASE_URL="https://$domain$(port_suffix)"
      NEED_LE_PORT80=1
      ;;
    2)
      TLS_CERT=$(ask "path to certificate PEM (fullchain)")
      TLS_KEY=$(ask "path to private key PEM")
      [ -f "$TLS_CERT" ] && [ -f "$TLS_KEY" ] || die "cert/key not found"
      local domain; domain=$(ask "public domain/host for the panel URL")
      PUBLIC_BASE_URL="https://$domain$(port_suffix)"
      ;;
    *)
      TLS_SELF_SIGNED=true
      local host; host=$(ask "public domain, hostname or IP of this server")
      [ -n "$host" ] || die "host required"
      PUBLIC_BASE_URL="https://$host$(port_suffix)"
      ;;
  esac
}

port_suffix() { [ "$PANEL_PORT" = 443 ] && printf '' || printf ':%s' "$PANEL_PORT"; }

install_acme() { # install_acme <domain>
  local domain="$1"
  have "$HOME/.acme.sh/acme.sh" || { log "installing acme.sh"; curl -fsSL https://get.acme.sh | sh -s email="admin@$domain" >/dev/null; }
  local acme="$HOME/.acme.sh/acme.sh"
  "$acme" --set-default-ca --server letsencrypt >/dev/null 2>&1 || true
  open_ports "80/tcp"
  log "issuing LE certificate for $domain (binds :80 briefly)"
  "$acme" --issue --standalone -d "$domain" >/dev/null || die "acme.sh issue failed (is :80 reachable, DNS correct?)"
  mkdir -p "$CA_DIR"
  "$acme" --install-cert -d "$domain" \
    --key-file "$CA_DIR/key.pem" --fullchain-file "$CA_DIR/fullchain.pem" \
    --reloadcmd "systemctl restart $PANEL_SVC" >/dev/null
}

install_panel() {
  mkdir -p "$PANEL_CFG_DIR" "$PANEL_DATA_DIR" "$CA_DIR"

  PANEL_PORT=$(ask "panel HTTPS port" "$PANEL_PORT")
  [ "$PANEL_PORT" = 80 ] && die "port 80 is reserved (ACME / redirect); pick another"

  configure_cert

  ADMIN_USER=$(ask "bootstrap admin username" admin)
  ADMIN_PASS=$(ask_secret "bootstrap admin password (blank = generate)")
  [ -n "$ADMIN_PASS" ] || { ADMIN_PASS=$(gen_pass); warn "generated admin password: $ADMIN_PASS"; }

  if [ "$MODE" = bin ]; then download "$PANEL_REPO" "wingsv-panel-linux-$(arch_tag)" "$PANEL_BIN"; fi

  write_panel_config
  ensure_user
  chown -R "$SVC_USER":"$SVC_USER" "$PANEL_CFG_DIR" "$PANEL_DATA_DIR"
  chmod 600 "$PANEL_CFG"

  if [ "$MODE" = docker ]; then start_panel_docker; else start_panel_bin; fi

  # self-signed: initialise the CA (leaf is served from it) and capture the pin
  if [ "$TLS_SELF_SIGNED" = true ]; then
    panel_cli ca init >/dev/null 2>&1 || true
    CA_PIN=$(panel_cli ca show-pin 2>/dev/null || true)
  fi
  open_ports "$PANEL_PORT/tcp"
  ok "panel installed ($PUBLIC_BASE_URL)"
}

write_panel_config() {
  local tls_lines=""
  if [ -n "$TLS_CERT" ]; then tls_lines="TLS_CERT = \"$TLS_CERT\"
TLS_KEY = \"$TLS_KEY\""; elif [ "$TLS_SELF_SIGNED" = true ]; then tls_lines="TLS_SELF_SIGNED = \"true\""; fi
  cat > "$PANEL_CFG" <<EOF
# Generated by install.sh
LISTEN_ADDR = ":$PANEL_PORT"
PUBLIC_BASE_URL = "$PUBLIC_BASE_URL"
DB_PATH = "$PANEL_DB"
DB_KIND = "sqlite"
CA_DIR = "$CA_DIR"
PROVISIONING_LISTEN = ":$PROV_PORT"
$tls_lines
BOOTSTRAP_ADMIN_USERNAME = "$ADMIN_USER"
BOOTSTRAP_ADMIN_PASSWORD = "$ADMIN_PASS"
SESSION_SECURE = "true"
EOF
}

start_panel_bin() {
  cat > "/etc/systemd/system/$PANEL_SVC.service" <<EOF
[Unit]
Description=WINGS V panel
After=network-online.target
Wants=network-online.target

[Service]
User=$SVC_USER
Environment=WINGS_PANEL_CONFIG=$PANEL_CFG
ExecStart=$PANEL_BIN serve
Restart=always
RestartSec=3
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "$PANEL_SVC"
}

start_panel_docker() {
  have docker || die "--docker requires docker installed"
  docker rm -f "$PANEL_SVC" >/dev/null 2>&1 || true
  docker run -d --name "$PANEL_SVC" --restart always \
    -p "$PANEL_PORT:$PANEL_PORT" -p "$PROV_PORT:$PROV_PORT" \
    -v "$PANEL_CFG_DIR:/etc/wings/panel" -v "$PANEL_DATA_DIR:$PANEL_DATA_DIR" \
    -e WINGS_PANEL_CONFIG=/etc/wings/panel/config.toml \
    "$PANEL_IMAGE" >/dev/null
  sleep 2
}

# --------------------------------------------------------------------------- #
# 3x-ui detection + wiring (must run before the vktp node so the wg backend is
# known at node-creation time)
# --------------------------------------------------------------------------- #
XUI_WIRED=0; XUI_NODE_ID=""
detect_xui() { systemctl status x-ui >/dev/null 2>&1 || [ -f /etc/x-ui/x-ui.db ] || have x-ui || docker ps --format '{{.Names}}' 2>/dev/null | grep -qiE '3x-ui|x-ui'; }

wire_xui() {
  detect_xui || { log "no local 3x-ui detected; skipping"; return; }
  yesno "local 3x-ui detected - wire it to the panel for profile creation?" y || return
  local token; token=$(gen_token)
  XUI_NODE_ID=$(panel_cli node add --kind xui --name "local-3x-ui" --grpc-endpoint "127.0.0.1:$XUI_GRPC_PORT" --grpc-token "$token" | jq -r .node_id)
  [ -n "$XUI_NODE_ID" ] && [ "$XUI_NODE_ID" != null ] || die "failed to register 3x-ui node"
  # enable the 3x-ui management gRPC + register the token, then restart it
  local panel_grpc; panel_grpc="127.0.0.1:$PROV_PORT"
  if have x-ui; then
    x-ui grpc-connect "$panel_grpc" "$token" "$XUI_NODE_ID" "0.0.0.0:$XUI_GRPC_PORT" >/dev/null || warn "x-ui grpc-connect failed"
    systemctl restart x-ui 2>/dev/null || true
  else
    warn "3x-ui runs in docker; run inside it: x-ui grpc-connect $panel_grpc $token $XUI_NODE_ID 0.0.0.0:$XUI_GRPC_PORT && restart the container"
  fi
  XUI_WIRED=1
  ok "3x-ui wired (node $XUI_NODE_ID)"
}

# --------------------------------------------------------------------------- #
# Local vk-turn-proxy node
# --------------------------------------------------------------------------- #
VKTP_INSTALLED=0
install_vktp() {
  yesno "install a local vk-turn-proxy relay node?" y || { log "skipping local vk-turn-proxy node"; return; }
  local name; name=$(ask "node name (optional)" "$(hostname)-vktp")

  # The vk-turn-proxy node always runs as a host binary (it needs host wg/NAT),
  # even under --docker.
  download "$VKTP_REPO" "server-linux-$(arch_tag)" "$VKTP_BIN"

  # wg backend: 3x-ui if wired, else the node's own wg
  local wg_backend=own xui_args=()
  if [ "$XUI_WIRED" = 1 ]; then wg_backend=xui; xui_args=(--xui-node-id "$XUI_NODE_ID"); fi

  local out node_id grpc_token panel_token
  out=$(panel_cli node add --kind vktp --name "$name" --grpc-endpoint "127.0.0.1:$VKTP_GRPC_PORT" --wg-backend "$wg_backend" "${xui_args[@]}")
  node_id=$(echo "$out" | jq -r .node_id); grpc_token=$(echo "$out" | jq -r .grpc_token); panel_token=$(gen_token)
  [ -n "$node_id" ] && [ "$node_id" != null ] || die "failed to register vk-turn-proxy node"

  # wg capability probe (netlink interface create needs CAP_NET_ADMIN + module)
  local wg_apply=true
  modprobe wireguard 2>/dev/null || true
  if ! ip link add wgprobe0 type wireguard 2>/dev/null; then wg_apply=false; warn "cannot create a WireGuard interface here (no NET_ADMIN / module); wg minting will be in-memory only"; else ip link del wgprobe0 2>/dev/null || true; fi

  local panel_tls="panel-insecure = true"
  [ -n "$CA_PIN" ] && panel_tls="panel-ca-pin = \"$CA_PIN\""

  mkdir -p "$VKTP_CFG_DIR"
  cat > "$VKTP_CFG" <<EOF
# Generated by install.sh
listen = "0.0.0.0:$VKTP_DTLS_PORT"
udp-connect = "127.0.0.1:$VKTP_WG_PORT"
grpc-listen = "127.0.0.1:$VKTP_GRPC_PORT"
grpc-token = "$grpc_token"
panel-grpc = "127.0.0.1:$PROV_PORT"
panel-token = "$panel_token"
$panel_tls
node-id = "$node_id"
wg-interface = "wg-wingsv"
wg-tunnel-cidr = "$VKTP_WG_CIDR"
wg-address = "$VKTP_WG_ADDR"
wg-listen-port = $VKTP_WG_PORT
wg-key-file = "$VKTP_KEY_FILE"
wg-apply = $wg_apply
EOF
  ensure_user
  chown -R "$SVC_USER":"$SVC_USER" "$VKTP_CFG_DIR"
  chmod 600 "$VKTP_CFG"

  [ "$wg_apply" = true ] && setup_host_networking
  start_vktp_bin
  open_ports "$VKTP_DTLS_PORT/udp" "$VKTP_WG_PORT/udp"
  VKTP_INSTALLED=1
  ok "vk-turn-proxy node installed (node $node_id)"
}

setup_host_networking() {
  log "enabling IP forwarding + NAT for $VKTP_WG_CIDR"
  cat > /etc/sysctl.d/99-wings-vktp.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
  sysctl --system >/dev/null 2>&1 || true
  local wan; wan=$(wan_iface)
  [ -n "$wan" ] || { warn "could not detect WAN interface; add NAT manually"; return; }
  if have nft; then
    nft list table inet wings >/dev/null 2>&1 || nft add table inet wings
    nft 'add chain inet wings postrouting { type nat hook postrouting priority 100 ; }' 2>/dev/null || true
    nft add rule inet wings postrouting ip saddr "$VKTP_WG_CIDR" oifname "$wan" masquerade 2>/dev/null || true
    have nft && nft list ruleset > /etc/nftables.conf 2>/dev/null || true
  elif have iptables; then
    iptables -t nat -C POSTROUTING -s "$VKTP_WG_CIDR" -o "$wan" -j MASQUERADE 2>/dev/null || \
      iptables -t nat -A POSTROUTING -s "$VKTP_WG_CIDR" -o "$wan" -j MASQUERADE
    have netfilter-persistent && netfilter-persistent save >/dev/null 2>&1 || true
  else
    warn "no nft/iptables; set up MASQUERADE from $VKTP_WG_CIDR out $wan manually"
  fi
}

start_vktp_bin() {
  cat > "/etc/systemd/system/$VKTP_SVC.service" <<EOF
[Unit]
Description=WINGS V vk-turn-proxy node
After=network-online.target $PANEL_SVC.service
Wants=network-online.target

[Service]
User=$SVC_USER
Environment=WINGS_VKTP_CONFIG=$VKTP_CFG
ExecStart=$VKTP_BIN
Restart=always
RestartSec=3
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "$VKTP_SVC"
}

# --------------------------------------------------------------------------- #
# Summary
# --------------------------------------------------------------------------- #
summary() {
  echo
  log "Done."
  echo "  Panel:        $PUBLIC_BASE_URL"
  echo "  Admin:        $ADMIN_USER / $ADMIN_PASS"
  [ -n "$CA_PIN" ]     && echo "  CA SPKI pin:  $CA_PIN  (embedded in app enrollment links)"
  [ "$VKTP_INSTALLED" = 1 ] && echo "  vk-turn-proxy: local node up (UDP $VKTP_DTLS_PORT)"
  [ "$XUI_WIRED" = 1 ]      && echo "  3x-ui:        wired (node $XUI_NODE_ID)"
  echo
  echo "  Next: open the panel, sign in, add clients. Register more nodes with:"
  echo "        $PANEL_BIN connect <node-id>"
}

# --------------------------------------------------------------------------- #
main() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --docker) MODE=docker;;
      --yes|-y) ASSUME_YES=1;;
      --uninstall) DO_UNINSTALL=1;;
      -h|--help) usage; exit 0;;
      *) die "unknown flag $1 (see --help)";;
    esac; shift
  done

  require_root
  [ "$MODE" = bin ] && require_systemd
  ensure_deps

  if [ "$DO_UNINSTALL" = 1 ]; then do_uninstall; exit 0; fi

  install_panel
  wire_xui
  install_vktp
  summary
}

main "$@"
