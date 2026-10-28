#!/usr/bin/env bash
# Builds "SendGrid Template Migration.app" for Apple Silicon and wraps it in
# a .dmg in dist/. Must run on macOS (uses sips, iconutil, codesign, hdiutil).
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.0.0-dev}"
APP=sendgrid-template-migration
NAME="SendGrid Template Migration"
BUNDLE="dist/macos/$NAME.app"
rm -rf dist/macos && mkdir -p "$BUNDLE/Contents/MacOS" "$BUNDLE/Contents/Resources"

[ -n "${SKIP_FRONTEND:-}" ] || packaging/build-frontend.sh
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 MACOSX_DEPLOYMENT_TARGET=11.0 go build -trimpath -tags production \
  -ldflags "-s -w -X main.version=${VERSION}" -o "$BUNDLE/Contents/MacOS/$APP" .

# Icon: .png -> .iconset -> .icns
ICONSET=dist/macos/AppIcon.iconset
mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z $s $s assets/app-icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  if [ $d -le 512 ]; then
    sips -z $d $d assets/app-icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
  else
    cp assets/app-icon.png "$ICONSET/icon_${s}x${s}@2x.png"
  fi
done
iconutil -c icns "$ICONSET" -o "$BUNDLE/Contents/Resources/AppIcon.icns"
rm -rf "$ICONSET"

cat > "$BUNDLE/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$NAME</string>
  <key>CFBundleDisplayName</key><string>$NAME</string>
  <key>CFBundleIdentifier</key><string>com.scggrowthpartners.sendgridtemplatemigration</string>
  <key>CFBundleExecutable</key><string>$APP</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION%%-*}</string>
  <key>CFBundleVersion</key><string>${VERSION%%-*}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

# Ad-hoc signature (required to run on Apple Silicon). Not notarized.
codesign --force --deep --sign - "$BUNDLE"

# Disk image with an Applications shortcut for drag-to-install.
STAGE=dist/macos/dmg
mkdir -p "$STAGE"
cp -R "$BUNDLE" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "$NAME" -srcfolder "$STAGE" -ov -format UDZO \
  "dist/${APP}_${VERSION}_macos_arm64.dmg"
rm -rf "$STAGE"
