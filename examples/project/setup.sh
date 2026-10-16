#!/usr/bin/env sh
#
# Load the dummy values from demo.env into the vault as a group, using whatever
# backend you have configured. Nothing here is a real secret, so this only
# demonstrates the workflow.
set -eu

# Same convention as the Makefile: override with SANDUK=/path/to/sanduk.
SANDUK=${SANDUK:-sanduk}
ENTRY=${DEMO_ENTRY:-demo}

cd "$(dirname "$0")"

if ! command -v "$SANDUK" >/dev/null 2>&1; then
  echo "sanduk is not on PATH. Install it first — see ../../README.md," >&2
  echo "or point SANDUK at the binary: SANDUK=./sanduk make setup" >&2
  exit 1
fi

echo "Loading demo.env into group \"$ENTRY\"."

# `sanduk config` writes its header to stderr and the config file itself to
# stdout, so route stderr into the pipe and drop stdout. This shows which
# backend is about to be written to, and why it was chosen.
if settings=$("$SANDUK" config 2>&1 >/dev/null); then
  echo
  echo "$settings" | sed 's/^# /  /'
fi

echo
if ! "$SANDUK" add-group "$ENTRY" --from-dotenv demo.env \
    --description "sanduk demo project"; then
  echo >&2
  echo "That failed. If the group already exists, refresh it field by field:" >&2
  echo "  $SANDUK group set $ENTRY --from-dotenv demo.env" >&2
  exit 1
fi

# Approve the committed .sanduk.toml so `sanduk exec -- <cmd>` works from any
# subdirectory without naming the entry. Editing the file revokes this. Show the
# file first: approval is a decision about its contents, so never make it blind.
echo "Approving .sanduk.toml:" >&2
sed 's/^/  /' .sanduk.toml >&2
"$SANDUK" allow

cat <<EOF

Done. Next:

  make doctor   # compare the entry against .env.example
  make show     # field names only
  make dev      # run ./check.sh with the secrets injected
  make clean    # remove the entry again
EOF
