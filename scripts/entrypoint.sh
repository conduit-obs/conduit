#!/bin/sh
set -e

# Generate dev RSA keys if they don't exist
KEY_DIR="${CONDUIT_KEY_DIR:-/etc/conduit/keys}"
mkdir -p "$KEY_DIR"

if [ ! -f "$KEY_DIR/private.pem" ]; then
  echo "Generating development RSA keys..."
  openssl genrsa -out "$KEY_DIR/private.pem" 2048 2>/dev/null
  openssl rsa -in "$KEY_DIR/private.pem" -pubout -out "$KEY_DIR/public.pem" 2>/dev/null
  echo "Keys generated at $KEY_DIR"
fi

export CONDUIT_JWT_PUBLIC_KEY_PATH="${CONDUIT_JWT_PUBLIC_KEY_PATH:-$KEY_DIR/public.pem}"
export CONDUIT_JWT_PRIVATE_KEY_PATH="${CONDUIT_JWT_PRIVATE_KEY_PATH:-$KEY_DIR/private.pem}"

exec /usr/local/bin/conduit "$@"
