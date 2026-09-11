#!/usr/bin/env bash
# Stamps a version into every build asset that hardcodes one.
# Usage: build/set-version.sh 1.2.3   (a leading "v" is stripped)
set -euo pipefail
V="${1#v}"
if [[ ! "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "set-version: '$1' is not a version like 1.2.3" >&2
  exit 1
fi
NUMERIC="${V%%[-+]*}"   # Windows resource versions must be purely numeric
cd "$(dirname "$0")/.."
perl -pi -e 's/^(\s*version:\s*")[^"]*(")/${1}'"$V"'${2}/' build/config.yml build/linux/nfpm/nfpm.yaml
perl -pi -e 's/("file_version":\s*")[^"]*(")/${1}'"$NUMERIC"'${2}/' build/windows/info.json
perl -pi -e 's/("ProductVersion":\s*")[^"]*(")/${1}'"$V"'${2}/' build/windows/info.json
perl -pi -e 's/(INFO_PRODUCTVERSION ")[^"]*(")/${1}'"$V"'${2}/' build/windows/nsis/wails_tools.nsh
perl -0pi -e 's/(<key>CFBundle(?:ShortVersionString|Version)<\/key>\s*<string>)[^<]*(<\/string>)/${1}'"$V"'${2}/g' build/darwin/Info.plist build/darwin/Info.dev.plist
echo "set-version: stamped $V"
