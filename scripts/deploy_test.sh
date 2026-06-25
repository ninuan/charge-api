#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_SCRIPT="$ROOT_DIR/scripts/deploy.sh"

help_output="$("$DEPLOY_SCRIPT" --help)"
if [[ "$help_output" != *"DEPLOY_HOST"* ]] || [[ "$help_output" != *"SKIP_CHECK"* ]]; then
  echo "deploy help should document DEPLOY_HOST and SKIP_CHECK"
  exit 1
fi

set +e
missing_host_output="$(env -u DEPLOY_HOST "$DEPLOY_SCRIPT" --dry-run 2>&1)"
missing_host_status=$?
set -e

if [[ "$missing_host_status" -eq 0 ]]; then
  echo "deploy should fail when DEPLOY_HOST is missing"
  exit 1
fi

if [[ "$missing_host_output" != *"DEPLOY_HOST"* ]]; then
  echo "missing host error should mention DEPLOY_HOST"
  exit 1
fi

dry_run_output="$(DEPLOY_HOST=root@example.invalid SKIP_CHECK=1 "$DEPLOY_SCRIPT" --dry-run)"
if [[ "$dry_run_output" != *"rsync"* ]] || [[ "$dry_run_output" != *"root@example.invalid"* ]]; then
  echo "dry-run should print the rsync command"
  exit 1
fi

if [[ "$dry_run_output" != *"bash scripts/check_frontend_sources.sh"* ]]; then
  echo "dry-run should print the remote frontend source check"
  exit 1
fi
