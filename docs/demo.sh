#!/usr/bin/env bash
# Records the README demo. Drives the real binary against a throwaway age
# vault with fake values, so it never touches a developer's keychain.
#
#   make build
#   docs/demo.sh                      # writes docs/demo.cast and docs/demo.gif
#
# Needs asciinema and agg (brew install asciinema agg).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(dirname "$here")"

if [ "${SANDUK_DEMO_INNER:-}" != "1" ]; then
  command -v asciinema >/dev/null || { echo "asciinema not found" >&2; exit 1; }
  command -v agg >/dev/null || { echo "agg not found" >&2; exit 1; }
  [ -x "$repo/sanduk" ] || { echo "build it first: make build" >&2; exit 1; }

  rm -f "$here/demo.cast" "$here/demo.gif"
  SANDUK_DEMO_INNER=1 asciinema rec "$here/demo.cast" \
    --cols 84 --rows 26 --overwrite --command "$0"

  # The age backend runs scrypt on every command, so most of the wall clock is
  # a KDF with no output. Cap those gaps rather than make people watch them.
  agg --idle-time-limit 0.6 --speed 1.2 --font-size 17 --theme monokai \
      --last-frame-duration 4 "$here/demo.cast" "$here/demo.gif"
  echo "wrote $here/demo.cast and $here/demo.gif"
  exit 0
fi

# ---------------------------------------------------------------- inner run --
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/project"
cd "$work/project"

export PATH="$repo:$PATH"
export SANDUK_BACKEND=file
export SANDUK_FILE_PASSPHRASE=demo-passphrase
export SANDUK_FILE_DIR="$work/vault"
export SANDUK_CONFIG="$work/config.toml"
unset OPENAI_API_KEY

dim=$'\033[2m'; grn=$'\033[1;32m'; off=$'\033[0m'

note() { printf '%s# %s%s\n' "$dim" "$1" "$off"; sleep 0.5; }
run()  { printf '%s$%s %s\n' "$grn" "$off" "$1"; sleep 0.4; eval "${2:-$1}"; echo; sleep 0.7; }

sleep 1
note "Store a key. The value is prompted for, never passed in argv."
run "printf 'sk-test-DEMO-VALUE\n' | sanduk add openai --as OPENAI_API_KEY"

note "It is in the OS keystore now, not a .env file."
run "sanduk list"

note "Run a command with it injected:"
run "sanduk exec openai -- sh -c 'echo \"the child sees: \$OPENAI_API_KEY\"'"

note "...and it was never in your own shell:"
run "echo \"this shell sees: \${OPENAI_API_KEY:-<unset>}\""

note "Share one key across projects with a link, not a copy."
run "sanduk add-group warden --from-dotenv <(printf 'DB_URL=postgres://localhost/app\n')"
run "sanduk group link warden openai"
run "sanduk group show warden --long"

note "Rotate the secret once; every group that links it follows."
run "printf 'sk-test-ROTATED\n' | sanduk rotate openai"
run "sanduk export warden --format dotenv"

note "And find the copies that escaped. Here is one, checked into the repo:"
printf 'OPENAI_API_KEY = "sk-test-ROTATED"\n' > settings.py
run "cat settings.py"
run "sanduk scan . --yes" "sanduk scan . --yes || true"

sleep 1
