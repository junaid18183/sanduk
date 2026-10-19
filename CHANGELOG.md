# Changelog

sanduk is pre-1.0 (`0.0.x`). A release may change behaviour when that closes a
security hole; each such change is listed under **Breaking**.

## 0.0.6 — 2026-10-19

Five additions, all of which keep the existing rule that a value leaves the
vault only into the process that needs it.

### Added

- **Rotation deadlines.** `--expires YYYY-MM-DD` on `add`, `add-group`,
  `add-file`, `group set` and `rotate` records when a credential should be
  replaced; `--expires none` clears it. `list` marks each entry and says
  `[EXPIRED …]` once a date has passed. `sanduk doctor --expiry [--within N]`
  lists deadlines soonest first and exits 3 when any has passed or is due,
  reading metadata only — no value is decrypted, so it is safe in CI and never
  prompts on a presence-gated backend. A deadline is advisory: sanduk will not
  refuse to inject an expired secret.
- **`sanduk scan` — find copies that escaped the vault.** Searches the working
  tree, and with `--git` everything git has ever recorded, and with `--shell`
  your shell history, for stored values. A finding is an entry name and a
  location, never the value, so the report is safe to paste. Exits 3 on a find.
  This is the one command that reads every value, so it confirms first and
  needs `--yes` without a terminal. Values are matched in-process and never
  handed to `grep`/`git grep` as an argument, where argv would expose them to
  every process on the machine.
- **File secrets.** `sanduk add-file <name> --from-file F --as VAR` stores a
  credential file — a service-account JSON, a `.p8` key, a certificate — and
  `exec` gives the child a path to it. On Unix the bytes go to a temporary file
  that is unlinked at once and passed as `/dev/fd/N` for the inherited
  descriptor, so nothing readable is left on disk and the data is freed when
  the child exits. `--materialize DIR` writes real files (mode `0600` on Unix)
  for tools that need a path with an extension; those persist. `export --materialize DIR`
  writes them out; `get` reports size and filename and never prints the bytes,
  not even with `--reveal`.
- **Import from a credential manager.** `import` now reads CSV exports and
  1Password `.1pux` archives as well as `.env` files, chosen by extension or
  `--format`. For CSV the header picks the name and value columns, and a
  `fields` column of `label: value` lines contributes one entry each. From a
  `.1pux` only secret-bearing fields are taken — a login's password and
  concealed custom fields — leaving usernames, URLs, notes and attachments
  behind.
- **`sanduk mcp` — an agent can see what exists without seeing a secret.** A
  read-only Model Context Protocol server over stdin/stdout with three tools:
  `list_entries`, `describe_entry` and `check_expiry`. None returns a value and
  none runs a command; the agent is told to write `sanduk exec` itself, so the
  approval that already governs its shell still applies.

### Changed

- `vault.Exec` takes an `ExecOptions` instead of a bare environment map, so it
  can materialize file secrets. `vault.ResolveEnv` now refuses a file secret
  and points at `ResolveEntry`, which returns payloads alongside variables.
- `--only` and `--except` filter file secrets too; both name variables either
  way.
- `list` shows `file` as a kind, and `doctor --metadata --fix` records it.
- `config.toml` gains a `[files]` table and an `expires` key per entry. Both
  are metadata: no secret value is ever written there.
- `scan` opens files through an `os.Root` anchored at the scan root, so a
  symlink cannot walk it out of the tree it was given.

## 0.0.5 — 2026-10-19

### Added

- **Group links: share one secret across groups without copying it.**
  `sanduk group link <group> [VAR=]<secret>` points a group field at an existing
  flat secret. The group stores the secret's *name*, never its value, and
  resolves it on every read — so `sanduk rotate elevenlabs` updates every group
  that links it, with no re-sync step. A bare secret name uses the variable that
  secret is already exposed as (`--as`, or the upper-cased name).
  `sanduk group unlink <group> KEY…` removes the pointer and leaves the secret
  untouched.
- A link may only address a flat secret, so a cycle is impossible by
  construction, and a variable is either stored in the group or linked, never
  both — there is no precedence rule to remember.
- A dangling link (target removed, or no longer a flat secret) fails closed at
  resolve time with a message naming the group, field and target, rather than
  leaving the variable silently absent.
- `sanduk doctor --links` checks every link in the vault and exits 3 if any
  dangles.
- `sanduk group show --long` shows which fields are links and what they point
  at. The default output is unchanged — one bare name per line — so it keeps
  piping into `--only` and `xargs`.
- `get --field`, `get --all` and `group get` resolve links; `--field` resolves
  only the field you asked for, so reviewing one secret does not read the rest
  of the group.

### Changed

- `doctor <group> --template` now counts a linked field as present. Without this
  every link would be reported as a missing template key.
- `keychain-biometric` reads within one command now share a single `LAContext`
  with a 10-second reuse window, so resolving a group and the secrets it links
  prompts once rather than once per read. The window is per process and dies
  with the command. **Unverified against a live biometric prompt** — like the
  rest of that backend, it is experimental and compile-checked only.
- Link targets are read once per distinct secret, so a group linking the same
  secret under two names costs one read.

## 0.0.4 — 2026-10-19

The first release after a full security, architecture and documentation audit.
Several changes are breaking; sanduk is pre-1.0 and fails closed by preference.

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

- Require Go 1.26.3, the first patch without GO-2026-4971 (a `net` panic on
  Windows reachable through the keyring library). CI builds and tests with the
  newest 1.26 patch so later stdlib fixes land without a go.mod change.

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
- `add-group` and `import` check for an existing entry by listing names instead
  of reading it, so they no longer decrypt a value (or prompt for Touch ID) just
  to refuse an overwrite. `add` still reads an existing entry once, to confirm
  from the backend (not possibly-drifted metadata) that it is a flat secret.
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
- `export -o FILE` refuses to replace an existing file unless `--force` is given,
  and if FILE is a symlink it is replaced by a regular file, not written through.
- `export --format dotenv` refuses a value containing `$` together with a single
  quote or a newline, which no dotenv quoting can represent without the reader
  expanding the `$`. Use `--format json` or `exec`.
- `doctor` without an entry argument requires the `.sanduk.toml` to be approved.
- A trust store with mode other than 0600 (group/other bits set), or owned by
  another user, is refused; fix with `chmod 600` on it. Root may use a user's
  files.
