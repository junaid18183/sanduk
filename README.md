# sanduk

**sanduk** (Hindi/Urdu for *box* / *chest*) is a small vault for API keys and
other development secrets. It stores secrets in your operating system's secure
keystore and exposes them to the processes that need them — on demand, without
writing plaintext secrets to disk.

![sanduk storing a key, injecting it into one process, linking it into a group, and finding a leaked copy](docs/demo.gif)

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
sanduk import .env.warden --group warden

sanduk list          # the entries in scope here; --scope all for every one
```

In a repository, set it up in one step instead — including one somebody else
wrote, whose keys you fill in from its committed template:

```sh
cd ~/work/warden && sanduk init --from-template
```

> `add-group --field KEY` prompts for that field's value with echo off. Values
> are never taken from the command line, where `ps` would show them.
>
> An existing entry is never replaced silently: `add-group` asks you to use
> `group add` (to add fields) or to pass `--replace` (to overwrite). `add` likewise
> refuses an existing secret and points at `rotate`, which changes its value.

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

### Share one secret across groups

A key you use in several projects should live in one place. Link it into a group
instead of storing a second copy:

```sh
# the shared secret, stored once
sanduk add openai --as OPENAI_API_KEY

# point a project group at it — nothing is copied
sanduk group link -g warden openai
# Linked OPENAI_API_KEY in group "warden" -> secret "openai"

# or bind it under a different variable name in this group
sanduk group link -g warden LLM_KEY=openai
```

A bare secret name uses the variable that secret is already exposed as — its
`--as` mapping, the same name `sanduk export openai` prints — so the common
case takes no second argument.

Nothing is read into the group: the link stores the *name* `openai`, and the
value is fetched when the group is resolved. So a rotation needs no re-sync:

```sh
sanduk rotate openai

sanduk export warden --format dotenv
# OPENAI_API_KEY=<the new value>
# DB_URL=postgres://…
```

`group show` lists a linked field like any other, because a consumer of that list
cares which variables the group contributes, not where each value is kept. Pass
`--long` to see the difference:

```sh
sanduk group show -g warden --long
# KEY                   SOURCE
# OPENAI_API_KEY    -> openai
# DB_URL         stored
```

Deleting the target does not corrupt the group, but it fails loudly at use
rather than quietly injecting nothing:

```sh
sanduk remove openai --yes
sanduk exec warden -- ./server
# sanduk: group "warden": field OPENAI_API_KEY links to "openai", which is not in the vault
```

`sanduk doctor --links` finds that before a deploy does, and exits 3 if any link
dangles. `sanduk group unlink -g warden OPENAI_API_KEY` drops the pointer and
leaves the secret itself untouched.

Two rules keep links predictable:

- **A link points at a flat secret or at a stored field of another group, and
  nothing else.** A field link may only name a value that group stores itself,
  never one of its links, so every link is one hop from its value and a cycle is
  impossible by construction rather than by a depth limit.
- **A variable is either stored in the group or linked, never both.** There is no
  precedence rule to remember; `group link` refuses a name the group already
  stores, and tells you to `group remove` it first or link under another name.

Links live in the vault entry, not in `config.toml`, so a group stays
self-contained in the backend: losing or moving the metadata config can never
change which variables an entry injects.

#### Which way to share

Three mechanisms overlap here, and the one to reach for depends on what you are
sharing rather than on where it is going:

| You want | Use | Notes |
| --- | --- | --- |
| A flat secret in several groups | `sanduk group link -g <group> <secret>` | Creates the group if it does not exist yet. The secret stays the single source of truth, and drops out of the top-level listing as that group's building block. |
| One field of a group, under another group | `sanduk group link -g <group> VAR=<Group>/<FIELD>` | The field stays stored in its group, which is the single source of truth. Nothing is copied, and the variable name is required. |
| A whole global group underneath one repository | `include = ["<entry>"]` in `.sanduk.toml` | Resolved before the project's own entry, so the project wins a variable both provide. Nothing is copied. |
| The same thing in every repository under a directory | A `.sanduk.toml` in that directory | [Sharing down a directory tree](#sharing-down-a-directory-tree). The nearest directory wins. |

A link to a whole group is still refused, because a group is an aggregate and
not a value:

```sh
$ sanduk group link -g bidops opencode
sanduk: "opencode" is a group; a link must point at a flat secret or a Group/FIELD
```

To take one key out of another group, name its field. A group keeping every
provider key in one place can then feed a project's variable directly:

```sh
$ sanduk group link -g maktub ELEVENLABS_API_KEY=ElevenLabs/elevenlabs_siddik
```

The variable name is required, since a field has no secret name to take it
from. The key stays in `ElevenLabs`, so rotating it there changes what `maktub`
injects. A flat secret is still the shared building block when the same value
is wanted by several groups, because a single `sanduk rotate` covers every
group using it. `include` takes the group whole, and a project file's `only`
narrows its own entry alone, so it
cannot be used to thin an include down to one key.

### Credential files

Some tools will not take a secret in a variable — they want a path to a file: a
service-account JSON, an App Store Connect `.p8` key, a client certificate.

```sh
sanduk add-file gcp --from-file sa.json --as GOOGLE_APPLICATION_CREDENTIALS
sanduk exec gcp -- gcloud storage ls
```

The bytes live in the vault like any other secret. What the child gets is a
path, but **not to a file anyone else can open**: sanduk writes the bytes to a
temporary file, unlinks it immediately, and hands the child `/dev/fd/N` for the
descriptor it inherits. The data has no name on disk, and the kernel frees it
when the child exits.

```sh
sanduk exec gcp -- sh -c 'cat "$GOOGLE_APPLICATION_CREDENTIALS"'
# reads /dev/fd/3 — which exists only for that process
```

For a tool that insists on a real path with an extension, `--materialize DIR`
writes real files there instead, mode `0600` on Unix. Those persist: `exec`
replaces the sanduk process, so there is nobody left to delete them.

`get` describes a file secret — its size and filename — and never prints the
bytes, whatever flags you pass. To get the contents back out, use
`sanduk export <name> --materialize DIR`.

> File secrets cannot be the target of `group link`: a link resolves to a value,
> and a file secret has a path instead. Bind it alongside the group instead —
> `sanduk exec warden gcp -- ...`.

### Rotation deadlines

Record when a credential should be replaced, and let a check tell you rather
than remembering:

```sh
sanduk add stripe --as STRIPE_KEY --expires 2027-04-01
sanduk rotate stripe --expires 2028-04-01     # move it when you rotate
sanduk rotate stripe --expires none           # or drop the deadline
```

`list` marks it, and overdue entries say so rather than printing a date you have
to compare against today:

```
NAME     TYPE    ENV / INFO
stripe   secret  STRIPE_KEY  [expires 2027-04-01]
legacy   secret  LEGACY_KEY  [EXPIRED 2026-01-09]
```

`sanduk doctor --expiry` lists them soonest first and exits 3 if anything has
passed or falls due within `--within` days (default 14), so it drops into CI:

```sh
sanduk doctor --expiry --within 30
```

It is metadata only — no value is decrypted — so it is safe to run unattended,
and it never prompts on a presence-gated backend. A deadline is a reminder, not
an enforcement: sanduk will not refuse to inject an expired secret, because
failing a deploy over a date in a config file is worse than the stale key it was
warning about.

### Project workflow

Start from the repository, before it has any secrets:

```sh
cd ~/work/warden
sanduk init                              # entry named after the directory
sanduk init warden --from-dotenv .env    # or name it, and seed it now
sanduk group add STRIPE_KEY              # fill it in afterwards: -g defaults to warden here
```

`sanduk init` writes `.sanduk.toml`, approves it, and records the directory
against the group. The entry does not have to exist yet.

**Joining a project that already exists** is the common case, and most of them
commit a key list. Point `init` at it and answer the prompts:

```sh
git clone https://github.com/someone/their-app && cd their-app
sanduk init --from-template
# .env.example lists 7 key(s); enter each value, or press enter to skip it.
# DATABASE_URL:
# STRIPE_SECRET_KEY:
# …
```

It reads the key names from `.env.example` (or `.env.template`) and prompts for
each value with echo off. The placeholders in the file are never stored, an
empty answer skips a key, and the keys you fill in become the `only` list.
Whatever you skipped, `sanduk doctor` reports as missing until you come back
to it:

```sh
$ sanduk doctor
KEY                STATUS
DATABASE_URL       ok
STRIPE_SECRET_KEY  missing
```

Or create the group first — from inside a repository it becomes that
repository's group:

```sh
cd ~/work/warden
sanduk init warden --from-dotenv .env
# Added group "warden" with 4 field(s)
# Recorded "warden" as the group for /Users/you/work/warden
# Wrote and approved .sanduk.toml — commit it: it names the entry, never a value
```

Two things were recorded, and they are different statements:

- **`.sanduk.toml`**, in the repository, names the entry for everyone who
  clones it. It is non-secret and meant to be committed. Add `only` to narrow
  it to the keys the project normally needs:

  ```toml
  # .sanduk.toml — non-secret, safe to commit
  entry = "warden"
  only  = ["DB_URL", "API_KEY"]
  ```

- **`location`**, in your own `config.toml`, notes which directory the group
  belongs to. It is private to your machine, it holds no value, and it is what
  keeps this group out of listings made anywhere else:

  ```toml
  [groups.warden]
    description = "Warden staging"
    location    = "/Users/you/work/warden"
  ```

`sanduk init` does this, and never overwrites an existing `.sanduk.toml`.
`add-group` does the same at a repository root or in a folder with no `.git`;
its `--global` flag creates a plain global group instead.

#### Layering a global entry underneath

A repository usually needs a few things that are not its own: a shared
provider key, a group of tool credentials. `include` names them, and they
resolve **before** the project's entry, so the project wins on a variable two
of them both provide — the same left-to-right, last-wins rule as
`sanduk exec openai agents warden`:

```toml
# .sanduk.toml
include = ["openai", "agents"]   # global flat secrets and groups, underneath
entry   = "warden"               # this repository's own group, on top
only    = ["DB_URL", "API_KEY", "OPENAI_API_KEY"]
```

```sh
sanduk init warden --include openai,agents
```

This is the layering direnv gives you, and it is declared in the repository
for the same reason: a global config that silently added secrets to every
`sanduk exec` anywhere would be the opposite of least privilege. `include`
names entries, never values, and the whole file is approved as one — a
repository cannot widen what it receives without the change showing up in
`sanduk allow`.

#### Sharing down a directory tree

A project file applies to its directory **and everything below it**, and the
files nest. Keep all your code under one directory and the layers fall out of
the layout you already have:

```
~/work/git/.sanduk.toml                  entry = "code"     GITHUB_TOKEN (personal), AI keys
~/work/git/acme/.sanduk.toml             entry = "acme"     GITHUB_TOKEN (work)
~/work/git/acme/bidops/.sanduk.toml      entry = "bidops"   DATABASE_URL, KUBECONFIG
```

```sh
$ cd ~/work/git/acme/bidops && sanduk export --format dotenv
ANTHROPIC_API_KEY=…      # from code
DATABASE_URL=…           # from bidops
GITHUB_TOKEN=…           # from acme — the nearer directory wins
KUBECONFIG=…             # from bidops
OPENAI_API_KEY=…         # from code
```

They resolve outermost first, so the nearest directory has the last word: the
same left-to-right, last-wins rule as `sanduk exec a b c`, applied down the
tree. Each file's `only` narrows its own entry and nothing else — a
repository's list must not silently delete what the directories above it
share. None of the directories has to be a git repository, and `sanduk init`
works in any of them.

**Every layer is approved separately.** A directory you cloned into can hold a
project file just as a repository can, so `exec` and `export` refuse the whole
chain until each file is approved, naming the one to look at:

```sh
$ sanduk allow
Approved ~/work/git/acme/bidops/.sanduk.toml
note: ~/work/git/.sanduk.toml also applies here and is not approved; review it,
      then run "sanduk allow ~/work/git"
```

**What a layer above the repository gives you, a clone does not.** Those files
are not in the repository, so `sanduk exec -- ./deploy` can work on your
machine and fail in CI. `sanduk doctor --chain` names exactly which variables
that applies to:

```sh
$ sanduk doctor --chain
ENTRY   FROM            STATUS
code    ~/work/git      outside this directory
acme    …/acme          outside this directory
bidops  …/bidops        applies here

4 variable(s) come from outside ~/work/git/acme/bidops, so a clone or CI will not have them:
  ANTHROPIC_API_KEY
  GITHUB_TOKEN
  NPM_TOKEN
  OPENAI_API_KEY
```

A repository that must resolve the same way everywhere opts out, and then
depends on its own file alone:

```toml
# .sanduk.toml
entry   = "bidops"
inherit = false     # ignore every project file above this directory
```

#### `only` keeps itself current

`only` records the keys the project expects, and `--only` errors on a key the
entry does not have, so a typo fails loudly. Once the list exists, sanduk
keeps it in step: `group add`, `group remove`, `group link` and `group unlink`
add and remove keys in the file and re-approve it, preserving your comments.

Delete the line to opt out for good — sanduk keeps such a list current, it
never starts one.

Because the list tracks the project's own group, it does **not** narrow what
`include` brings in; those entries were named in the same reviewed file and
pass through in full. `--only` on the command line is one explicit request,
so it still narrows everything.

For a repository you cloned rather than set up, the project file arrives with
it — and is **not trusted automatically**, because a cloned repository could
otherwise name any entry (say `aws-prod`) and have its scripts receive those
secrets. Review it, then approve it (which also records the location):

```sh
sanduk allow
```

Approval is a hash of the file, so re-approve after any edit. Either way, the
entry can now be omitted, from any subdirectory:

```sh
sanduk exec -- pnpm dev
sanduk export --format dotenv
```

Pair that with a committed `.env.example` or `.env.template` (the keys, with
dummy values) and `sanduk doctor` reports drift between what the app expects and what is stored:

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

### What `list` shows

A vault that is used grows a long tail, and most of it is not what you are
looking at right now. `sanduk list` shows the entries that matter where you
are standing:

```sh
$ cd ~/work/warden && sanduk list
PROJECT  warden (group)  /Users/you/work/warden
KEY             SOURCE
API_KEY         stored
DB_URL          stored
OPENAI_API_KEY  -> openai

NAME        TYPE    ENV / INFO
elevenlabs  secret  ELEVENLABS_API_KEY
groq        secret  GROQ_API_KEY
sanduk: 14 entry/entries hidden (9 member, 5 project) — see "sanduk list --scope all"
```

Inside a project that is the whole answer: the chain is shown, and the table
underneath is left out, because the vault's other entries are a different
question and printing them buries the keys the directory actually uses. With
more than one layer the listing says which one wins each variable:

```sh
$ cd ~/work/git/acme/bidops && sanduk list
PROJECT  ~/work/git/acme/bidops
ENTRY   FROM        TYPE
code    ~/work/git  group
acme    …/acme      group
bidops  …/bidops    group

KEY                FROM    SOURCE
ANTHROPIC_API_KEY  code    stored
DATABASE_URL       bidops  stored
GITHUB_TOKEN       acme    stored  (shadows code)
KUBECONFIG         bidops  stored
OPENAI_API_KEY     code    stored
sanduk: 6 entry/entries hidden (4 member, 2 global) — see "sanduk list --scope all"
```

Outside any project there is no chain to show, so the global entries are the
answer, and two kinds are left out:

- **project** — entries that belong to a repository, yours or another one.
- **member** — flat secrets that a group links to. They are that group's
  building blocks, and they already appear as the `SOURCE` of the variable
  they supply.

Expanding the chain opens the entries it resolves — never a value, and each
entry at most once, so a presence-gated backend prompts once per entry;
`--no-keys` skips the reads altogether. An unapproved project file narrows
nothing: the listing says so and shows the vault as if the file were not
there.

`--scope all` shows everything with a `SCOPE` column, and `--scope project|member|global`
shows one kind. A project entry names the directory that owns it, `project (~/work/x)`,
or `project (here)` when the folder you are in resolves it. The hidden count always goes to stderr, so a shorter list never
reads as a missing entry. Membership is tracked in `config.toml` as a
`linked_by` hint, kept current by `group link`/`unlink` and rebuilt from the
vault by `sanduk doctor --links --fix`; nothing resolves through it, so a stale
hint can only misplace a row in a listing.

A location that no longer exists claims nothing: if you delete or move the
repository, the group is global again and shows up everywhere, marked
`(missing)` by `sanduk group list` and reported by `sanduk doctor --metadata`.
Re-run `sanduk allow` in its new place to repair it.

`sanduk group list` is the view from the other side — every group, and the
repository each one serves:

```sh
$ sanduk group list
NAME      LOCATION              INFO
warden *  ~/work/warden         Warden staging
agents    -                     LLM provider keys
```

The groups that apply where you are standing come first, then the directories
above and below you, then everything else alphabetically.

Use `.envrc` only for interactive work — direnv exports into the shell, so every
child process of that shell (including an agent) inherits the secrets:

```sh
sanduk init --envrc     # writes it, with that warning next to it
```

```sh
# .envrc — committed; names only, never values
eval "$(sanduk export --format shell)"
```

It is off by default for that reason, and it does not make the prompting any
cheaper: direnv runs the export on every load, so a group with three links
costs four keychain reads each time you enter the directory, where
`sanduk exec` costs them once per command you actually run.

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

### Find copies that have escaped the vault

A secret is only as contained as its worst copy. `scan` looks for your stored
values where they should never be:

```sh
sanduk scan                     # the working tree
sanduk scan --git               # and everything git has ever recorded
sanduk scan --shell             # and your shell history
```

```
ENTRY            LOCATION
stripe           ./config/settings.py:14
warden.DB_URL    git history  (run: git log -S <value> to find the commit)
```

A finding is an entry name and a location — **never the value** — so the output
is safe to paste into an issue. It exits 3 when anything turns up.

Two things make this unlike every other command. It reads *every* value in the
vault, because it has to know what to look for, so it asks first and needs
`--yes` without a terminal; on a presence-gated backend expect a prompt per
entry. And the values are matched inside sanduk, never handed to `grep` or
`git grep` as an argument, because argv is readable by every process on the
machine — searching for a secret that way would leak it.

Dependency and build directories (`node_modules`, `vendor`, `target`, `dist`, …)
are skipped, binary files are not searched, and values shorter than
`--min-length` (default 8) are reported as unsearchable rather than matching
everywhere.

### Import from another credential manager

`import` reads a `.env` file, a CSV export, or a 1Password `.1pux` archive. The
format comes from the extension and can be forced with `--format`:

```sh
sanduk import .env.production            # KEY=VALUE pairs
sanduk import export.csv --dry-run       # names only, nothing written
sanduk import export.csv
sanduk import account.1pux --group work
```

For CSV the header picks the columns: a name column (`name`, `title`, …) and a
value column (`login_password`, `password`, `credential`, …). A `fields` column
of `label: value` lines contributes one entry each, which is usually where API
keys live rather than in the password column.

From a `.1pux` archive only secret-bearing fields are taken: a login's password
and concealed custom fields. Usernames, URLs, notes and attachments are left
behind — a credential store should not absorb an address book.

Entry names come from the item title (and field label), upper-cased with other
characters replaced by underscores, so `Acme API` / `API Key` becomes
`ACME_API_API_KEY`. Two items that would collide both survive, under distinct
names.

> An export file is a plaintext copy of another vault. Review it with
> `--dry-run` first — which prints names and never values — and pass `--delete`
> to remove it once the import succeeds.

### Give an agent access without giving it secrets

`sanduk mcp` serves entry metadata to a coding agent over the Model Context
Protocol, on stdin/stdout:

```json
{ "mcpServers": { "sanduk": { "command": "sanduk", "args": ["mcp"] } } }
```

Three tools, all read-only: `list_entries`, `describe_entry` and `check_expiry`.
They return names, kinds, the variables each entry provides, descriptions and
rotation dates. **None returns a value, and none runs a command.**

The point is what the agent does next. Knowing that `warden` provides `DB_URL`
and `API_KEY`, it writes an ordinary command:

```sh
sanduk exec warden --only DB_URL -- psql
```

That stays subject to whatever approval already governs the agent's shell. An
execution path inside the server would be a way around it, which is why there
isn't one — an agent that can already run commands gains nothing from a second
door, and the user loses the one checkpoint they had.

The server inherits the invocation's backend settings, so point it at the vault
you want described with `--backend`/`--config` or the `SANDUK_*` variables.

## Commands

| Command | Description |
| --- | --- |
| `sanduk init [entry]` | Write and approve this repository's `.sanduk.toml` (`--from-template` prompts for the keys a committed `.env.example`/`.env.template` lists, `--from-dotenv`, `--include`, `--only`, `--envrc`, `--force`). |
| `sanduk add <name>` | Add a new flat secret (`--as VAR`, `--from-env VAR`, stdin, `--description`, `--expires`, `--replace` to overwrite any entry). Refuses an existing secret; use `rotate`. |
| `sanduk add-group <name>` | Create a group (`--from-json F\|-`, `--field KEY` prompts, `--description`, `--expires`, `--replace`, `--global`). With no fields it creates an empty group. Only this command creates a group. At a repository root, or in a folder with no `.git`, it also records the directory and writes `.sanduk.toml`. |
| `sanduk add-file <name>` | Store a credential file, injected as a path (`--from-file F\|-`, `--as VAR`, `--filename`, `--description`, `--expires`, `--replace`). |
| `sanduk group add [-g G] [KEY…]` | Add fields to an existing group (prompt, stdin, `--from-env VAR` for the one KEY, `--from-dotenv F`, `--replace` to overwrite a field that exists, `--description`, `--expires`). |
| `sanduk group remove [-g G] KEY…` | Remove fields from a group. |
| `sanduk group rotate [-g G] KEY` | Replace the stored value of a field that already exists (prompt, stdin or `--from-env VAR`). Refuses a field the group does not store. |
| `sanduk group link [-g G] [VAR=]<target>` | Point a group field at a flat secret, or at `Group/FIELD` (the `VAR=` form is required for that). |
| `sanduk group unlink [-g G] KEY…` | Remove a link; the target it pointed at is untouched. |
| `sanduk group list` | List every group and the directory it belongs to (`--here`, `--keys`, `--json`, `--names`). |
| `sanduk group show [-g G]` | List a group's field names (never values; `--long` shows which are links). |
| `sanduk get <name>` | Inspect a value, redacted unless `--reveal` (terminal-gated, `--force`; `--field KEY` for one field of a group). |
| `sanduk exec [selector…] -- <cmd>` | Run a command with secrets injected (`--only`, `--except`, `--list`, `--materialize DIR`). |
| `sanduk export [selector…]` | Emit secrets for scripts (`-f shell\|dotenv\|json`, `--only`, `--except`, `-o FILE`, `--replace`, `--materialize DIR`). |
| `sanduk list` | List the entries in scope: this repository's, plus the global ones (never values; `--scope`, `--no-keys`, `--json`, `--names`). An entry with no metadata shows as `unknown`. |
| `sanduk doctor [entry]` | Check a group against `.env.example` (`--template F`); exits 3 on missing keys. `--metadata [--fix]` checks config.toml against the vault; `--links` checks every group link resolves (`--fix` rebuilds the `linked_by` hints); `--expiry [--within N]` reports rotation deadlines; `--chain` reports the `.sanduk.toml` files that apply here and which variables come from outside this directory. |
| `sanduk scan [path…]` | Search the tree (`--git`, `--shell`) for stored values that escaped the vault; exits 3 on a find. Reads every value, so it needs `--yes` without a terminal. |
| `sanduk mcp` | Serve entry metadata to an AI agent over MCP — names only, never values, no execution. |
| `sanduk allow [dir]` | Approve a repository's `.sanduk.toml` for `exec`/`export`/`doctor`, and record the group's location. Names the files above it that still need approving. |
| `sanduk remove <name>` | Delete an entry and its metadata (prompts; `--yes` when non-interactive). |
| `sanduk rotate <name>` | Replace a flat secret's value (`--from-env VAR`, or prompt/stdin; `--expires` to move the deadline). |
| `sanduk import <file>` | Bulk-import a `.env`, CSV export or `.1pux` archive (`--format`, `--group NAME`, `--replace`, `--delete`, `--dry-run`). |
| `sanduk config` | Show the config file and the settings sanduk resolved (`--path`, `--edit` opens `$EDITOR`). |
| `sanduk completion <shell>` | Print a completion script for bash, zsh, fish or powershell. |
| `sanduk version` | Print the version (also `--version`). |

Global flags, accepted by every command: `--backend`, `--keychain`, `--service`
(each overriding its `SANDUK_*` variable and the config file) and `--config`
(the metadata config path; the trust store moves with it).

Both `exec` and `export` take the same *selectors*: a bare entry name (a group
contributes one variable per field), or `ENVVAR=entry` to bind a single flat
secret to a variable. They are merged left to right, and `.sanduk.toml` supplies
the entry when none is given (after you approve it with `sanduk allow`) — along
with the project files in parent directories, outermost first.
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

Reads within one command share a single `LAContext` with a 10-second reuse
window, so resolving a group and the secrets it links prompts once rather than
once per read. The window is per process: it dies with the command that opened
it, and never carries over to the next invocation.

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
- After a reboot the dedicated keychain is locked, so reads will prompt. It also
  re-locks on a timer: `security show-keychain-info ~/Library/Keychains/<name>.keychain-db`
  shows it, and `security set-keychain-settings -l ~/Library/Keychains/<name>.keychain-db`
  drops the idle timeout while keeping lock-on-sleep.
- **A prompt is per item, not per command.** Resolving a group with three links
  reads four entries, so an untrusted binary is asked four times. sanduk puts
  itself in the ACL of every item it writes, which removes the prompt for
  items written after v0.0.7; "Always Allow" does the same for older ones, one
  at a time.
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
| `.sanduk.toml` (file) | Repository's default entry and optional key subset (approve with `sanduk allow`). Files in parent directories apply too, outermost first; `inherit = false` seals a directory. |

The metadata config can also carry standing backend settings. They are defaults
only, and are resolved last:

```toml
# ~/.config/sanduk/config.toml — non-secret, safe to version
backend  = "keychain"   # or secret-service, wincred, pass, file/age, ...
keychain = "sanduk"     # macOS: a dedicated keychain (opt in deliberately)
service  = "sanduk"     # namespace; also names the age vault file

# Entry metadata, written by sanduk: which variable each secret becomes, what
# it is for, and when it should be rotated. Values live in the keystore and
# never appear here.
[secrets]
  [secrets.stripe]
    env = "STRIPE_KEY"
    description = "billing"
    expires = "2027-04-01"

[groups]
  [groups.warden]
    description = "the warden service"

[files]
  [files.gcp]
    env = "GOOGLE_APPLICATION_CREDENTIALS"
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
trusted.toml hold names, variable mappings, descriptions, rotation dates and
hashes — never a value — and are written with mode 0600. `.sanduk.toml` comes
from the repository and is untrusted until you approve its exact contents. Your
shell's environment is outside the boundary: `export` and direnv put secrets
there; `exec` does not. Plaintext reaches disk only where you ask for it:
`export --output`, and `--materialize DIR` for file secrets.

### Security notes

- Values are never accepted as command-line arguments, so they cannot leak via
  process listings (`ps`). Input comes from an echo-disabled prompt, an env var,
  a file, or stdin — including for `add-group`, whose `--field KEY` prompts and
  whose `--from-json` reads a file or stdin, and for `add`, which reads stdin
  (`sanduk add NAME < key.txt`) rather than taking a file flag.
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
- A **file secret** never becomes a file anyone else can open. `exec` writes the
  bytes to a temporary file, unlinks it immediately, and passes the child
  `/dev/fd/N` for the descriptor it inherits, so the data has no name on disk
  and is freed when the child exits. `exec --materialize DIR` and
  `export --materialize DIR` are the deliberate exceptions: they write real
  files (mode `0600` on Unix), and those persist until you delete them. `get` never prints file
  bytes, whatever flags you pass.
- **`sanduk scan` is the one command that reads every value**, because it cannot
  search for secrets it has not loaded. It says so and asks before doing it, and
  refuses without `--yes` when there is no terminal; on `keychain-biometric`
  expect a prompt per entry. Its findings name an entry and a location and never
  a value, so the report is safe to paste. The values it searches for stay in
  this process — they are never passed to `grep` or `git grep` as arguments,
  where every process on the machine could read them out of `ps`.
- **Rotation deadlines are advisory.** `--expires` is metadata, and sanduk will
  not refuse to inject an expired secret — failing a deploy over a date in a
  config file is worse than the stale key it warns about. `doctor --expiry`
  reads no values, so it is safe to run unattended and in CI.
- An **export file from another credential manager** (`.csv`, `.1pux`) is a
  plaintext copy of that whole vault. Review it with `import --dry-run`, which
  prints names only, and pass `--delete` to remove it once the import succeeds.
- A process running as *you* with shell access can still run `sanduk`. Pair this
  with your agent's own command-permission rules, and use the experimental
  `keychain-biometric` backend so reading a secret requires Touch ID, which a
  headless agent cannot satisfy. `sanduk mcp` is the other half of that: it lets
  an agent discover what credentials exist — names, kinds and variable names —
  without being shown a value, and without giving it a way to run commands that
  bypasses your approval.

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

The message names the file, which may be in a **parent** directory: project
files apply to everything below them, and each is approved separately. Pass the
directory to approve one without changing into it (`sanduk allow ~/work/git`),
and run `sanduk doctor --chain` to see every layer at once.

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
