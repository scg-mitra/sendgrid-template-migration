#!/usr/bin/env bash
# Prints the CHANGELOG.md section for a version, for use as release notes.
# Usage: packaging/release-notes.sh 1.2.3   (a leading "v" is ignored)
# Fails if the section is missing or empty, so a release can't go out
# without notes.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:?usage: $0 VERSION}"
version="${version#v}"

notes="$(awk -v ver="$version" '
  /^## \[/ { if (found) exit; if (index($0, "## [" ver "]") == 1) { found = 1; next } }
  /^\[[^]]+\]: / { if (found) exit }
  found { print }
' CHANGELOG.md | sed -e '/./,$!d' -e :a -e '/^\n*$/{$d;N;ba' -e '}')"

if [[ -z "$notes" ]]; then
  echo "CHANGELOG.md has no entries for $version. Add a '## [$version] - YYYY-MM-DD' section." >&2
  exit 1
fi
printf '%s\n' "$notes"
