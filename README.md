# sanduk

**sanduk** (Hindi/Urdu for *box* / *chest*) is a small vault for API keys and
other development secrets. It stores secrets in your operating system's secure
keystore and exposes them to the processes that need them — on demand, without
writing plaintext secrets to disk.

## Why

Secrets in a plaintext `.env` are readable by everything that can read files,
including AI coding agents running in the same environment. `sanduk` keeps the
values in the OS keychain instead, and injects them only into the child process
that actually needs them.

## Install

```sh
# from source
make build        # produces ./sanduk

# or with Go
go install github.com/junaid18183/sanduk@latest

# or download a release binary
# https://github.com/junaid18183/sanduk/releases/latest
```

Prebuilt binaries for macOS, Linux, and Windows (amd64/arm64) are attached to
each [release](https://github.com/junaid18183/sanduk/releases).

## Quick start

```sh
# a flat secret (value is read from an echo-off prompt, never from argv)
sanduk add openai --env OPENAI_API_KEY

# a group of related fields (field names are the env var names)
sanduk add-group warden --from-dotenv .env.warden

sanduk list
```

> `add-group --field KEY=VALUE` also exists, but a value passed as an argument
> is visible in `ps`. Prefer `--from-dotenv`, or `sanduk group set`, which prompts.
>
> An existing entry is never replaced silently: `add-group` asks you to use
> `group set` (to merge) or to pass `--replace` (to overwrite).

> A runnable version of everything below lives in
> [`examples/project`](examples/project):
> `cd examples/project && make setup && make dev`.

### Run a command with secrets injected

```sh
sanduk exec openai -- aider
sanduk exec warden -- pnpm dev

# bind one secret to a specific variable
sanduk exec OPENAI_API_KEY=openai -- python train.py

# merge several selections; a later one wins
sanduk exec BASE=base warden -- pnpm dev

# narrow what the child receives (unknown keys are an error, not a silence)
sanduk exec warden --only DB_URL,API_KEY -- psql "$DB_URL"
```

The secrets exist only inside the child process — they are never exported into
your shell, so they are not inherited by anything else you launch from it.

### Project workflow

Commit a `.sanduk.toml` in the repository root to name the entry the project uses
(and optionally the keys you normally need), so nobody has to retype it:

```toml
# .sanduk.toml — non-secret, safe to commit
entry = "warden"
only  = ["DB_URL", "API_KEY"]
```

A project file is **not trusted automatically**: a cloned repository could
otherwise name any entry (say `aws-prod`) and have its scripts receive those
secrets. Approve it once, after reviewing it, and re-approve after any edit:

```sh
sanduk allow
```

Now the entry can be omitted, from any subdirectory:

```sh
sanduk exec -- pnpm dev
sanduk export --format dotenv
```

Pair that with a committed `.env.example` (the keys, with dummy values) and
`sanduk doctor` reports drift between what the app expects and what is stored:

```sh
$ sanduk doctor
KEY          STATUS
DB_URL       ok
STRIPE_KEY   missing
LEGACY_TOKEN extra
warden: 1/2 template key(s) present; 1 not in the template: LEGACY_TOKEN
sanduk: 1 key(s) missing from group "warden": STRIPE_KEY
```

Missing keys exit non-zero; extra keys are a warning. Compare against another
file with `--template`.

Use `.envrc` only for interactive work — direnv exports into the shell, so every
child process of that shell (including an agent) inherits the secrets:

```sh
# .envrc — committed; names only, never values
eval "$(sanduk export --format shell)"
```

[`examples/project/.envrc`](examples/project/.envrc) takes the opposite approach:
it exports nothing and explains why, so the secrets still exist only in the
`sanduk exec` child.

### Use from a `.env` / script

`export` is the sanctioned scripting interface (it is deliberately *not* gated on
a terminal):

```sh
# shell / dotenv
eval "$(sanduk export openai)"
sanduk export openai --format dotenv -o .env    # writes with mode 0600

# feed a tool that insists on a file, without writing plaintext to disk
docker compose --env-file <(sanduk export warden --format dotenv) up

# the aws-vault-style JSON flow
CREDS_JSON=$(sanduk export aws-prod --format json)
export AWS_ACCESS_KEY_ID=$(echo "$CREDS_JSON" | jq -r .AWS_ACCESS_KEY_ID)
export AWS_SECRET_ACCESS_KEY=$(echo "$CREDS_JSON" | jq -r .AWS_SECRET_ACCESS_KEY)
```

## Commands

| Command | Description |
| --- | --- |
| `sanduk add <name>` | Add/update a flat secret (`--as`, `--replace`). |
| `sanduk add-group <name>` | Create a group (`--field K=V`, `--from-json`, `--from-dotenv`, `--replace`). |
| `sanduk group set <name> [KEY…]` | Merge fields into a group (prompt, stdin, `--from-env`, `--from-dotenv`, `--description`). |
| `sanduk group unset <name> KEY…` | Remove fields from a group. |
| `sanduk group show <name>` | List a group's field names (never values). |
| `sanduk group get <name> KEY` | Print one field (redacted unless `--reveal`; terminal-gated, `--force`). |
| `sanduk get <name>` | Print a value (terminal-gated; `--field`/`--all`/`--reveal` for groups). |
| `sanduk exec [selector…] -- <cmd>` | Run a command with secrets injected (`--only`, `--except`, `--list`). |
| `sanduk export [selector…] -f shell\|dotenv\|json` | Emit secrets for scripts (`--only`, `--except`, `-o FILE` writes mode 0600). |
| `sanduk list` | List names and metadata (never values; `--json`, `--names`). |
| `sanduk doctor [entry]` | Check a group against `.env.example` (`--template`). |
| `sanduk allow [dir]` | Approve a repository's `.sanduk.toml` for `exec`/`export`. |
| `sanduk remove <name>` | Delete an entry (prompts; `--yes` when non-interactive). |
| `sanduk rotate <name>` | Replace a flat secret's value. |
| `sanduk import <file.env>` | Bulk-import a `.env` (`--group`, `--replace`, `--delete`, `--dry-run`). |
| `sanduk config` | Show the config file and the settings sanduk resolved (`--path`, `--edit`). |

Both `exec` and `export` take the same *selectors*: a bare entry name (a group
contributes one variable per field), or `ENVVAR=entry` to bind a single flat
secret to a variable. They are merged left to right, and `.sanduk.toml` supplies
the entry when none is given (after you approve it with `sanduk allow`).
`sanduk exec --list` prints only the variable names that would be injected.

## How it works

- **Secrets** live in the keychain (service `sanduk`), one entry per name:
  either a flat `{value}` or a group `{fields}`.
- **Metadata** lives in `~/.config/sanduk/config.toml` — entry names, env-var
  mappings, and descriptions only. It never contains secret values, so it is
  safe to read, commit, and version.
- **`exec`** resolves an entry to environment variables and `execve`s your
  command. **`export`** formats the same resolution for scripts and `.env`.

### Backends

Selected with `--backend`, `SANDUK_BACKEND`, or `backend` in the config file. A
flag beats the environment variable, which beats the config file, which beats the
platform default — so one config file can serve several machines:

| Backend | Notes |
| --- | --- |
| `keychain` | macOS login Keychain |
| `keychain-biometric` | macOS only, **experimental**: every read requires Touch ID / the login password |
| `secret-service` | GNOME Keyring |
| `kwallet` | KDE KWallet |
| `wincred` | Windows Credential Manager |
| `pass` | [password-store](https://www.passwordstore.org/) |
| `file` / `age` | age-encrypted file, for headless or scripted use |

When unset, the platform default is used. `OpenKeyring` falls back to the age
file only when no native keystore is *compiled in* (for example a
`CGO_ENABLED=0` macOS build); a native backend that merely fails to open returns
an error, so secrets are never silently split across two stores.

The `age` backend derives a key from a passphrase via age's scrypt, keeps every
item in one encrypted document, and writes it atomically with mode `0600` under
`$XDG_DATA_HOME/sanduk` (or `~/.local/share/sanduk`). It takes its passphrase from
`SANDUK_FILE_PASSPHRASE` or prompts once per process, asks for it twice when the
vault is first created (so a typo cannot lock it), and holds an exclusive lock
while writing so two concurrent invocations cannot lose an update.

#### Requiring Touch ID (`keychain-biometric`)

```sh
sanduk --backend keychain-biometric add openai
sanduk --backend keychain-biometric list
```

On macOS this stores each item with a **user-presence access control**, so
reading it prompts for Touch ID or your login password. Because a headless
process cannot satisfy that prompt, this is the strongest gate sanduk offers
against an agent reading your secrets.

Two caveats:

- It is **experimental**. It compiles on macOS with cgo enabled (`CGO_ENABLED=1`),
  but the biometric prompt has not been verified in CI. Off macOS, or with cgo
  disabled, selecting this backend returns an error.
- It uses a separate namespace (`sanduk-biometric`), so items you stored with
  the standard `keychain` backend will not appear. Re-add them to use it.

#### Using a dedicated keychain

By default sanduk uses the macOS **login keychain**, the one that is already
unlocked for a logged-in user. You can point it at its own keychain instead:

```sh
sanduk --keychain sanduk list        # or SANDUK_KEYCHAIN=sanduk
```

This creates `~/Library/Keychains/sanduk.keychain` on the first write, with a
password the system prompts for. Things worth knowing before you reach for it:

- It is a **second unlock boundary**, not a strictly stronger one. A dedicated
  keychain's password normally has to live in the login keychain, which collapses
  per-item access control into one grant.
- After a reboot the dedicated keychain is locked, so reads will prompt.
- Prompt fatigue is mostly a **code-signing** problem: macOS ties keychain access
  to a signed identity, so an unsigned or rebuilt binary looks like a new app and
  re-prompts. `aws-vault` signs its macOS releases for exactly this reason.
- `SANDUK_KEYCHAIN_PASSPHRASE` sets the password non-interactively when the
  keychain is first created (for scripting; it is a secret, so prefer the prompt).

### Configuration

| Variable | Purpose |
| --- | --- |
| `SANDUK_BACKEND` | Select the keyring backend (same as `--backend`). |
| `SANDUK_KEYCHAIN` | macOS keychain name (same as `--keychain`; default `login`). |
| `SANDUK_KEYCHAIN_PASSPHRASE` | Password used when creating a dedicated keychain. |
| `SANDUK_SERVICE` | Override the keychain service name (same as `--service`). |
| `SANDUK_CONFIG` | Override the config file path. |
| `SANDUK_TRUST` | Override the project-approval store path (`trusted.toml`). |
| `SANDUK_FILE_DIR` | Data directory for the `file`/`age` backend. |
| `SANDUK_FILE_PASSPHRASE` | Passphrase for the `file`/`age` backend (non-interactive). |
| `.sanduk.toml` (file) | Repository's default entry and optional key subset (approve with `sanduk allow`). |

The metadata config can also carry standing backend settings. They are defaults
only, and are resolved last:

```toml
# ~/.config/sanduk/config.toml — non-secret, safe to version
backend  = "keychain"   # or secret-service, wincred, pass, file/age, ...
keychain = "sanduk"     # macOS: a dedicated keychain (opt in deliberately)
service  = "sanduk"     # namespace; also names the age vault file
```

`backend` is machine-specific — a config that says `"keychain"` fails on Linux —
so set it on the machine that needs it, or leave it out and let the platform
decide. Ask sanduk what it actually resolved, and where each value came from:

```sh
$ sanduk config
# path:     /Users/you/.config/sanduk/config.toml
# backend:  keychain (from default)
# keychain: login (from default)
# service:  sanduk (from default)
# flags beat environment variables, which beat this file
```

A value that loses to a flag or an environment variable says so (`from flag`,
`from env`), so there is no guessing about which of the three is in play. The
annotated file is [`examples/config.toml.example`](examples/config.toml.example).

## Security notes

- Values are never accepted as command-line arguments, so they cannot leak via
  process listings (`ps`). Input comes from an echo-disabled prompt, an env var,
  a file, or stdin. The one exception is `add-group --field K=V` /
  `--from-json`, kept for convenience; `--from-dotenv` and `group set` avoid it.
- `exec` strips every `SANDUK_*` variable from the child environment, so a child
  cannot read `SANDUK_FILE_PASSPHRASE` (or any other sanduk setting) out of its
  own environment and unlock the whole vault. This keeps `--only` meaningful.
- `--only` injects a least-privilege subset of a group, and an unknown key is an
  error rather than a silent omission. `--except` removes keys instead.
- `sanduk get` refuses to print a secret to a non-terminal unless `--force` is
  given. For a group, `--field KEY` reads one value and redacts it unless
  `--reveal` is passed, so reviewing a secret never dumps the whole group.
- Field names are validated on write, so a name with a space or a
  loader-affecting name such as `LD_PRELOAD` or `NODE_OPTIONS` is refused rather
  than injected into a child.
- An unapproved `.sanduk.toml` is refused by `exec`/`export`; approve it with
  `sanduk allow`, which stores a hash so an edit revokes the approval.
- Unlike aws-vault, sanduk cannot issue short-lived credentials — there is no
  token exchange for a static API key, so a leaked key does not expire. The
  protection is that the value lives in the keychain and is injected only where
  needed.
- A process running as *you* with shell access can still run `sanduk`. Pair this
  with your agent's own command-permission rules, and use the experimental
  `keychain-biometric` backend so reading a secret requires Touch ID, which a
  headless agent cannot satisfy.

## Troubleshooting

### `sanduk: No directory provided for file keyring`

This means the binary has **no macOS Keychain backend**. That backend is
compiled only under `darwin && cgo` in `99designs/keyring`, so a macOS build
made with `CGO_ENABLED=0` has no OS keystore at all and falls through to a
misconfigured file backend.

Official release binaries are built with cgo. If you built sanduk yourself,
build with cgo enabled (the default on macOS):

```sh
go install github.com/junaid18183/sanduk@latest
```

When no OS keystore is *compiled in*, sanduk says so and falls back to the
age-encrypted file instead of failing:

```
sanduk: no OS keystore compiled in, using the encrypted file backend at ~/.local/share/sanduk/sanduk.age
```

### `sanduk list` shows no entries after changing backend

Each backend is a separate namespace. Items stored with `keychain` are not
visible under `keychain-biometric`, and vice versa. Use the same `--backend`
you stored them with.

## Development

```sh
make test     # unit + integration tests (encrypted-file backend, no keychain prompts)
make vet
```

CI runs `gofmt`, `go vet`, the test suite, and cross-compilation on Linux and
macOS. Releases are cut by pushing a `v*` tag, which triggers GoReleaser.

**macOS builds must use `CGO_ENABLED=1`** or the Keychain backend is omitted.
`.goreleaser.yml` therefore splits darwin (cgo) from linux/windows (no cgo), and
the release workflow runs on a macOS runner so darwin can be linked against the
Security framework. `vault.TestNativeBackendsIncludesKeychainOnMacOS` fails the
build if that regresses.

## License

MIT
