#!/usr/bin/env bash
set -euo pipefail

echo "[multipass_file example] Updating apt cache..."
sudo apt-get update -y >/dev/null

echo "[multipass_file example] Creating app directory..."
mkdir -p "$HOME/app/bin"

echo "[multipass_file example] Writing marker file..."
cat <<'EOF' > "$HOME/app/README.txt"
This VM was provisioned by the multipass_file resource example.
It demonstrates transferring scripts, inline env files, and full directories.
EOF

echo "[multipass_file example] Done."

