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
ADVANCED=0              # advanced config: expose ports / DB path / WG CIDR
LANG_SEL=ru             # ru (default) | en; overridden by the language prompt

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

Usage: install.sh [--docker] [--yes] [--advanced] [--uninstall] [--lang ru|en]

  --docker      run the panel as a container (default: standalone binary)
  --yes         accept defaults, do not prompt (non-interactive)
  --advanced    prompt for ports, DB path and WireGuard CIDR (else use defaults)
  --uninstall   stop and remove the panel + local node services
  --lang ru|en  interface language (default: ru; also LANG_SEL env)

Env overrides: PANEL_REPO, VKTP_REPO, PANEL_IMAGE, LANG_SEL.
EOF
}

# --------------------------------------------------------------------------- #
# i18n. t <key> -> localized string; tf <key> args... -> printf-formatted.
# --------------------------------------------------------------------------- #
t() {
  if [ "$LANG_SEL" = en ]; then
    case "$1" in
      q_port)          echo "panel HTTPS port";;
      lbl_current)     echo "Existing install found. Current configuration:";;
      q_update_panel)  echo "Update the panel with this config (re-download binary, restart)?";;
      q_update_vktp)   echo "Update the vk-turn-proxy node with this config (re-download binary, restart)?";;
      q_advanced)      echo "advanced configuration (ports, DB path, WireGuard CIDR)?";;
      q_prov_port)     echo "provisioning gRPC port (node -> panel)";;
      q_db_path)       echo "SQLite database path";;
      q_vktp_dtls)     echo "vk-turn-proxy data port (UDP)";;
      q_vktp_grpc)     echo "vk-turn-proxy management gRPC port (loopback)";;
      q_wg_port)       echo "WireGuard listen port (UDP)";;
      q_wg_cidr)       echo "WireGuard tunnel CIDR";;
      q_cert_choose)   echo "choose 1/2/3";;
      cert_header)     echo "Certificate:";;
      cert_1)          echo "  1) Let's Encrypt (acme.sh, HTTP-01 standalone) - needs a domain and free :80";;
      cert_2)          echo "  2) Existing certificate (provide cert + key paths)";;
      cert_3)          echo "  3) Self-signed (own CA + SPKI pin) - domain or bare IP, no external CA needed";;
      q_le_domain)     echo "domain (A record must point here)";;
      q_cert_path)     echo "path to certificate PEM (fullchain)";;
      q_key_path)      echo "path to private key PEM";;
      q_url_host)      echo "public domain/host for the panel URL";;
      q_host)          echo "enter IP or domain of this server";;
      q_admin_user)    echo "bootstrap admin username";;
      q_admin_pass)    echo "bootstrap admin password (blank = admin)";;
      q_node_name)     echo "node name (optional)";;
      q_install_vktp)  echo "install a local vk-turn-proxy relay node?";;
      q_wire_xui)      echo "local 3x-ui detected - wire it to the panel for profile creation?";;
      q_rm_data)       echo "remove config and data (%s, %s, %s)?";;
      err_no_tty)      echo "no terminal for prompts; re-run with --yes for defaults, or download install.sh and run it directly";;
      err_root)        echo "run as root (sudo)";;
      err_systemd)     echo "systemd is required for the binary install; use --docker or install manually";;
      warn_pkg)        echo "unknown package manager; ensure these are installed: %s";;
      log_deps)        echo "Installing dependencies";;
      log_wg_module)   echo "Installing the wireguard kernel module";;
      log_reinstall)   echo "Existing install found, stopping the service to update";;
      err_arch)        echo "unsupported arch %s; set the release asset manually";;
      log_download)    echo "Downloading %s from %s";;
      err_download)    echo "download failed: %s (override with the repo's release asset or build from source)";;
      log_uninstall)   echo "Uninstalling";;
      ok_uninstalled)  echo "Uninstall complete";;
      err_le_domain)   echo "domain required for Let's Encrypt";;
      err_certkey)     echo "cert/key not found";;
      err_host)        echo "host required";;
      log_acme)        echo "Installing acme.sh";;
      log_le_issue)    echo "Issuing LE certificate for %s (binds :80 briefly)";;
      err_acme)        echo "acme.sh issue failed (is :80 reachable, DNS correct?)";;
      warn_gen_pass)   echo "generated admin password: %s";;
      ok_panel)        echo "panel installed (%s)";;
      err_docker)      echo "--docker requires docker installed";;
      log_no_xui)      echo "No local 3x-ui detected; skipping";;
      err_xui_reg)     echo "failed to register 3x-ui node";;
      warn_xui_conn)   echo "x-ui grpc-connect failed";;
      warn_xui_docker) echo "3x-ui runs in docker; run inside it: x-ui grpc-connect %s %s %s 0.0.0.0:%s && restart the container";;
      ok_xui)          echo "3x-ui wired (node %s)";;
      log_skip_vktp)   echo "Skipping local vk-turn-proxy node";;
      err_vktp_reg)    echo "failed to register vk-turn-proxy node";;
      warn_wg)         echo "cannot create a WireGuard interface here (no NET_ADMIN / module); wg minting will be in-memory only";;
      log_nat)         echo "Enabling IP forwarding + NAT for %s";;
      warn_wan)        echo "could not detect WAN interface; add NAT manually";;
      warn_no_fw)      echo "no nft/iptables; set up MASQUERADE from %s out %s manually";;
      ok_vktp)         echo "vk-turn-proxy node installed (node %s)";;
      s_done)          echo "Done.";;
      s_panel)         echo "  Panel:         %s";;
      s_admin)         echo "  Admin:         %s / %s";;
      s_pin)           echo "  CA SPKI pin:   %s  (embedded in app enrollment links)";;
      s_vktp)          echo "  vk-turn-proxy: local node up (UDP %s)";;
      s_xui)           echo "  3x-ui:         wired (node %s)";;
      s_next)          echo "  Next: open the panel, sign in, add clients. Register more nodes with:";;
      err_flag)        echo "unknown flag %s (see --help)";;
      *)               echo "$1";;
    esac
  else
    case "$1" in
      q_port)          echo "порт HTTPS панели";;
      lbl_current)     echo "Найдена установка. Текущая конфигурация:";;
      q_update_panel)  echo "Обновить панель с этой конфигурацией (перекачать бинарь, перезапуск)?";;
      q_update_vktp)   echo "Обновить vk-turn-proxy ноду с этой конфигурацией (перекачать бинарь, перезапуск)?";;
      q_advanced)      echo "расширенная конфигурация (порты, путь к БД, WireGuard CIDR)?";;
      q_prov_port)     echo "порт provisioning gRPC (нода -> панель)";;
      q_db_path)       echo "путь к базе SQLite";;
      q_vktp_dtls)     echo "порт данных vk-turn-proxy (UDP)";;
      q_vktp_grpc)     echo "порт управления vk-turn-proxy gRPC (loopback)";;
      q_wg_port)       echo "порт WireGuard (UDP)";;
      q_wg_cidr)       echo "CIDR туннеля WireGuard";;
      q_cert_choose)   echo "выберите 1/2/3";;
      cert_header)     echo "Сертификат:";;
      cert_1)          echo "  1) Let's Encrypt (acme.sh, HTTP-01 standalone) - нужен домен и свободный :80";;
      cert_2)          echo "  2) Существующий сертификат (указать пути cert + key)";;
      cert_3)          echo "  3) Самоподписанный (свой CA + SPKI pin) - домен или голый IP, без внешнего CA";;
      q_le_domain)     echo "домен (A-запись должна указывать сюда)";;
      q_cert_path)     echo "путь к сертификату PEM (fullchain)";;
      q_key_path)      echo "путь к приватному ключу PEM";;
      q_url_host)      echo "публичный домен/хост для URL панели";;
      q_host)          echo "введите IP или домен этого сервера";;
      q_admin_user)    echo "имя администратора";;
      q_admin_pass)    echo "пароль администратора (пусто = admin)";;
      q_node_name)     echo "имя ноды (опционально)";;
      q_install_vktp)  echo "установить локальную vk-turn-proxy relay-ноду?";;
      q_wire_xui)      echo "обнаружен локальный 3x-ui - подключить его к панели для создания профилей?";;
      q_rm_data)       echo "удалить конфиги и данные (%s, %s, %s)?";;
      err_no_tty)      echo "нет терминала для ввода; перезапустите с --yes для дефолтов, либо скачайте install.sh и запустите напрямую";;
      err_root)        echo "запустите от root (sudo)";;
      err_systemd)     echo "для установки бинарём нужен systemd; используйте --docker или ставьте вручную";;
      warn_pkg)        echo "неизвестный пакетный менеджер; установите вручную: %s";;
      log_deps)        echo "Установка зависимостей";;
      log_wg_module)   echo "Установка модуля ядра wireguard";;
      log_reinstall)   echo "Найдена прежняя установка, остановка сервиса для обновления";;
      err_arch)        echo "неподдерживаемая архитектура %s; задайте ассет релиза вручную";;
      log_download)    echo "Скачивание %s из %s";;
      err_download)    echo "не удалось скачать: %s (укажите ассет релиза или соберите из исходников)";;
      log_uninstall)   echo "Удаление";;
      ok_uninstalled)  echo "Удаление завершено";;
      err_le_domain)   echo "для Let's Encrypt нужен домен";;
      err_certkey)     echo "cert/key не найдены";;
      err_host)        echo "нужен хост";;
      log_acme)        echo "Установка acme.sh";;
      log_le_issue)    echo "Выпуск LE-сертификата для %s (ненадолго занимает :80)";;
      err_acme)        echo "acme.sh issue не удался (доступен ли :80, верный ли DNS?)";;
      warn_gen_pass)   echo "сгенерирован пароль администратора: %s";;
      ok_panel)        echo "панель установлена (%s)";;
      err_docker)      echo "--docker требует установленного docker";;
      log_no_xui)      echo "Локальный 3x-ui не обнаружен; пропуск";;
      err_xui_reg)     echo "не удалось зарегистрировать ноду 3x-ui";;
      warn_xui_conn)   echo "x-ui grpc-connect не удался";;
      warn_xui_docker) echo "3x-ui в docker; выполните внутри: x-ui grpc-connect %s %s %s 0.0.0.0:%s и перезапустите контейнер";;
      ok_xui)          echo "3x-ui подключён (нода %s)";;
      log_skip_vktp)   echo "Пропуск локальной vk-turn-proxy ноды";;
      err_vktp_reg)    echo "не удалось зарегистрировать vk-turn-proxy ноду";;
      warn_wg)         echo "здесь нельзя создать WireGuard-интерфейс (нет NET_ADMIN / модуля); wg будет только в памяти";;
      log_nat)         echo "Включение IP forwarding + NAT для %s";;
      warn_wan)        echo "не удалось определить WAN-интерфейс; добавьте NAT вручную";;
      warn_no_fw)      echo "нет nft/iptables; настройте MASQUERADE из %s через %s вручную";;
      ok_vktp)         echo "vk-turn-proxy нода установлена (нода %s)";;
      s_done)          echo "Готово.";;
      s_panel)         echo "  Панель:        %s";;
      s_admin)         echo "  Админ:         %s / %s";;
      s_pin)           echo "  CA SPKI pin:   %s  (встраивается в enrollment-ссылки приложения)";;
      s_vktp)          echo "  vk-turn-proxy: локальная нода поднята (UDP %s)";;
      s_xui)           echo "  3x-ui:         подключён (нода %s)";;
      s_next)          echo "  Далее: откройте панель, войдите, добавляйте клиентов. Регистрация новых нод:";;
      err_flag)        echo "неизвестный флаг %s (см. --help)";;
      *)               echo "$1";;
    esac
  fi
}
tf() { local k="$1"; shift; printf "$(t "$k")" "$@"; }

# --------------------------------------------------------------------------- #
# Prompt helpers (respect --yes)
# --------------------------------------------------------------------------- #
# When the installer is piped (curl ... | bash) its stdin IS the script, so a
# plain `read` hits EOF and every prompt silently takes its default. Read prompts
# from the controlling terminal instead so `curl | bash` stays interactive; only
# when there is no terminal at all do we require --yes.
if [ -r /dev/tty ]; then TTY_IN=/dev/tty; else TTY_IN=; fi
no_tty() { die "$(t err_no_tty)"; }

# Some terminals wrap pasted text in bracketed-paste markers (ESC[200~ ... ESC[201~).
# A bare `read` cannot consume them and shows stray `^` characters, so pasting a
# password / cert path / IP fails. Turn the mode off on the prompt terminal. The
# group redirect swallows the "cannot open /dev/tty" message on hosts where the
# node exists but has no controlling terminal.
[ -n "$TTY_IN" ] && { printf '\033[?2004l' >"$TTY_IN"; } 2>/dev/null || true

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

# Ask the interface language before anything else is printed. Bilingual prompt so
# it is understandable regardless of the current default.
choose_lang() {
  if [ "$ASSUME_YES" = 1 ] || [ -z "$TTY_IN" ]; then return; fi
  local a=""
  read -r -p "Язык / Language: 1) Русский  2) English [1]: " a <"$TTY_IN" || true
  case "${a,,}" in 2|en|english|английский|англ) LANG_SEL=en;; *) LANG_SEL=ru;; esac
}

gen_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
gen_pass()  { head -c 18 /dev/urandom | base64 | tr -d '/+=' | cut -c1-24; }
have()      { command -v "$1" >/dev/null 2>&1; }
# cfg_get <key> <file> -> value of a `key = "value"` or `key = value` TOML line.
cfg_get()   { sed -n "s/^$1[[:space:]]*=[[:space:]]*\"\{0,1\}\([^\"]*\)\"\{0,1\}.*/\1/p" "$2" 2>/dev/null | head -1; }

# Best-effort detection of this server's reachable IP: public IPv4 first (what a
# client actually dials), falling back to the primary/route-source local address.
detect_ip() {
  local ip=""
  ip=$(curl -fsS4 --max-time 4 https://api.ipify.org 2>/dev/null || true)
  [ -n "$ip" ] || ip=$(curl -fsS4 --max-time 4 https://ifconfig.me 2>/dev/null || true)
  [ -n "$ip" ] || ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')
  [ -n "$ip" ] || ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  printf '%s' "$ip"
}

# --------------------------------------------------------------------------- #
# Preflight
# --------------------------------------------------------------------------- #
require_root() { [ "$(id -u)" = 0 ] || die "$(t err_root)"; }
require_systemd() { have systemctl || die "$(t err_systemd)"; }

pkg_install() {
  if   have apt-get; then apt-get update -y >/dev/null && DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" >/dev/null
  elif have dnf;     then dnf install -y "$@" >/dev/null
  elif have yum;     then yum install -y "$@" >/dev/null
  elif have pacman;  then pacman -Sy --noconfirm "$@" >/dev/null
  elif have apk;     then apk add --no-cache "$@" >/dev/null
  else warn "$(tf warn_pkg "$*")"; fi
}

ensure_deps() {
  local need=()
  have curl || need+=(curl); have jq || need+=(jq); have tar || need+=(tar); have openssl || need+=(openssl)
  # Must be an `if`, not `X && {...}`: as the function's last statement a false
  # `[ ${#need[@]} -gt 0 ]` would make ensure_deps return 1, and under
  # `set -e` that silently aborts the whole installer once all deps are present.
  if [ "${#need[@]}" -gt 0 ]; then
    log "$(t log_deps): ${need[*]}"
    pkg_install "${need[@]}"
  fi
}

arch_tag() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64;;
    aarch64|arm64) echo arm64;;
    armv7l|armv6l|arm) echo arm;;
    riscv64) echo riscv64;;
    *) die "$(tf err_arch "$(uname -m)")";;
  esac
}

download() { # download <repo> <asset> <out>
  local repo="$1" asset="$2" out="$3"
  local url="https://github.com/$repo/releases/latest/download/$asset"
  log "$(tf log_download "$asset" "$repo")"
  # Visible progress bar (not -s). --http1.1: GitHub's release-assets host serves
  # over HTTP/2 where curl intermittently hangs after the body completes (100%)
  # waiting on the stream close; HTTP/1.1 closes cleanly on Content-Length.
  curl -fL --http1.1 --progress-bar "$url" -o "$out" || die "$(tf err_download "$url")"
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
  log "$(t log_uninstall)"
  for svc in "$PANEL_SVC" "$VKTP_SVC"; do
    systemctl disable --now "$svc" 2>/dev/null || true
    rm -f "/etc/systemd/system/$svc.service"
  done
  systemctl daemon-reload 2>/dev/null || true
  [ "$MODE" = docker ] && { docker rm -f "$PANEL_SVC" 2>/dev/null || true; }
  rm -f "$PANEL_BIN" "$VKTP_BIN"
  if yesno "$(tf q_rm_data "$PANEL_CFG_DIR" "$VKTP_CFG_DIR" "$PANEL_DATA_DIR")" n; then
    rm -rf "$PANEL_CFG_DIR" "$VKTP_CFG_DIR" "$PANEL_DATA_DIR"
  fi
  ok "$(t ok_uninstalled)"
}

# --------------------------------------------------------------------------- #
# Panel install
# --------------------------------------------------------------------------- #
PUBLIC_BASE_URL=""
TLS_CERT=""; TLS_KEY=""; TLS_SELF_SIGNED=false
ADMIN_USER=""; ADMIN_PASS=""; CA_PIN=""
NEED_LE_PORT80=0

configure_cert() {
  echo "$(t cert_header)"
  echo "$(t cert_1)"
  echo "$(t cert_2)"
  echo "$(t cert_3)"
  local choice; choice=$(ask "$(t q_cert_choose)" 3)
  case "$choice" in
    1)
      local domain; domain=$(ask "$(t q_le_domain)")
      [ -n "$domain" ] || die "$(t err_le_domain)"
      install_acme "$domain"
      TLS_CERT="$CA_DIR/fullchain.pem"; TLS_KEY="$CA_DIR/key.pem"
      PUBLIC_BASE_URL="https://$domain$(port_suffix)"
      NEED_LE_PORT80=1
      ;;
    2)
      TLS_CERT=$(ask "$(t q_cert_path)")
      TLS_KEY=$(ask "$(t q_key_path)")
      [ -f "$TLS_CERT" ] && [ -f "$TLS_KEY" ] || die "$(t err_certkey)"
      local domain; domain=$(ask "$(t q_url_host)")
      PUBLIC_BASE_URL="https://$domain$(port_suffix)"
      ;;
    *)
      TLS_SELF_SIGNED=true
      local host; host=$(ask "$(t q_host)" "$(detect_ip)")
      [ -n "$host" ] || die "$(t err_host)"
      PUBLIC_BASE_URL="https://$host$(port_suffix)"
      ;;
  esac
}

port_suffix() { [ "$PANEL_PORT" = 443 ] && printf '' || printf ':%s' "$PANEL_PORT"; }

install_acme() { # install_acme <domain>
  local domain="$1"
  have "$HOME/.acme.sh/acme.sh" || { log "$(t log_acme)"; curl -fsSL https://get.acme.sh | sh -s email="admin@$domain" >/dev/null; }
  local acme="$HOME/.acme.sh/acme.sh"
  "$acme" --set-default-ca --server letsencrypt >/dev/null 2>&1 || true
  open_ports "80/tcp"
  log "$(tf log_le_issue "$domain")"
  "$acme" --issue --standalone -d "$domain" >/dev/null || die "$(t err_acme)"
  mkdir -p "$CA_DIR"
  "$acme" --install-cert -d "$domain" \
    --key-file "$CA_DIR/key.pem" --fullchain-file "$CA_DIR/fullchain.pem" \
    --reloadcmd "systemctl restart $PANEL_SVC" >/dev/null
}

update_panel() {
  PUBLIC_BASE_URL=$(cfg_get PUBLIC_BASE_URL "$PANEL_CFG")
  PANEL_PORT=$(cfg_get LISTEN_ADDR "$PANEL_CFG" | tr -d ':')
  PANEL_DB=$(cfg_get DB_PATH "$PANEL_CFG"); [ -n "$PANEL_DB" ] && PANEL_DATA_DIR=$(dirname "$PANEL_DB")
  grep -q '^TLS_SELF_SIGNED' "$PANEL_CFG" 2>/dev/null && TLS_SELF_SIGNED=true
  systemctl stop "$PANEL_SVC" 2>/dev/null || true
  if [ "$MODE" = bin ]; then download "$PANEL_REPO" "wingsv-panel-linux-$(arch_tag)" "$PANEL_BIN"; fi
  ensure_user
  chown -R "$SVC_USER":"$SVC_USER" "$PANEL_CFG_DIR" "$PANEL_DATA_DIR" 2>/dev/null || true
  if [ "$MODE" = docker ]; then start_panel_docker; else start_panel_bin; fi
  [ "$TLS_SELF_SIGNED" = true ] && CA_PIN=$(panel_cli ca show-pin 2>/dev/null || true)
  open_ports "$PANEL_PORT/tcp"
  ok "$(tf ok_panel "$PUBLIC_BASE_URL")"
}

install_panel() {
  # Existing install: show the current config and offer a quick update (just
  # re-download the binary and restart) instead of re-asking everything.
  if [ -f "$PANEL_CFG" ] && [ "$ADVANCED" = 0 ]; then
    echo "$(t lbl_current)"
    echo "  URL:  $(cfg_get PUBLIC_BASE_URL "$PANEL_CFG")"
    echo "  Port: $(cfg_get LISTEN_ADDR "$PANEL_CFG")"
    echo "  DB:   $(cfg_get DB_PATH "$PANEL_CFG")"
    if yesno "$(t q_update_panel)" y; then update_panel; return; fi
  fi

  PANEL_PORT=$(ask "$(t q_port)" "$PANEL_PORT")
  [ "$PANEL_PORT" = 80 ] && die "port 80 is reserved (ACME / redirect); pick another"

  if [ "$ADVANCED" = 1 ]; then
    PROV_PORT=$(ask "$(t q_prov_port)" "$PROV_PORT")
    PANEL_DB=$(ask "$(t q_db_path)" "$PANEL_DB")
    PANEL_DATA_DIR=$(dirname "$PANEL_DB")
  fi

  mkdir -p "$PANEL_CFG_DIR" "$PANEL_DATA_DIR" "$CA_DIR"

  configure_cert

  ADMIN_USER=$(ask "$(t q_admin_user)" admin)
  ADMIN_PASS=$(ask_secret "$(t q_admin_pass)")
  [ -n "$ADMIN_PASS" ] || ADMIN_PASS=admin

  # Re-install: a running service holds its binary open (busy executable), so
  # overwriting it with curl stalls. Stop it first; it is (re)started below.
  if systemctl is-active --quiet "$PANEL_SVC" 2>/dev/null; then log "$(t log_reinstall)"; fi
  systemctl stop "$PANEL_SVC" 2>/dev/null || true

  if [ "$MODE" = bin ]; then download "$PANEL_REPO" "wingsv-panel-linux-$(arch_tag)" "$PANEL_BIN"; fi

  write_panel_config
  ensure_user

  # Self-signed CA: for the binary install create it BEFORE the chown + start, so
  # the root-created ca.key gets chowned to the service user - otherwise the wings
  # service cannot read it and crash-loops ("open .../ca.key: permission denied").
  # Docker creates it after the container is up (there the panel runs as root
  # inside the container and reads the mounted files).
  if [ "$TLS_SELF_SIGNED" = true ] && [ "$MODE" = bin ]; then init_ca; fi

  chown -R "$SVC_USER":"$SVC_USER" "$PANEL_CFG_DIR" "$PANEL_DATA_DIR"
  chmod 600 "$PANEL_CFG"

  if [ "$MODE" = docker ]; then start_panel_docker; else start_panel_bin; fi

  if [ "$TLS_SELF_SIGNED" = true ] && [ "$MODE" = docker ]; then init_ca; fi

  open_ports "$PANEL_PORT/tcp"
  ok "$(tf ok_panel "$PUBLIC_BASE_URL")"
}

init_ca() {
  panel_cli ca init >/dev/null 2>&1 || true
  CA_PIN=$(panel_cli ca show-pin 2>/dev/null || true)
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
  # StateDirectory makes systemd create the data dir and (re)own it to the
  # service user on every start - the canonical fix for the SQLite "unable to
  # open database file" (CANTOPEN) that hits when the wings user cannot traverse
  # or write /var/lib/wings/panel. Only meaningful when the DB lives under
  # /var/lib; a custom advanced path relies on the installer's chown instead.
  local state_dir=""
  case "$PANEL_DATA_DIR" in
    /var/lib/*) state_dir="StateDirectory=${PANEL_DATA_DIR#/var/lib/}";;
  esac
  cat > "/etc/systemd/system/$PANEL_SVC.service" <<EOF
[Unit]
Description=WINGS V panel
# network.target (not network-online.target): the latter makes systemctl start
# block until a wait-online service reports every interface up, which hangs the
# installer on hosts where that never settles. The panel binds a socket and
# Restart=always retries, so it does not need to wait for full connectivity.
After=network.target

[Service]
User=$SVC_USER
$state_dir
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
  have docker || die "$(t err_docker)"
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
  detect_xui || { log "$(t log_no_xui)"; return; }
  yesno "$(t q_wire_xui)" y || return
  local token; token=$(gen_token)
  XUI_NODE_ID=$(panel_cli node add --kind xui --name "local-3x-ui" --grpc-endpoint "127.0.0.1:$XUI_GRPC_PORT" --grpc-token "$token" | jq -r .node_id)
  [ -n "$XUI_NODE_ID" ] && [ "$XUI_NODE_ID" != null ] || die "$(t err_xui_reg)"
  # enable the 3x-ui management gRPC + register the token, then restart it
  local panel_grpc; panel_grpc="127.0.0.1:$PROV_PORT"
  if have x-ui; then
    x-ui grpc-connect "$panel_grpc" "$token" "$XUI_NODE_ID" "0.0.0.0:$XUI_GRPC_PORT" >/dev/null || warn "$(t warn_xui_conn)"
    systemctl restart x-ui 2>/dev/null || true
  else
    warn "$(tf warn_xui_docker "$panel_grpc" "$token" "$XUI_NODE_ID" "$XUI_GRPC_PORT")"
  fi
  XUI_WIRED=1
  ok "$(tf ok_xui "$XUI_NODE_ID")"
}

# --------------------------------------------------------------------------- #
# Local vk-turn-proxy node
# --------------------------------------------------------------------------- #
VKTP_INSTALLED=0
update_vktp() {
  systemctl stop "$VKTP_SVC" 2>/dev/null || true
  download "$VKTP_REPO" "server-linux-$(arch_tag)" "$VKTP_BIN"
  ensure_user
  chown -R "$SVC_USER":"$SVC_USER" "$VKTP_CFG_DIR" 2>/dev/null || true
  if [ "$(cfg_get wg-apply "$VKTP_CFG")" = true ]; then
    VKTP_WG_CIDR=$(cfg_get wg-tunnel-cidr "$VKTP_CFG"); setup_host_networking
  fi
  start_vktp_bin
  open_ports "$(cfg_get listen "$VKTP_CFG" | sed 's/.*://')/udp" "$(cfg_get wg-listen-port "$VKTP_CFG")/udp"
  VKTP_INSTALLED=1
  ok "$(tf ok_vktp "$(cfg_get node-id "$VKTP_CFG")")"
}

install_vktp() {
  # Existing node: show its config and offer a quick binary update + restart.
  if [ -f "$VKTP_CFG" ] && [ "$ADVANCED" = 0 ]; then
    echo "$(t lbl_current)"
    echo "  node:   $(cfg_get node-id "$VKTP_CFG")"
    echo "  listen: $(cfg_get listen "$VKTP_CFG")"
    echo "  wg:     $(cfg_get wg-tunnel-cidr "$VKTP_CFG")"
    if yesno "$(t q_update_vktp)" y; then update_vktp; return; fi
  fi

  yesno "$(t q_install_vktp)" y || { log "$(t log_skip_vktp)"; return; }
  local name; name=$(ask "$(t q_node_name)" "$(hostname)-vktp")

  if [ "$ADVANCED" = 1 ]; then
    VKTP_DTLS_PORT=$(ask "$(t q_vktp_dtls)" "$VKTP_DTLS_PORT")
    VKTP_GRPC_PORT=$(ask "$(t q_vktp_grpc)" "$VKTP_GRPC_PORT")
    VKTP_WG_PORT=$(ask "$(t q_wg_port)" "$VKTP_WG_PORT")
    VKTP_WG_CIDR=$(ask "$(t q_wg_cidr)" "$VKTP_WG_CIDR")
  fi

  # The vk-turn-proxy node always runs as a host binary (it needs host wg/NAT),
  # even under --docker. Stop a running one first so its busy binary can be
  # overwritten without stalling.
  systemctl stop "$VKTP_SVC" 2>/dev/null || true
  download "$VKTP_REPO" "server-linux-$(arch_tag)" "$VKTP_BIN"

  # wg backend: 3x-ui if wired, else the node's own wg
  local wg_backend=own xui_args=()
  if [ "$XUI_WIRED" = 1 ]; then wg_backend=xui; xui_args=(--xui-node-id "$XUI_NODE_ID"); fi

  local out node_id grpc_token panel_token
  out=$(panel_cli node add --kind vktp --name "$name" --grpc-endpoint "127.0.0.1:$VKTP_GRPC_PORT" --wg-backend "$wg_backend" "${xui_args[@]}")
  node_id=$(echo "$out" | jq -r .node_id); grpc_token=$(echo "$out" | jq -r .grpc_token); panel_token=$(gen_token)
  [ -n "$node_id" ] && [ "$node_id" != null ] || die "$(t err_vktp_reg)"

  # wg capability probe (netlink interface create needs CAP_NET_ADMIN + module).
  # Ensure the wireguard kernel module is available first: it is built into
  # modern kernels, but on older ones the wireguard package supplies the DKMS
  # build - without it wg-apply falls back to in-memory-only minting.
  if ! modprobe wireguard 2>/dev/null && ! lsmod 2>/dev/null | grep -q '^wireguard'; then
    log "$(t log_wg_module)"
    pkg_install wireguard
    modprobe wireguard 2>/dev/null || true
  fi
  local wg_apply=true
  if ! ip link add wgprobe0 type wireguard 2>/dev/null; then wg_apply=false; warn "$(t warn_wg)"; else ip link del wgprobe0 2>/dev/null || true; fi

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
  # The node is registered on 127.0.0.1 (loopback management); the panel derives
  # the client-facing DTLS endpoint from its own PUBLIC_BASE_URL host when the
  # relay endpoint is loopback, so no VK_TURN_ENDPOINT is needed here.
  VKTP_INSTALLED=1
  ok "$(tf ok_vktp "$node_id")"
}

setup_host_networking() {
  log "$(tf log_nat "$VKTP_WG_CIDR")"
  cat > /etc/sysctl.d/99-wings-vktp.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
  sysctl --system >/dev/null 2>&1 || true
  local wan; wan=$(wan_iface)
  [ -n "$wan" ] || { warn "$(t warn_wan)"; return; }
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
    warn "$(tf warn_no_fw "$VKTP_WG_CIDR" "$wan")"
  fi
}

start_vktp_bin() {
  cat > "/etc/systemd/system/$VKTP_SVC.service" <<EOF
[Unit]
Description=WINGS V vk-turn-proxy node
# network.target only (see the panel unit re: network-online). Deliberately NOT
# ordered After the panel service: the node reaches the panel over the network
# with retries, and ordering after a crash-looping panel made systemctl start
# block here. Decoupling keeps the node start independent.
After=network.target

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
  log "$(t s_done)"
  printf '%s\n' "$(tf s_panel "$PUBLIC_BASE_URL")"
  [ -n "$ADMIN_PASS" ]      && printf '%s\n' "$(tf s_admin "$ADMIN_USER" "$ADMIN_PASS")"
  [ -n "$CA_PIN" ]          && printf '%s\n' "$(tf s_pin "$CA_PIN")"
  [ "$VKTP_INSTALLED" = 1 ] && printf '%s\n' "$(tf s_vktp "$VKTP_DTLS_PORT")"
  [ "$XUI_WIRED" = 1 ]      && printf '%s\n' "$(tf s_xui "$XUI_NODE_ID")"
  echo
  printf '%s\n' "$(t s_next)"
  echo "        $PANEL_BIN connect <node-id>"
}

# --------------------------------------------------------------------------- #
main() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --docker) MODE=docker;;
      --yes|-y) ASSUME_YES=1;;
      --advanced|-a) ADVANCED=1;;
      --uninstall) DO_UNINSTALL=1;;
      --lang) shift; [ "${1:-}" = en ] && LANG_SEL=en || LANG_SEL=ru;;
      -h|--help) usage; exit 0;;
      *) die "$(tf err_flag "$1")";;
    esac; shift
  done

  choose_lang

  require_root
  [ "$MODE" = bin ] && require_systemd
  ensure_deps

  if [ "$DO_UNINSTALL" = 1 ]; then do_uninstall; exit 0; fi

  # Offer advanced config unless already forced with --advanced.
  if [ "$ADVANCED" = 0 ]; then yesno "$(t q_advanced)" n && ADVANCED=1 || true; fi

  install_panel
  wire_xui
  install_vktp
  summary
}

main "$@"
