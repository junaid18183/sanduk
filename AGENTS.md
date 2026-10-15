# AGENTS.md

Guidance for AI coding agents working in this repository.

## What sanduk is

`sanduk` is a CLI that stores API keys and other secrets in the operating
system's secure keystore and injects them into child processes on demand. It is
modelled on [aws-vault](https://github.com/99designs/aws-vault) but targets
static API keys (OpenAI, DeepSeek, Stripe, …) rather than temporary AWS
credentials. Written in Go, using `spf13/cobra` for the CLI, `99designs/keyring`
for the native backends, and `filippo.io/age` for the encrypted-file backend.

## Repository layout

```
main.go                 entry point; calls cli.Execute()
cli/                    one file per cobra command, registered in cli/root.go
  root.go               root command, --config and --backend flags, config helpers
  input.go              terminal input helpers (echo-off prompt, stdin, confirm)
  util.go               shared helpers (field parsing, metadata recording)
  *_test.go             command-level tests
vault/                  all domain logic; no cobra imports here
  entry.go              Secret/Group entry types and validation
  vault.go              Vault: Get/Add/List/Remove/ResolveEnv
  config.go             non-secret metadata config (~/.config/sanduk/config.toml)
  keyring.go            backend selection
  backend_age.go        age-encrypted file backend
  backend_biometric_*   macOS Touch ID backend (darwin+cgo) and its stub
  exec_unix.go          execve-based secret injection
  exec_windows.go       child-process variant for Windows
  exec_env.go           environment merging
  format.go             shell/dotenv/json output
  dotenv.go             .env parsing
  redact.go, tty.go     display helpers
.github/workflows/      ci.yml (checks) and release.yml (GoReleaser on v* tags)
.goreleaser.yml         release configuration
```

## Common tasks

```sh
make build     # build ./sanduk
make test      # go test ./...
make vet       # go vet ./...
make fmt       # gofmt -l -w .
```

Before pushing, run `gofmt -l .` (must be empty), `go vet ./...`, `go test ./...`,
and a cross-compile check:

```sh
for t in linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64; do
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -o /dev/null .
done
```

## Security rules — these are not optional

- **Never print, paste, log, echo, or commit a secret value.** Use `[REDACTED]`
  or a clearly fake placeholder (`sk-test-value`, `AKIA...`). This applies to
  tool output summaries, commit messages, and examples.
- **Never touch a developer's real keychain while developing or testing.** Tests
  must run against the age/file backend via `SANDUK_BACKEND=file`,
  `SANDUK_FILE_DIR=<temp>`, and `SANDUK_FILE_PASSPHRASE=<fake>`.
- **Never run the `keychain-biometric` backend automatically.** It stores items
  with a user-presence ACL, so any read triggers a Touch ID / login prompt on the
  machine and can block the agent. It is opt-in and manually verified only.
- **Never run `sanduk add`, `get`, or `exec` against a real backend with a real
  secret** as part of exploration. Use temp dirs and fake values.
- Do not add a command that dumps every secret at once.

## Architecture invariants

Break these and you break the tool's security model:

1. **Secrets live only in the backend; metadata lives only in the config.**
   `~/.config/sanduk/config.toml` must never contain a secret value.
2. **`list` must never read secret values.** It combines backend `Keys()` with
   config metadata (entry type, env mapping, description). Adding a value read to
   the list path is a regression.
3. **Secret values are never accepted as command-line arguments** (argv is
   visible in process listings). Input comes from an echo-off prompt, `--from-env`,
   `--from-file`, or stdin.
4. **`get` is terminal-gated; `export` is not.** `get` refuses to write to a
   non-terminal unless `--force` is given. `export` is the sanctioned scripting
   interface and must keep working in pipes.
5. **`exec` replaces the current process (`execve`) on Unix** so secrets exist
   only in the child. Windows has no `execve` and waits on the child instead.
6. **Every backend implements `keyring.Keyring`** (`Get`, `GetMetadata`, `Set`,
   `Remove`, `Keys`). New backends are selected in `vault/keyring.go` from
   `SANDUK_BACKEND` / `--backend`.

## Coding conventions

- Keep `gofmt` and `go vet` clean.
- Domain logic goes in `vault/`; `cli/` files stay thin and cobra-only.
- Wrap errors with `%w`; use the `vault.ErrNotFound` sentinel for missing entries
  so `errors.Is` works across the CLI.
- Data goes to stdout; status/confirmation messages go to stderr (`warnf`).
- Values entering the keychain are validated on write **and** on read
  (`entry.validate()`), so a hand-edited backend can't produce a half-valid entry.
- Tests set configuration through `t.Setenv`; `vault.newFileVault` opens the age
  backend in a `t.TempDir()`.

## Adding a command

1. Add `cli/<name>.go` with a `newXxxCmd() *cobra.Command` constructor.
2. Register it in `cli/root.go`'s `AddCommand` list.
3. Document it in the README command table.
4. Add a test; if it needs a vault, use the age backend.

## Releasing

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs
GoReleaser and publishes darwin/linux/windows (amd64/arm64) archives plus
`checksums.txt`. The version is stamped with `-X .../cli.version` from the tag.
Homebrew publishing is intentionally disabled until a tap token exists — see the
commented block in `.goreleaser.yml`. `dist/` is gitignored; never commit it.
