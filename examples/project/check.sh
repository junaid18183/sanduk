#!/usr/bin/env sh
#
# A stand-in for "the app". It reports which of the project's expected
# variables this process received.
#
# Only presence is reported, never a value, so the output is safe to paste into
# an issue. Compare it with what runs without sanduk: nothing will be set.
set -eu

cd "$(dirname "$0")"

if [ ! -f .env.example ]; then
  echo "no .env.example next to this script" >&2
  exit 1
fi

echo "This process received:"
sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' .env.example |
  while read -r name; do
    if printenv "$name" >/dev/null 2>&1; then
      printf '  %-15s set\n' "$name"
    else
      printf '  %-15s not set\n' "$name"
    fi
  done
