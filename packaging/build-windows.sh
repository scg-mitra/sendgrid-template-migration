#!/usr/bin/env bash
# Cross-compiles the Windows app from Linux and builds an installer and a
# portable .zip into dist/.
# Needs: Go, Node.js, nsis, zip, go-winres. No C compiler: Wails on Windows is pure Go.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.0.0-dev}"
APP=sendgrid-template-migration
NUMERIC_VERSION="${VERSION%%-*}" # Windows file versions must be numeric
mkdir -p dist/windows

# Embed the icon, version info and a DPI-aware manifest into the .exe.
go-winres simply \
  --arch amd64 \
  --icon assets/app-icon.png \
  --manifest gui \
  --product-name "SendGrid Template Migration" \
  --file-description "SendGrid Template Migration" \
  --product-version "$NUMERIC_VERSION" \
  --file-version "$NUMERIC_VERSION" \
  --copyright "SCG Growth Partners"

# -H windowsgui: no console window when opened from the Start menu.
[ -n "${SKIP_FRONTEND:-}" ] || packaging/build-frontend.sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags production \
  -ldflags "-s -w -H windowsgui -X main.version=${VERSION}" -o dist/windows/$APP.exe .
rm -f rsrc_windows_*.syso

makensis -V2 -DVERSION="$VERSION" -DNUMERIC_VERSION="$NUMERIC_VERSION" \
  -DOUTFILE="$PWD/dist/${APP}_${VERSION}_windows_amd64_setup.exe" \
  -DSRCDIR="$PWD" packaging/windows/installer.nsi
(cd dist/windows && zip -q "../${APP}_${VERSION}_windows_amd64_portable.zip" $APP.exe)
