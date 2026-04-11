#!/usr/bin/env bash
set -euo pipefail

KEY_DIR="${1:-.conduit/keys}"
mkdir -p "$KEY_DIR"

echo "Generating RSA 2048-bit keypair in $KEY_DIR/"
openssl genrsa -out "$KEY_DIR/private.pem" 2048 2>/dev/null
openssl rsa -in "$KEY_DIR/private.pem" -pubout -out "$KEY_DIR/public.pem" 2>/dev/null
chmod 600 "$KEY_DIR/private.pem"
chmod 644 "$KEY_DIR/public.pem"

echo "Done."
echo "  Private key: $KEY_DIR/private.pem"
echo "  Public key:  $KEY_DIR/public.pem"
