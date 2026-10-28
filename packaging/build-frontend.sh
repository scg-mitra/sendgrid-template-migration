#!/usr/bin/env bash
# Builds the React frontend into frontend/dist, which the Go binary embeds.
# Needs: Node.js 20+.
set -euo pipefail
cd "$(dirname "$0")/../frontend"

npm ci --no-audit --no-fund
npm run build
