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
make build        # produces ./sanduk
# or
go install github.com/junaid18183/sanduk@latest
```

## Quick start

```sh
# a flat secret
sanduk add openai --env OPENAI_API_KEY

# a group of related fields (field names are the env var names)
sanduk add-group saic-gov \
  --field AWS_ACCESS_KEY_ID=AKIA... \
  --field AWS_SECRET_ACCESS_KEY=...

sanduk list
```

### Run a command with secrets injected

```sh
sanduk exec openai -- aider
sanduk exec saic-gov -- aws s3 ls

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
CREDS_JSON=$(sanduk export saic-gov --format json)
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

`keychain` (macOS), `secret-service` (GNOME Keyring), `kwallet` (KDE),
`wincred` (Windows), `pass`, and `file` (encrypted file, for headless use).

When unset, the platform default is used. The `file` backend reads its
passphrase from `SANDUK_FILE_PASSPHRASE` or prompts, and stores data under
`$XDG_DATA_HOME/sanduk` (or `~/.local/share/sanduk`).

### Configuration

| Variable | Purpose |
| --- | --- |
| `SANDUK_BACKEND` | Select the keyring backend. |
| `SANDUK_SERVICE` | Override the keychain service name. |
| `SANDUK_CONFIG` | Override the config file path. |
| `SANDUK_FILE_DIR` | Data directory for the `file` backend. |
| `SANDUK_FILE_PASSPHRASE` | Passphrase for the `file` backend (non-interactive). |

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
  with your agent's own command-permission rules, and consider requiring
  user-presence (Touch ID) on the keychain item for a stronger gate.

## Development

```sh
make test     # unit + integration tests (encrypted-file backend, no keychain prompts)
make vet
```

## License

MIT
