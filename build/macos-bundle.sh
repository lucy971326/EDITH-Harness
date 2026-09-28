#!/bin/sh
set -eu

bundle=.build/Harness.app
mkdir -p "$bundle/Contents/MacOS"
cp .build/harness-desktop "$bundle/Contents/MacOS/Harness"
cat > "$bundle/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>com.edith.harness.desktop</string>
  <key>CFBundleName</key><string>Harness</string>
  <key>CFBundleExecutable</key><string>Harness</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
</dict></plist>
EOF
codesign --force --deep --sign - "$bundle"
