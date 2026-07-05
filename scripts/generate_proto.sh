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

rm -f "$ROOT_DIR/wingsv.pb.go" "$ROOT_DIR/guardian.pb.go"
rm -f "$WINGSV_PKG_DIR/wingsv.pb.go" "$GUARDIAN_PKG_DIR/guardian.pb.go"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$TMP_OUT" \
  --go_opt=paths=source_relative \
  "$PROTO_DIR/wingsv.proto" \
  "$PROTO_DIR/guardian.proto"

mv "$ROOT_DIR/wingsv.pb.go" "$WINGSV_PKG_DIR/wingsv.pb.go"
mv "$ROOT_DIR/guardian.pb.go" "$GUARDIAN_PKG_DIR/guardian.pb.go"

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
