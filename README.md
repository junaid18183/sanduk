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

# or with Go (Go 1.26+; on macOS also a C toolchain, e.g. xcode-select --install,
# because the Keychain backend needs cgo)
go install github.com/junaid18183/sanduk@latest

# or download a release binary
# https://github.com/junaid18183/sanduk/releases/latest
```

Prebuilt binaries for macOS, Linux, and Windows (amd64/arm64) are attached to
each [release](https://github.com/junaid18183/sanduk/releases), with a signed
`checksums.txt`. To verify a download:

```sh
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/junaid18183/sanduk/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt     # macOS: shasum -a 256 ...
gh attestation verify sanduk_*_darwin_arm64.tar.gz --repo junaid18183/sanduk
```

## Quick start

```sh
# a flat secret (value is read from an echo-off prompt, never from argv)
sanduk add openai --as OPENAI_API_KEY

# a group of related fields (field names are the env var names)
sanduk add-group warden --from-dotenv .env.warden

sanduk list
```

> `add-group --field KEY` prompts for that field's value with echo off. Values
> are never taken from the command line, where `ps` would show them.
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
sanduk exec warden --only DB_URL,API_KEY -- sh -c 'psql "$DB_URL"'
```

Quote a `$VAR` the command needs, as above: written bare, your shell would expand
it *before* sanduk runs — and it is not set in your shell, which is the point.

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
KEY           STATUS
DB_URL        ok
STRIPE_KEY    missing
LEGACY_TOKEN  extra
warden: 1/2 template key(s) present; 1 not in the template: LEGACY_TOKEN
sanduk: 1 key(s) missing from group "warden": STRIPE_KEY
```

Missing keys exit with status 3 (other failures exit 1); extra keys are a
warning. Compare against another file with `--template`. `sanduk doctor
--metadata` checks something else: that the metadata config agrees with the
vault (`--fix` repairs it).

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
# shell (POSIX: sh, bash, zsh — not fish)
eval "$(sanduk export openai)"

# dotenv file: mode 0600 on Unix; replaces the whole file, and refuses an
# existing one unless --force is given (it never appends)
sanduk export openai --format dotenv -o .env

# feed a tool that insists on a file, without writing plaintext to disk
docker compose --env-file <(sanduk export warden --format dotenv) up

# JSON, for a program that reads it
sanduk export aws-prod --format json | ./deploy --credentials-from-stdin
```

`eval "$(sanduk export …)"` puts the secrets in your shell, where every later
command can read them; prefer `exec`, which confines them to one process.

## Commands

| Command | Description |
| --- | --- |
| `sanduk add <name>` | Add/update a flat secret (`--as VAR`, `--from-env VAR`, `--from-file F`, `--description`, `--replace`). |
| `sanduk add-group <name>` | Create a group (`--from-dotenv F`, `--from-json F\|-`, `--field KEY` prompts, `--description`, `--replace`). |
| `sanduk group set <name> [KEY…]` | Merge fields into a group (prompt, stdin, `--from-env` reads each KEY from `$KEY`, `--from-dotenv F`, `--description`). |
| `sanduk group unset <name> KEY…` | Remove fields from a group. |
| `sanduk group show <name>` | List a group's field names (never values). |
| `sanduk group get <name> KEY` | Inspect one field (redacted unless `--reveal`; terminal-gated, `--force`). |
| `sanduk get <name>` | Inspect a value, redacted unless `--reveal` (terminal-gated, `--force`; `--field`/`--all` for groups). |
| `sanduk exec [selector…] -- <cmd>` | Run a command with secrets injected (`--only`, `--except`, `--list`). |
| `sanduk export [selector…]` | Emit secrets for scripts (`-f shell\|dotenv\|json`, `--only`, `--except`, `-o FILE`, `--force`). |
| `sanduk list` | List names and metadata (never values; `--json`, `--names`). An entry with no metadata shows as `unknown`. |
| `sanduk doctor [entry]` | Check a group against `.env.example` (`--template F`); exits 3 on missing keys. `--metadata [--fix]` checks config.toml against the vault. |
| `sanduk allow [dir]` | Approve a repository's `.sanduk.toml` for `exec`/`export`/`doctor`. |
| `sanduk remove <name>` | Delete an entry and its metadata (prompts; `--yes` when non-interactive). |
| `sanduk rotate <name>` | Replace a flat secret's value (`--from-env VAR`, `--from-file F`, or prompt/stdin). |
| `sanduk import <file.env>` | Bulk-import a `.env` (`--group NAME`, `--replace` to overwrite existing entries, `--delete`, `--dry-run`). |
| `sanduk config` | Show the config file and the settings sanduk resolved (`--path`, `--edit` opens `$EDITOR`). |
| `sanduk completion <shell>` | Print a completion script for bash, zsh, fish or powershell. |
| `sanduk version` | Print the version (also `--version`). |

Global flags, accepted by every command: `--backend`, `--keychain`, `--service`
(each overriding its `SANDUK_*` variable and the config file) and `--config`
(the metadata config path; the trust store moves with it).

Both `exec` and `export` take the same *selectors*: a bare entry name (a group
contributes one variable per field), or `ENVVAR=entry` to bind a single flat
secret to a variable. They are merged left to right, and `.sanduk.toml` supplies
the entry when none is given (after you approve it with `sanduk allow`).
`sanduk exec --list` prints only the variable names that would be injected.

## How it works

- **Secrets** live in the keychain (service `sanduk`), one entry per name:
  either a flat `{value}` or a group `{fields}`.
- **Metadata** lives in `~/.config/sanduk/config.toml` (or
  `$XDG_CONFIG_HOME/sanduk/config.toml`) — entry names, env-var mappings, and
  descriptions only. It never contains secret values, so it is safe to read,
  commit, and version. Project approvals live next to it in `trusted.toml`.
- **`exec`** resolves an entry to environment variables and `execve`s your
  command, so sanduk itself is gone by the time it runs (on Windows, which has no
  `execve`, sanduk starts the command and passes its exit status through).
  **`export`** formats the same resolution for scripts and `.env`.

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

When unset, exactly one platform keystore is used: the Keychain on macOS, the
Credential Manager on Windows, and the Secret Service elsewhere. If it fails to
open, that is an error — sanduk never moves on to another store, so secrets are
not silently split across two of them. `kwallet` and `pass` are used only when
selected explicitly. sanduk falls back to the age file only when the platform
keystore is not *compiled in* (for example a `CGO_ENABLED=0` macOS build).

The `age` backend derives a key from a passphrase via age's scrypt, keeps every
item in one encrypted document, and writes it atomically with mode `0600` under
`$XDG_DATA_HOME/sanduk` (or `~/.local/share/sanduk`). It takes its passphrase from
`SANDUK_FILE_PASSPHRASE` or prompts once per process, asks for it twice when the
vault is first created (so a typo cannot lock it), and holds an exclusive
cross-process lock while writing so two concurrent invocations cannot lose an
update. It warns if the file has become readable by other users.

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
  re-prompts. Signed macOS releases avoid this; sanduk's are not signed yet.
- `SANDUK_KEYCHAIN_PASSPHRASE` sets the password non-interactively when the
  keychain is first created (for scripting; it is a secret, so prefer the prompt).

### Configuration

| Variable | Purpose |
| --- | --- |
| `SANDUK_BACKEND` | Select the keyring backend (same as `--backend`). |
| `SANDUK_KEYCHAIN` | macOS keychain name (same as `--keychain`; default `login`). |
| `SANDUK_KEYCHAIN_PASSPHRASE` | Password used when creating a dedicated keychain. |
| `SANDUK_SERVICE` | Override the keychain service name (same as `--service`). |
| `SANDUK_CONFIG` | Override the config file path (same as `--config`). |
| `SANDUK_TRUST` | Override the project-approval store path (default `trusted.toml` next to the config). |
| `SANDUK_FILE_DIR` | Data directory for the `file`/`age` backend. |
| `SANDUK_FILE_PASSPHRASE` | Passphrase for the `file`/`age` backend (non-interactive). |
| `XDG_CONFIG_HOME` / `XDG_DATA_HOME` | Base directories for the config (`…/sanduk/config.toml`) and the age vault (`…/sanduk/`). |
| `EDITOR` | Editor for `sanduk config --edit` (default `vi`). |
| `PASSWORD_STORE_DIR` | Read by the `pass` backend, as by `pass` itself. |
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
# path:    /Users/you/.config/sanduk/config.toml
# backend: platform default (from default)
# keychain: login (from default)
# service: sanduk (from default)
# flags beat environment variables, which beat this file
```

A value that loses to a flag or an environment variable says so (`from flag`,
`from env`), so there is no guessing about which of the three is in play. The
annotated file is [`examples/config.toml.example`](examples/config.toml.example).

## Threat model

**What sanduk protects against.** The main case is a process that runs as you
and reads files: an AI coding agent, a compromised dependency's install script,
a tool that uploads your working tree. A plaintext `.env` gives all of these
every secret. With sanduk the values are in the OS keystore, and a command gets
them only when you run it under `sanduk exec`. sanduk also guards against a
**cloned repository** choosing which secrets its own scripts receive, and
against values leaking through process listings, shell history, logs or pipes.

**What it does not protect against.** Anything that can run commands as you can
run `sanduk` too, and root or a debugger attached to your processes can read
anything. A secret handed to a child is that child's to leak. sanduk cannot
make a static API key short-lived: there is no token exchange for one, so a
leaked key stays valid until you rotate it.

**Trust boundaries.** The keystore holds the values. config.toml and
trusted.toml hold names and hashes only, and are written with mode 0600.
`.sanduk.toml` comes from the repository and is untrusted until you approve its
exact contents. Your shell's environment is outside the boundary: `export` and
direnv put secrets there; `exec` does not.

### Security notes

- Values are never accepted as command-line arguments, so they cannot leak via
  process listings (`ps`). Input comes from an echo-disabled prompt, an env var,
  a file, or stdin — including for `add-group`, whose `--field KEY` prompts and
  whose `--from-json` reads a file or stdin.
- `exec` strips every `SANDUK_*` variable (in any letter case) from the child
  environment, so a child cannot read `SANDUK_FILE_PASSPHRASE` (or any other
  sanduk setting) out of its own environment and unlock the whole vault. This
  keeps `--only` meaningful.
- `--only` injects a least-privilege subset of a group, and an unknown key is an
  error rather than a silent omission. `--except` removes keys instead.
- `sanduk get` is for inspecting: everything it prints shows only the last four
  characters unless `--reveal` is passed, and it refuses to print to a
  non-terminal unless `--force` is given. For a group, `--field KEY` reads one
  value, so reviewing a secret never dumps the whole group. This gate is a speed
  bump, not a boundary: `export` is ungated by design, for scripts.
- Every variable name is validated when it is stored and again before it reaches
  a child or `export`'s output: a group field, a flat secret's name, an
  `ENVVAR=entry` binding and a name from config.toml alike. A name that is not a
  plain identifier is refused, as are names that change how a child runs
  (`LD_*`, `DYLD_*`, `GIT_CONFIG_*`, `SANDUK_*`, `NODE_OPTIONS`, `PYTHONPATH`,
  `BASH_ENV`, …).
- `export --format shell` is POSIX shell syntax (sh, bash, zsh). Do not `eval`
  it in fish, which treats backslashes inside single quotes differently.
- An unapproved `.sanduk.toml` is refused by `exec`, `export` and `doctor`;
  approve it with `sanduk allow`, which stores a hash of the exact bytes it
  parsed, so an edit revokes the approval. A project file that is world-writable
  or owned by another user is refused, and so is a trust store that lives inside
  the project or that group/other users can access.
- With no backend set, sanduk uses exactly one platform keystore and treats a
  failure to open it as an error, so secrets never end up split across stores.
- A process running as *you* with shell access can still run `sanduk`. Pair this
  with your agent's own command-permission rules, and use the experimental
  `keychain-biometric` backend so reading a secret requires Touch ID, which a
  headless agent cannot satisfy.

Report a vulnerability privately — see [SECURITY.md](SECURITY.md).

## Windows

sanduk runs on Windows with the Credential Manager (`wincred`) as its default
backend, or the age file. The differences:

- `exec` starts the command as a child and waits for it, since Windows has no
  `execve`; the child's exit status is passed through.
- Files are not protected by Unix mode bits. "Mode 0600" in this README means
  nothing there; the file inherits the directory's ACL, so keep the config and
  age directories under your user profile. The ownership checks on
  `.sanduk.toml` and the trust store are Unix-only.
- Environment variable names are case-insensitive, and sanduk strips `SANDUK_*`
  from a child in any case.

## Shell completion

```sh
sanduk completion zsh > "${fpath[1]}/_sanduk"          # zsh
sanduk completion bash > /etc/bash_completion.d/sanduk  # bash
sanduk completion fish > ~/.config/fish/completions/sanduk.fish
```

Completion suggests entry names. It lists names only, and with the age backend
only when `SANDUK_FILE_PASSPHRASE` is set, so it never stops to prompt.

## Backup, migration and removal

- **Back up** the vault and its metadata together. With the age backend that is
  `~/.local/share/sanduk/<service>.age` plus `config.toml` and `trusted.toml`
  from `~/.config/sanduk/`. With a native keystore, the items live in that store
  (on macOS, Keychain Access lists them under the service `sanduk`), and the
  same two config files hold the metadata.
- **Move entries to another backend** one at a time, since by design there is no
  command that dumps every secret at once:

  ```sh
  # a group
  sanduk --backend file import --group warden \
    <(sanduk --backend keychain export warden --format dotenv)
  # a flat secret
  sanduk --backend keychain export openai --format json | jq -r .OPENAI_API_KEY |
    sanduk --backend file add openai --as OPENAI_API_KEY
  ```

  Then remove the originals with `sanduk --backend keychain remove <name>`.
- **Revoke a project approval** by deleting its line from `trusted.toml` (or
  editing the project file, which revokes it automatically).
- **Uninstall**: `sanduk remove` each entry (`sanduk list --names`), then delete
  the binary, `~/.config/sanduk/` and `~/.local/share/sanduk/`.

## Troubleshooting

### `sanduk: no OS keystore compiled in, using the encrypted file backend`

This means the binary has **no macOS Keychain backend**. That backend is
compiled only under `darwin && cgo` in `99designs/keyring`, so a macOS build
made with `CGO_ENABLED=0` has no OS keystore at all.

Official release binaries are built with cgo. If you built sanduk yourself,
build with cgo enabled (the default on macOS):

```sh
go install github.com/junaid18183/sanduk@latest
```

Until you rebuild, sanduk uses the age-encrypted file and says so:

```
sanduk: no OS keystore compiled in, using the encrypted file backend at ~/.local/share/sanduk/sanduk.age
```

### `sanduk list` shows no entries after changing backend

Each backend is a separate namespace. Items stored with `keychain` are not
visible under `keychain-biometric`, and vice versa. Use the same `--backend`
you stored them with.

### `… .sanduk.toml is not approved`

The repository's project file has not been approved, or has changed since. Read
it, then run `sanduk allow`. Approval is per file contents, so any edit — even
one you made — needs a fresh `allow`.

### `could not open the secret-service keystore` (Linux)

With no backend set, sanduk uses only the Secret Service, and reports an error
rather than switching to another store. Make sure a Secret Service provider is
running and unlocked (GNOME Keyring, or KWallet 5.97+ which also provides it),
or pick a backend explicitly: `--backend kwallet`, `--backend pass` or
`--backend file`. Headless machines usually want `file`.

### `trust store: … has mode 0644`

sanduk writes `trusted.toml` as 0600 and refuses one that other users can read
or write, since such a file may not be yours (a repository's `.envrc` could point
`SANDUK_TRUST` at one it ships). If it is yours, `chmod 600` it.

### `config is unreadable`

config.toml failed to parse. Commands that use the vault stop rather than write
secrets whose metadata cannot be recorded. `sanduk config` still prints the file
and the error; fix it with `sanduk config --edit`. If entries and metadata have
drifted, `sanduk doctor --metadata --fix` repairs them.

## Development

```sh
make test     # unit + integration tests (encrypted-file backend, no keychain prompts)
make check    # everything CI runs: gofmt, go mod tidy, vet, race tests, lint, govulncheck, cross-compile
```

CI runs on Linux, macOS and Windows. Releases are cut by pushing a `v*` tag: the
release workflow runs CI first, then GoReleaser, which signs `checksums.txt` with
keyless cosign and attaches SBOMs and build-provenance attestations. See
[CONTRIBUTING.md](CONTRIBUTING.md) and [CHANGELOG.md](CHANGELOG.md).

**macOS builds must use `CGO_ENABLED=1`** or the Keychain backend is omitted.
`.goreleaser.yml` therefore splits darwin (cgo) from linux/windows (no cgo), and
the release workflow runs on a macOS runner so darwin can be linked against the
Security framework. `vault.TestNativeBackendsIncludesKeychainOnMacOS`, which CI
runs on macOS with cgo enabled, fails if the Keychain backend is not compiled in.

## License

MIT
