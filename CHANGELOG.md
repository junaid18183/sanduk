# Changelog

sanduk is pre-1.0 (`0.0.x`). A release may change behaviour when that closes a
security hole; each such change is listed under **Breaking**.

## 0.0.8 — 2026-10-26

Secrets are shared down a directory tree, not just inside one repository: the
token every checkout of yours uses, the one an organisation's repositories use
instead, and the repository's own entry are three statements about three
directories, and sanduk now reads all three.

### Added

- **A `.sanduk.toml` applies to its directory and everything below it.** The
  files nest: `~/code`, `~/code/acme` and `~/code/acme/bidops` each name an
  entry, and a command in the repository resolves all three, outermost first,
  so the nearest directory wins on a variable two of them provide — the same
  left-to-right rule as `sanduk exec a b c`. Each file's `only` narrows its
  own entry and nothing else. None of the directories has to be a git
  repository.
- **`inherit = false`** seals a directory: the files above it are ignored, so
  a repository that must resolve the same way in a clone and in CI depends on
  its own file alone.
- **`sanduk doctor --chain`** reports the files that apply here, which are
  approved, and which variables come from *outside* this directory — the ones
  a clone or a CI job will not have, since those files are not in the
  repository. Exits 3 when a layer is not approved.
- **`sanduk list` shows the chain**, with the entry each variable comes from
  and what it shadows, so "which `GITHUB_TOKEN` do I get here" is answerable
  by looking.
- **`sanduk allow` names the files above it** that still need approving, with
  the command for each, instead of letting you find them one failed command at
  a time. Each file is still approved on its own.

### Changed

- **Inside a project, `sanduk list` lists the chain and nothing else.** The
  vault's global entries are a different question, and printing a machine's
  worth of them under the three keys the directory uses is what the scope was
  introduced to stop. Outside a project the global table is unchanged. The
  hidden count still goes to stderr, and `--all` / `--scope global` still show
  everything.
- **`sanduk group list` puts the groups that apply where you are standing
  first**, then the directories above and below you, then the rest
  alphabetically, and shortens `$HOME` to `~`.
- An entry is read at most once per command, however many layers include it:
  on a presence-gated backend that is one prompt, not one per layer.

### Breaking

- **An unapproved project file above the working directory now fails `exec`,
  `export` and `doctor`**, where before only the nearest file was consulted
  and the ones above it were never read. Approve each with
  `sanduk allow <dir>`, or set `inherit = false`. A listing never fails: it
  says which layer is unapproved and shows the vault as if the file were not
  there.
- **`sanduk list --json` replaces `project.entry`/`project.path` with
  `project.dir` and a `project.layers` array**, and each key now carries
  `from` and `shadows`.

## 0.0.7 — 2026-10-26

A vault that is used grows a long tail: entries that belong to one repository,
and flat secrets that exist only to feed a group. This release teaches sanduk
which entries belong where, so a listing shows what you are standing in, and
makes setting a repository up a single command.

### Added

- **`sanduk init`.** Declare a repository before it has any secrets: it writes
  `.sanduk.toml`, approves it, and records the directory against the group.
  The entry does not have to exist yet. An existing file is never replaced
  without `--force`.
- **`sanduk init --from-template`** is the way into a project somebody else
  set up. It reads the key names from a committed `.env.example` or
  `.env.template` and prompts for each value with echo off. The placeholders
  in the file are never stored, an empty answer skips a key for `sanduk
  doctor` to report, and the keys entered become the `only` list. `doctor`
  now also looks for `.env.template` before asking for `--template`.
- **`sanduk init --envrc`** writes a direnv script for the project, with the
  warning that belongs beside it. Off by default: direnv exports into the
  shell, so everything started from that shell inherits the values, and it
  re-runs the export on every load.
- **A group belongs to a directory.** `sanduk add-group`, run at a repository
  root, records that directory as the group's `location` in `config.toml` and
  writes and approves a `.sanduk.toml` naming it, unless one already exists.
  `sanduk allow` records the location too. The location lives in your own
  private config rather than in the repository, so no cloned repository can
  claim a group by shipping a file; a project file still needs approving, and
  still wins when the two disagree. `--no-project` creates a plain global
  group. A location that no longer exists — deleted, moved, or never cloned
  on this machine — claims nothing and the group is global again, marked
  `(missing)` by `group list` and reported by `doctor --metadata`.
- **`include` in `.sanduk.toml`** names the global secrets and groups a
  repository also needs. They resolve before the project's own entry, in the
  order given, so the project wins on a variable two of them both provide —
  the same rule as `sanduk exec a b c`. Still names only, still approved as
  one file, and nothing is injected anywhere without a project file asking
  for it.
- **`only` keeps itself current.** Once a project file lists the keys it
  expects, `group set`, `group unset`, `group link` and `group unlink` add
  and remove them in the file and re-approve it. Only that assignment is
  rewritten, so comments survive; a file with no `only` is never given one,
  and an unapproved file is never touched. Because the list tracks the
  project's own group, it narrows that entry alone and not the entries the
  file includes — filtering those with it would drop everything they
  contribute the first time it synced. The `--only` flag is unchanged and
  still narrows the whole selection.
- **Scoped listings.** `sanduk list` shows the entries in scope: inside a
  repository, that repository's entry expanded into the variables it
  provides, then the global entries. Left out by default are entries
  belonging to a repository (yours or another one) and flat secrets a group
  links to, which already show as the `SOURCE` of the variable they supply.
  The count of hidden entries always goes to stderr, `--all` adds a `SCOPE`
  column and shows everything, and `--scope project|member|global` shows one
  kind. Nothing about resolution changed: `exec` and `export` inject exactly
  what they injected before.
- **`sanduk group list`.** Every group with the directory it belongs to,
  built from metadata alone — no entry is opened. `--here` narrows to the
  group of the working directory, `--keys` adds the variables each group
  provides (that one does open every group), plus `--json` and `--names`.
- **Scope in the MCP server.** `list_entries` labels each entry `project`,
  `member` or `global` and marks the one the working directory resolves to,
  so an agent can tell this repository's secrets from every other
  repository's. It still returns every entry, and still no value.
- **`sanduk doctor --links --fix`** rebuilds the `linked_by` hints in
  `config.toml` from the links the backend actually holds. Those hints only
  group related entries in a listing; nothing resolves through them, so a
  stale one can misplace a row and nothing more.

### Fixed

- macOS keychain items sanduk writes now list sanduk in their ACL
  (`KeychainTrustApplication`), so reading one back does not ask the user to
  approve sanduk again. A prompt is per item, so resolving a group with three
  links asked four times. This applies to items written from now on; "Always
  Allow" does the same for existing ones. The trust is tied to the binary's
  signature, so an unsigned rebuild is still a new application.
- `sanduk group show` and `sanduk doctor --links` read each entry once instead
  of twice. On a backend that asks per read — a dedicated keychain, or
  `keychain-biometric` — the second read was a second password or Touch ID
  prompt for an entry sanduk had just read.

### Breaking

- `sanduk list` no longer prints every entry by default, and `--json` now
  emits an object (`project`, `entries`, `hidden`) instead of a bare array,
  with a `scope` on each entry. `sanduk list --all --json` is the closest
  equivalent of the old output.

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
