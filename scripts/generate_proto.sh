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
