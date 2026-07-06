#!/usr/bin/env bash
set -euo pipefail

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required to generate deployment secrets" >&2
  exit 1
}

cat <<'EOF'
# Charge / yyb_go deployment secrets
# Store these in service environment files such as /etc/charge-api.env and /etc/yyb-go.env.
# Do not commit generated values to Git.
EOF

printf 'CHARGE_COOKIE_KEY='
openssl rand -base64 32

printf 'YYB_SECRET_KEY='
openssl rand -base64 32

printf 'YYB_API_SECRET='
openssl rand -base64 48
