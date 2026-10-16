# Changelog

sanduk is pre-1.0 (`0.0.x`). A release may change behaviour when that closes a
security hole; each such change is listed under **Breaking**.

## Unreleased

### Security

- **Shell injection through `export`.** Variable names are now validated before
  `export` prints them (shell and dotenv print names unquoted), and before `exec`
  hands them to a child. `ParseDotenv` rejects keys that are not plain
  identifiers, so `import` cannot store a name like `X;curl …|sh;Y`.
- **Loader-variable block was bypassable.** The check now applies to every name
  that reaches a child, not only group fields: flat secrets (`add ld_preload`
  used to become `LD_PRELOAD`), `ENVVAR=entry` bindings, `import`, and
  config.toml `env` mappings. The deny-list now covers the `LD_*`, `DYLD_*`,
  `GIT_CONFIG_*` and `SANDUK_*` families plus `PYTHONPATH`, `PERL5LIB`,
  `JAVA_TOOL_OPTIONS`, `GIT_SSH_COMMAND`, `PROMPT_COMMAND`, `IFS`, `GCONV_PATH`
  and others.
- **Secrets could be split across keystores.** With no backend set, sanduk
  offered every native backend to the keyring library, which silently moves on
  when one fails to open; on Linux a Secret Service hiccup could put new secrets
  in kwallet or pass. Exactly one platform keystore is now used, and failing to
  open it is an error.
- **Project approval read the file twice.** The approval hash is now of the
  exact bytes that were parsed, so swapping `.sanduk.toml` between the parse and
  the check no longer passes. `doctor` now requires approval too.
- **Trust store relocation.** A trust store inside the project it would approve,
  or one readable/writable by group or others, is refused; a repository's
  `.envrc` can no longer point `SANDUK_TRUST` at a file it ships.
- **Planted project files.** A `.sanduk.toml` that is world-writable or owned by
  another user (for example in `/tmp`) is refused.
- **Empty entry names** are rejected. In the `keychain-biometric` backend an
  empty name matched every item, so `remove ""` could delete the whole vault.
  The biometric `Set` now builds the replacement before deleting the old item.
- **Windows:** `SANDUK_*` variables are stripped from the child environment in
  any letter case.
- `--service` / `SANDUK_SERVICE` is validated as a name, so it cannot place the
  age file outside its directory.
- Parse errors no longer echo text that may be part of a secret.
- The age backend confirms the passphrase when the vault file is empty (as when
  it is missing) and warns when the file is readable by group or others.

### Fixed

- A broken config.toml no longer drops `--backend`/`--keychain`/`--service`: a
  write used to land in the platform keystore and then fail on the metadata
  update. Commands that open the vault now refuse up front; `sanduk config`
  still works.
- `--config` now also moves the trust store (`trusted.toml`).
- Concurrent commands no longer lose each other's metadata updates: config
  writes take a cross-process lock.
- The age backend and config lock now work on Windows (`LockFileEx`).
- `remove` cleans up metadata left behind for an entry the backend no longer
  has, and reports a failed config update instead of ignoring it.
- Errors from config, trust store and `.env` parsing name the file.

### Added

- `sanduk doctor --metadata [--fix]` reports and repairs drift between the vault
  and config.toml.
- Releases are built only after the full CI suite passes, with `-trimpath`, and
  without rewriting go.mod at release time. `checksums.txt` is signed with
  keyless cosign (`checksums.txt.sigstore.json`), each archive has an SBOM, and
  GitHub build-provenance attestations are published.
- CI runs on Windows too, plus golangci-lint, govulncheck, a `go mod tidy`
  check and `goreleaser check`. Actions are pinned to commit SHAs and kept
  current by Dependabot.

### Documentation

- README: a threat model; Windows, shell-completion, and backup/migration/
  uninstall sections; release verification; a complete command and global-flag
  reference; corrected sample output for `config` and `doctor`; troubleshooting
  for approval, Secret Service, trust-store and broken-config errors.
- New SECURITY.md (private vulnerability reporting) and CONTRIBUTING.md.
- AGENTS.md describes the current layout and invariants; CLAUDE.md imports it.
- The example project no longer uses a `SANDUK_*` variable of its own
  (`DEMO_ENTRY` instead), and its walkthrough includes `sanduk allow`.

### Dependencies

- `jose2go` 1.5.0 → 1.11.0 (GO-2023-2409; reachable only through the keyring
  library's file backend, which sanduk never selects), `wincred` 1.2.3,
  `pflag` 1.0.10, `x/crypto` 0.57.0.

### Breaking

- Flat `import` refuses to overwrite existing entries unless `--replace` is
  given (it used to warn and overwrite, including whole groups).
- `doctor` exits with status 3 (not 1) when keys are missing, so scripts can
  tell drift from failure.
- Removed deprecated spellings with no replacement period: `add --env` (use
  `--as`), `add-group --from-file` and `group set --from-file` (use
  `--from-dotenv`), `import --shred` (use `--delete`), and the `biometric`
  backend alias (use `keychain-biometric`).
- `get` redacts everything it prints — a flat secret, `--field` and `--all` —
  unless `--reveal` is given. It is for inspection; scripts use `export`.
- `list` shows an entry with no metadata as `unknown` instead of guessing
  `secret`.
- `add`, `add-group` and `import` check for an existing entry by listing names
  instead of reading it, so they no longer decrypt a value (or prompt for
  Touch ID) just to refuse an overwrite.
- `add-group --field KEY=VALUE` is refused; `--field KEY` now prompts for the
  value with echo off. `--from-json` takes a file path or `-` (stdin) instead of
  inline JSON. Values on the command line are visible in `ps`.
- Variable names that change how a child process runs (see above) are refused
  everywhere. An existing entry that uses one still loads, so it can be fixed or
  removed, but `exec`/`export` refuse it until then; `--except NAME` works around
  it in the meantime.
- `.env` files with keys that are not `[A-Za-z_][A-Za-z0-9_]*` no longer parse
  (`import`, `add-group --from-dotenv`, `group set --from-dotenv`, `doctor`
  templates).
- With no backend set, Linux uses the Secret Service only; select `kwallet` or
  `pass` with `--backend`. A failure to open the keystore is now an error.
- `export -o FILE` refuses to replace an existing file unless `--force` is given.
- `export --format dotenv` refuses a value containing `$` together with a single
  quote or a newline, which no dotenv quoting can represent without the reader
  expanding the `$`. Use `--format json` or `exec`.
- `doctor` without an entry argument requires the `.sanduk.toml` to be approved.
- A trust store with mode other than 0600 (group/other bits set) is refused;
  fix with `chmod 600` on it.
