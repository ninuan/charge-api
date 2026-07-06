#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELPER="$ROOT_DIR/scripts/lib/dev_env.sh"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-dev-env-test.XXXXXX")"
ENV_FILE="$TMP_DIR/dev.env"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"$ENV_FILE" <<'ENV'
# Local YYB sidecar configuration
YYB_BASE_URL=http://127.0.0.1:8000
YYB_API_SECRET=file-secret
MOCELE_ORG=2
ENV

if [[ ! -f "$HELPER" ]]; then
  echo "dev env helper is missing"
  exit 1
fi

source "$HELPER"

YYB_API_SECRET=existing-secret
export YYB_API_SECRET
load_dev_env_file "$ENV_FILE"

if [[ "${YYB_BASE_URL:-}" != "http://127.0.0.1:8000" ]]; then
  echo "dev env should load YYB_BASE_URL from file"
  exit 1
fi

if [[ "${YYB_API_SECRET:-}" != "existing-secret" ]]; then
  echo "dev env file should not override existing YYB_API_SECRET"
  exit 1
fi

if [[ "${MOCELE_ORG:-}" != "2" ]]; then
  echo "dev env should load optional Mocele settings"
  exit 1
fi
