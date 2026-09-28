#!/bin/sh
set -eu

version=$1
bundle=.build/EDITH.app
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp .build/EDITH "$bundle/Contents/MacOS/EDITH"
cp build/macos/icon.icns "$bundle/Contents/Resources/EDITH.icns"
cat > "$bundle/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>com.edith.harness.desktop</string>
  <key>CFBundleName</key><string>EDITH</string>
  <key>CFBundleDisplayName</key><string>EDITH</string>
  <key>CFBundleExecutable</key><string>EDITH</string>
  <key>CFBundleIconFile</key><string>EDITH.icns</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
</dict></plist>
EOF
codesign --force --deep --sign - "$bundle"
