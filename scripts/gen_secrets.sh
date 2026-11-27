#!/usr/bin/env bash
set -euo pipefail

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required to generate deployment secrets" >&2
  exit 1
}

cat <<'EOF'
# Charge Console deployment secrets
# Store these in the service environment file (for example /etc/charge-api.env).
# Do not commit generated values to Git.
EOF

printf 'CHARGE_COOKIE_KEY='
openssl rand -base64 32
