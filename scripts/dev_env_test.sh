#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELPER="$ROOT_DIR/scripts/lib/dev_env.sh"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-dev-env-test.XXXXXX")"
ENV_FILE="$TMP_DIR/dev.env"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"$ENV_FILE" <<'ENV'
# Local Charge development configuration
PUBLIC_BASE_URL=http://127.0.0.1:3000
WXPUSHER_APP_TOKEN=file-token
TRUSTED_PROXY_HEADER=none
ENV

if [[ ! -f "$HELPER" ]]; then
  echo "dev env helper is missing"
  exit 1
fi

# shellcheck source=scripts/lib/dev_env.sh
source "$HELPER"

WXPUSHER_APP_TOKEN=existing-token
export WXPUSHER_APP_TOKEN
load_dev_env_file "$ENV_FILE"

if [[ "${PUBLIC_BASE_URL:-}" != "http://127.0.0.1:3000" ]]; then
  echo "dev env should load PUBLIC_BASE_URL from file"
  exit 1
fi

if [[ "${WXPUSHER_APP_TOKEN:-}" != "existing-token" ]]; then
  echo "dev env file should not override existing WXPUSHER_APP_TOKEN"
  exit 1
fi

if [[ "${TRUSTED_PROXY_HEADER:-}" != "none" ]]; then
  echo "dev env should load optional proxy settings"
  exit 1
fi
