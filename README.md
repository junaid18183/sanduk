# sanduk

**sanduk** (Hindi/Urdu for *box* / *chest*) is a small vault for API keys and
other development secrets. It stores secrets in your operating system's secure
keystore and exposes them to the processes that need them — on demand, without
writing plaintext secrets to disk.

It is modelled on [aws-vault](https://github.com/99designs/aws-vault), but for
static API keys (OpenAI, DeepSeek, Stripe, …) rather than temporary AWS
credentials.

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
# a flat secret
sanduk add openai --env OPENAI_API_KEY

# a group of related fields (field names are the env var names)
sanduk add-group aws-prod \
  --field AWS_ACCESS_KEY_ID=AKIA... \
  --field AWS_SECRET_ACCESS_KEY=...

sanduk list
```

### Run a command with secrets injected

```sh
sanduk exec openai -- aider
sanduk exec aws-prod -- aws s3 ls

# bind one secret to a specific variable
sanduk exec OPENAI_API_KEY=openai -- python train.py
```

The secrets exist only inside the child process — they are never exported into
your shell, so they are not inherited by anything else you launch from it.

### Use from a `.env` / script

`export` is the sanctioned scripting interface (it is deliberately *not* gated on
a terminal):

```sh
# shell / dotenv
eval "$(sanduk export openai)"
sanduk export openai --format dotenv >> .env

# the aws-vault-style JSON flow
CREDS_JSON=$(sanduk export aws-prod --format json)
export AWS_ACCESS_KEY_ID=$(echo "$CREDS_JSON" | jq -r .AWS_ACCESS_KEY_ID)
export AWS_SECRET_ACCESS_KEY=$(echo "$CREDS_JSON" | jq -r .AWS_SECRET_ACCESS_KEY)
```

## Commands

| Command | Description |
| --- | --- |
| `sanduk add <name>` | Add/update a flat secret. |
| `sanduk add-group <name>` | Add/update a group (`--field K=V`, `--from-json`, `--from-file`). |
| `sanduk get <name>` | Print a raw value (terminal-gated; `--force` to override). |
| `sanduk exec <name> -- <cmd>` | Run a command with secrets injected. |
| `sanduk export <name> -f shell\|dotenv\|json` | Emit secrets for scripts. |
| `sanduk list` | List names and metadata (never values). |
| `sanduk remove <name>` | Delete an entry. |
| `sanduk rotate <name>` | Replace a flat secret's value. |
| `sanduk import <file.env>` | Bulk-import a `.env` (`--group`, `--shred`). |
| `sanduk config` | Show/open the metadata config. |

## How it works

- **Secrets** live in the keychain (service `sanduk`), one entry per name:
  either a flat `{value}` or a group `{fields}`.
- **Metadata** lives in `~/.config/sanduk/config.toml` — entry names, env-var
  mappings, and descriptions only. It never contains secret values, so it is
  safe to read, commit, and version.
- **`exec`** resolves an entry to environment variables and `execve`s your
  command. **`export`** formats the same resolution for scripts and `.env`.

### Backends

Selected with `--backend` / `SANDUK_BACKEND` (mirroring aws-vault):

| Backend | Notes |
| --- | --- |
| `keychain` | macOS login Keychain |
| `keychain-biometric` | macOS only, **experimental**: every read requires Touch ID / the login password |
| `secret-service` | GNOME Keyring |
| `kwallet` | KDE KWallet |
| `wincred` | Windows Credential Manager |
| `pass` | [password-store](https://www.passwordstore.org/) |
| `file` / `age` | age-encrypted file, for headless or scripted use |

When unset, the platform default is used. The `age` backend derives a key from
a passphrase via age's scrypt, keeps every item in one encrypted document, and
writes it atomically with mode `0600` under `$XDG_DATA_HOME/sanduk` (or
`~/.local/share/sanduk`). It takes its passphrase from
`SANDUK_FILE_PASSPHRASE` or prompts once per process.

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

### Configuration

| Variable | Purpose |
| --- | --- |
| `SANDUK_BACKEND` | Select the keyring backend (same as `--backend`). |
| `SANDUK_SERVICE` | Override the keychain service name. |
| `SANDUK_CONFIG` | Override the config file path. |
| `SANDUK_FILE_DIR` | Data directory for the `file`/`age` backend. |
| `SANDUK_FILE_PASSPHRASE` | Passphrase for the `file`/`age` backend (non-interactive). |

## Security notes

- Values are never accepted as command-line arguments, so they cannot leak via
  process listings (`ps`). Input comes from an echo-disabled prompt, an env var,
  a file, or stdin.
- `sanduk get` refuses to print a secret to a non-terminal unless `--force` is
  given.
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

You are running a macOS build that has **no Keychain backend**. Affected
versions are **v0.1.0 and v0.1.1**, which were compiled with `CGO_ENABLED=0`;
the Keychain backend in `99designs/keyring` is compiled only under
`darwin && cgo`, so those binaries had no OS keystore and fell through to a
misconfigured file backend.

Fixed in **v0.1.2**. Either upgrade, or build from source (cgo is on by default
on macOS):

```sh
go install github.com/junaid18183/sanduk@latest
```

sanduk now detects the situation and says so, falling back to the
age-encrypted file rather than failing:

```
sanduk: no OS keystore available, using the encrypted file backend at ~/.local/share/sanduk/sanduk.age
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
