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
  root.go               root command, --config/--backend/--keychain flags, config helpers
  input.go              terminal input helpers (echo-off prompt, stdin, confirm)
  util.go               shared helpers (field parsing, metadata recording)
  selector.go           exec/export selectors (entry or ENVVAR=entry) + --only filter
  project.go            resolves .sanduk.toml into a default entry
  group.go              group set/unset/show/get, the field-level edit path
  get.go                get (--field/--all/--reveal) and the terminal gate
  allow.go              approve a repository's .sanduk.toml
  completion.go         shell-completion helpers (entry-name completion)
  doctor.go             compares a group against .env.example
  settings.go           resolves backend settings (flag > env > config) for every command
  *_test.go             command-level tests
vault/                  all domain logic; no cobra imports here
  entry.go              Secret/Group entry types and validation
  vault.go              Vault: Get/Add/List/Remove/ResolveEnv, group field edits
  config.go             non-secret metadata config (~/.config/sanduk/config.toml)
  project.go            .sanduk.toml: the repo's default entry (non-secret)
  keyring.go            backend selection, keychain target
  backend_age.go        age-encrypted file backend
  backend_biometric_*   macOS Touch ID backend (darwin+cgo) and its stub
  exec_unix.go          execve-based secret injection
  exec_windows.go       child-process variant for Windows
  exec_env.go           environment merging (strips SANDUK_* from the child)
  format.go             shell/dotenv/json output
  dotenv.go             .env parsing (inline comments, multi-line quoted values)
  atomic.go             WriteFileAtomic: temp + fsync + rename
  filelock_unix.go      flock-based cross-process lock (stub on Windows)
  envname.go            environment-variable name validation
  project.go            .sanduk.toml discovery
  trust.go              the per-user project-approval store (trusted.toml)
  redact.go             IsTerminal + Redact display helpers
.github/workflows/      ci.yml (checks) and release.yml (GoReleaser on v* tags)
.goreleaser.yml         release configuration
examples/               config.toml.example, plus project/ — a runnable starter project
```

## Common tasks

```sh
make build      # build ./sanduk
make test       # go test ./...      (make race: with -race)
make cover      # coverage summary
make lint       # golangci-lint, pinned to the CI version
make vuln       # govulncheck, pinned to the CI version
make fmt        # gofmt -l -w .      (make fmt-check: report only)
make check      # everything CI runs: fmt-check tidy-check vet race lint vuln cross
```

Before pushing, run `make check`. Lint the platform-specific files too:
`GOOS=windows golangci-lint run ./...` and `GOOS=linux …` (CI does both).

On macOS, also check the real darwin build (cgo **on**), which is what ships:

```sh
CGO_ENABLED=1 go build -o /dev/null .
go test ./vault -run TestNativeBackendsIncludesKeychainOnMacOS
```

## Environment variables

| Variable | Effect |
| --- | --- |
| `SANDUK_BACKEND` / `--backend` | Backend: `keychain`, `keychain-biometric`, `secret-service`, `wincred`, `kwallet`, `pass`, `file`/`age`. |
| `SANDUK_KEYCHAIN` / `--keychain` | macOS keychain name; default `login`. A name, never a path. |
| `SANDUK_KEYCHAIN_PASSPHRASE` | Password used only when creating a dedicated keychain. |
| `SANDUK_SERVICE` | Keychain service name; default `sanduk`. Also names the age file. |
| `SANDUK_CONFIG` | Metadata config path; default `~/.config/sanduk/config.toml`. |
| `SANDUK_TRUST` | Project-approval store; default `trusted.toml` next to the config. |
| `SANDUK_FILE_DIR` | Directory of the age vault; default `~/.local/share/sanduk`. |
| `SANDUK_FILE_PASSPHRASE` | Passphrase for the age backend (non-interactive). |
| `./.sanduk.toml` | Committed project file: `entry` and optional `only`. Non-secret. |

`backend`, `keychain` and `service` may also be set in the metadata config. They
are defaults only: the precedence is always **flag > env > config > platform
default**, and `sanduk config` prints the effective value with its source.

## Smoke test

`examples/project` is a runnable end-to-end check of the CLI. Drive it against
the age backend so it never touches a real keychain:

```sh
make build                       # ./sanduk at the repo root
cd examples/project
export SANDUK=$PWD/../../sanduk
export SANDUK_BACKEND=file SANDUK_FILE_PASSPHRASE=fake \
       SANDUK_FILE_DIR="$(mktemp -d)" SANDUK_CONFIG="$(mktemp -d)/config.toml"
make setup dev                   # doctor fails here by design: demo.env omits LEGACY_TOKEN
make add-legacy
make doctor                      # now clean
make clean
```

It stores a group literally named `demo`, so `make clean` removes it again.

## Security rules — these are not optional

- **Never print, paste, log, echo, or commit a secret value.** Use `[REDACTED]`
  or a clearly fake placeholder (`sk-test-value`, `AKIA...`). This applies to
  tool output summaries, commit messages, and examples.
- **Never touch a developer's real keychain while developing or testing.** Tests
  must run against the age/file backend via `SANDUK_BACKEND=file`,
  `SANDUK_FILE_DIR=<temp>`, and `SANDUK_FILE_PASSPHRASE=<fake>`. Set
  `SANDUK_CONFIG=<temp>` too, so tests never write the real metadata config.
- **Never run the `keychain-biometric` backend automatically.** It stores items
  with a user-presence ACL, so any read triggers a Touch ID / login prompt on the
  machine and can block the agent. It is opt-in and manually verified only.
- **Never run `sanduk add`, `get`, or `exec` against a real backend with a real
  secret** as part of exploration. Use temp dirs and fake values.
- Do not add a command that dumps every secret at once.

## Architecture invariants

Break these and you break the tool's security model:

1. **Secrets live only in the backend; metadata lives only in the config.**
   `~/.config/sanduk/config.toml` (and the `trusted.toml` trust store) must never
   contain a secret value.
2. **`list` must never read secret values.** It combines backend `Keys()` with
   config metadata (entry type, env mapping, description). Adding a value read to
   the list path is a regression.
3. **Secret values are never accepted as command-line arguments** (argv is
   visible in process listings). Input comes from an echo-off prompt, `--from-env`,
   `--from-file`, or stdin. `group set` deliberately has no VALUE position for
   this reason, and `add-group --field KEY=VALUE` / inline `--from-json` are
   refused (`--field KEY` prompts; `--from-json` takes a file or `-`).
4. **`get` is terminal-gated; `export` is not.** `get` refuses to write to a
   non-terminal unless `--force` is given. `export` is the sanctioned scripting
   interface and must keep working in pipes.
5. **`exec` replaces the current process (`execve`) on Unix** so secrets exist
   only in the child. Windows has no `execve` and waits on the child instead.
6. **Every backend implements `keyring.Keyring`** (`Get`, `GetMetadata`, `Set`,
   `Remove`, `Keys`). New backends are selected in `vault/keyring.go`
   (`openKeyring`) from `vault.Options.Backend`, which the CLI resolves from
   `--backend`, `SANDUK_BACKEND` or the config file.
7. **macOS binaries must be built with `CGO_ENABLED=1`.** Both the Keychain
   backend (`99designs/keyring`, tag `darwin && cgo`) and the
   `keychain-biometric` backend require cgo. A `CGO_ENABLED=0` macOS build has no
   OS keystore at all. `.goreleaser.yml` keeps darwin (cgo) separate from
   linux/windows and the release workflow runs on macOS for this reason.
   `vault.TestNativeBackendsIncludesKeychainOnMacOS` guards it.
8. **Never let the library's own file backend be selectable.** Its opener cannot
   fail, so it shadows a missing native keystore and then errors on the first
   read with "No directory provided for file keyring". `nativeBackends()`
   filters it out. With no backend set, `defaultNativeBackend()` offers
   `keyring.Open` **exactly one** backend per OS (keychain / wincred /
   secret-service): `Open` silently tries the next allowed backend on failure,
   so offering several would split the vault across stores. Only when that one
   is not compiled in does sanduk fall back to its own age backend.
9. **`.sanduk.toml` is non-secret, and only ever names an entry.** It is
   committed to repositories, so it must never grow a value field. It supplies
   the default entry plus an optional key subset (`only`), nothing more.
10. **Selectors resolve left to right and the last one wins**, in both `exec` and
    `export`; an entry name resolves through its own mapping, and `ENVVAR=entry`
    binds one flat secret. `--only` filters the merged result and errors on an
    unknown key rather than silently injecting nothing.
11. **A dedicated keychain is opt-in.** `--keychain` / `SANDUK_KEYCHAIN` accepts a
    name, never a path (`keychainTarget()` rejects separators). The default stays
    `login`; see the README for why a second keychain is not automatically safer.
12. **Config-file backend settings are defaults, never overrides.** Precedence is
    flag > env > config > platform default, resolved once (`currentSettings()` in
    `cli/settings.go`, primed by `PersistentPreRunE`) and passed to the vault as
    `vault.Options` via `openVault()` — never exported back into the environment.
    Commands open the vault with `openVault()`, not `vault.New()` (which reads
    only the environment and exists for tests). The config and trust paths come
    from `configPath()`/`trustPath()`, so `--config` moves both. A broken config
    still resolves flags and env (so `--backend` is honoured), lands in
    `settingsErr`, and makes `openVault()` refuse — but `sanduk config`, which
    does not open the vault, keeps working so you can inspect it. Config
    read-modify-writes go through `vault.UpdateConfig`, which holds a
    cross-process lock.
13. **`exec` never hands the child a `SANDUK_*` variable.** `buildEnv` drops the
    prefix from the inherited environment, because `SANDUK_FILE_PASSPHRASE` and
    `SANDUK_KEYCHAIN_PASSPHRASE` each unlock the whole vault; leaking one would
    defeat `--only`. Overrides may still set any name (that is an explicit
    injection), and the output order is sorted, not map-ordered.
14. **`.sanduk.toml` is untrusted until approved.** `defaultEntry` refuses a
    project file that is not in the trust store with a matching content hash, so
    a cloned repository cannot choose which entry `exec`/`export`/`doctor` uses.
    The hash is of the bytes `LoadProject` parsed (`Project.Hash()`), never a
    second read. The store is `trusted.toml` (non-secret, mode 0600); a store
    inside the project, or one readable by group/other, is refused, since
    `SANDUK_TRUST`/`SANDUK_CONFIG` can move it.
15. **Destructive writes are never the default.** `add-group` and `import` (flat
    or `--group`) refuse to overwrite existing entries without `--replace`; `add` refuses to
    turn a group into a flat secret without `--replace`; `remove` requires `--yes`
    when stdin is not a terminal. Failing closed matters more than convenience.
16. **Every variable name is validated on write and again on output**
    (`vault.ValidateEnvName`): in `vault.Exec` and `vault.FormatEnv`, which every
    name passes through after `--only`/`--except`. That covers group fields, flat
    secret names, `ENVVAR=entry` bindings and config.toml mappings, and stops
    shell injection through `export`, which prints names unquoted. `ParseDotenv`
    rejects non-identifier keys. Don't add an output path that bypasses those
    two functions. Entry names are checked by `vault.ValidateEntryName` (an empty
    name matches every item in the biometric backend). File writes go through
    `vault.WriteFileAtomic`, and the age backend takes a cross-process lock
    (`filelock_*.go`) around read-modify-write.

## Coding conventions

- Keep `gofmt` and `go vet` clean.
- Domain logic goes in `vault/`; `cli/` files stay thin and cobra-only.
- Wrap errors with `%w`; use the `vault.ErrNotFound` sentinel for missing entries
  so `errors.Is` works across the CLI.
- Data goes to stdout; status/confirmation messages go to stderr (`warnf`).
- Values entering the keychain are validated on write **and** on read
  (`entry.validate()`), so a hand-edited backend can't produce a half-valid entry.
- Tests set configuration through `t.Setenv`; `vault.newFileVault` opens the age
  backend in a `t.TempDir()`. Both packages have a `TestMain` that points every
  `SANDUK_*` setting at a throwaway age vault before any test runs, and calls
  `vault.UseFastScryptForTests()` (which panics outside `go test`). In `cli`,
  drive commands through `runCmd(t, stdin, args...)` after `testEnv(t)` (see
  `cli/cmd_test.go`); it captures stdout through `cmd.OutOrStdout()`.
- CI pins every action to a commit SHA and every tool to a version (see the
  `env:` block in ci.yml). When bumping one, pick a release at least two weeks
  old; Dependabot enforces the same cooldown.

## Adding a command

1. Add `cli/<name>.go` with a `newXxxCmd() *cobra.Command` constructor.
2. Register it in `cli/root.go`'s `AddCommand` list.
3. Document it in the README command table.
4. Add a test; if it needs a vault, use the age backend.

## Releasing

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs
GoReleaser and publishes darwin/linux/windows (amd64/arm64) archives plus
`checksums.txt`. The version is stamped with `-X .../cli.version` from the tag.

**Versioning: stay on `0.0.x`.** The next release is always the next
`0.0.X` increment (`v0.0.1` → `v0.0.2` → …) unless the user asks for a specific
version. Do not promote to `0.1.0`, `1.0.0`, or any other number on your own,
even for a feature release that would conventionally warrant a minor bump.

Homebrew publishing is intentionally disabled until a tap token exists — see the
commented block in `.goreleaser.yml`. `dist/` is gitignored; never commit it.
