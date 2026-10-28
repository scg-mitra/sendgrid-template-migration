#!/usr/bin/env bash
# Builds the Linux binary plus .deb, .rpm and .tar.gz into dist/.
# Needs: Go, gcc, GTK3 + WebKit2GTK 4.1 dev headers, Node.js and nfpm.
set -euo pipefail
cd "$(dirname "$0")/.."

export VERSION="${VERSION:-0.0.0-dev}"
APP=sendgrid-template-migration
mkdir -p dist/linux

[ -n "${SKIP_FRONTEND:-}" ] || packaging/build-frontend.sh
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags gtk3,production \
  -ldflags "-s -w -X main.version=${VERSION}" -o dist/linux/$APP .

nfpm package --config packaging/nfpm.yaml --packager deb --target dist/
nfpm package --config packaging/nfpm.yaml --packager rpm --target dist/
tar -czf "dist/${APP}_${VERSION}_linux_amd64.tar.gz" -C dist/linux $APP -C ../.. README.md
