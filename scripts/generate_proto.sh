#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="$ROOT_DIR/external/wingsv-proto"
OUT_DIR="$ROOT_DIR/internal/gen"
TMP_OUT="$ROOT_DIR"
WINGSV_PKG_DIR="$OUT_DIR/wingsvpb"
GUARDIAN_PKG_DIR="$OUT_DIR/guardianpb"

mkdir -p "$WINGSV_PKG_DIR" "$GUARDIAN_PKG_DIR"
PATH="$PATH:$(go env GOPATH)/bin"

# The Relay management API proto is owned by vk-turn-proxy (proto/control.proto).
# Pull JUST that file over git - a blobless sparse checkout, not a full clone and
# not a submodule - so the panel never hand-maintains a drifting copy. The synced
# file lands at external/wingsv-proto/control.proto and is committed; re-run this
# script to update it. Override the source with VKTP_PROTO_REPO / VKTP_PROTO_REF.
VKTP_PROTO_REPO="${VKTP_PROTO_REPO:-https://github.com/WINGS-N/vk-turn-proxy.git}"
VKTP_PROTO_REF="${VKTP_PROTO_REF:-main}"
# The federation head's panel API proto is owned by wingsvpn-federation. Same
# arrangement as control.proto above, but the sync only runs when FED_PROTO_REPO
# is set: that repository is not published yet, so the committed copy is the
# source of truth until it is, and a clone failure must not look like a no-op.
sync_federation_proto() {
  local repo="${FED_PROTO_REPO:-}"
  if [ -z "$repo" ]; then
    return 0
  fi
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  git clone --quiet --depth 1 --filter=blob:none --sparse \
    --branch "${FED_PROTO_REF:-main}" "$repo" "$tmp"
  git -C "$tmp" sparse-checkout set proto >/dev/null
  cp "$tmp/proto/headpanel.proto" "$PROTO_DIR/headpanel.proto"
  cp "$tmp/proto/intake.proto" "$PROTO_DIR/intake.proto"
  cp "$tmp/proto/federation.proto" "$PROTO_DIR/federation.proto"
}

sync_vktp_control_proto() {
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  git clone --quiet --depth 1 --filter=blob:none --sparse \
    --branch "$VKTP_PROTO_REF" "$VKTP_PROTO_REPO" "$tmp"
  git -C "$tmp" sparse-checkout set proto >/dev/null
  cp "$tmp/proto/control.proto" "$PROTO_DIR/control.proto"
}
sync_vktp_control_proto
sync_federation_proto

rm -f "$ROOT_DIR/wingsv.pb.go" "$ROOT_DIR/guardian.pb.go" "$ROOT_DIR/guardian_grpc.pb.go"
rm -f "$WINGSV_PKG_DIR/wingsv.pb.go" "$GUARDIAN_PKG_DIR/guardian.pb.go" "$GUARDIAN_PKG_DIR/guardian_grpc.pb.go"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$TMP_OUT" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$TMP_OUT" \
  --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR/wingsv.proto" \
  "$PROTO_DIR/guardian.proto"

mv "$ROOT_DIR/wingsv.pb.go" "$WINGSV_PKG_DIR/wingsv.pb.go"
mv "$ROOT_DIR/guardian.pb.go" "$GUARDIAN_PKG_DIR/guardian.pb.go"
mv "$ROOT_DIR/guardian_grpc.pb.go" "$GUARDIAN_PKG_DIR/guardian_grpc.pb.go"

PROVISIONING_PKG_DIR="$OUT_DIR/provisioningpb"
mkdir -p "$PROVISIONING_PKG_DIR"
rm -f "$PROVISIONING_PKG_DIR/provisioning.pb.go" "$PROVISIONING_PKG_DIR/provisioning_grpc.pb.go"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module=v.wingsnet.org \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module=v.wingsnet.org \
  "$PROTO_DIR/provisioning.proto"

RELAY_PKG_DIR="$OUT_DIR/relaypb"
mkdir -p "$RELAY_PKG_DIR"
rm -f "$RELAY_PKG_DIR"/*.pb.go

# control.proto declares go_package=controlpb (vk-turn-proxy's own package); the M
# override maps it into the panel's relaypb so the generated Go lands there.
protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module=v.wingsnet.org \
  --go_opt=Mcontrol.proto=v.wingsnet.org/internal/gen/relaypb \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module=v.wingsnet.org \
  --go-grpc_opt=Mcontrol.proto=v.wingsnet.org/internal/gen/relaypb \
  "$PROTO_DIR/control.proto"

# control.proto pins its Go package name to controlpb (vk-turn-proxy's own);
# rename it to relaypb so it matches the panel's import path and directory.
sed -i 's/^package controlpb$/package relaypb/' "$RELAY_PKG_DIR"/*.go

XUI_PKG_DIR="$OUT_DIR/xuipb"
mkdir -p "$XUI_PKG_DIR"
rm -f "$XUI_PKG_DIR/xui.pb.go" "$XUI_PKG_DIR/xui_grpc.pb.go"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module=v.wingsnet.org \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module=v.wingsnet.org \
  "$PROTO_DIR/xui.proto"

HEAD_PKG_DIR="$OUT_DIR/headpb"
mkdir -p "$HEAD_PKG_DIR"
rm -f "$HEAD_PKG_DIR"/*.pb.go

# headpanel.proto declares go_package=wingsnet.org/federation/gen/headpb (the
# federation's own path); the M override maps it into the panel's tree.
protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module=v.wingsnet.org \
  --go_opt=Mheadpanel.proto=v.wingsnet.org/internal/gen/headpb \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module=v.wingsnet.org \
  --go-grpc_opt=Mheadpanel.proto=v.wingsnet.org/internal/gen/headpb \
  "$PROTO_DIR/headpanel.proto"

# Приём расписок. Отдельный сервис у головы, отдельные стабы и тут: панель ходит
# в него тем же секретом, что и в панельный API
protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module=v.wingsnet.org \
  --go_opt=Mintake.proto=v.wingsnet.org/internal/gen/intakepb \
  --go_opt=Mfederation.proto=v.wingsnet.org/internal/gen/fedpb \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module=v.wingsnet.org \
  --go-grpc_opt=Mintake.proto=v.wingsnet.org/internal/gen/intakepb \
  --go-grpc_opt=Mfederation.proto=v.wingsnet.org/internal/gen/fedpb \
  "$PROTO_DIR/intake.proto" "$PROTO_DIR/federation.proto"
