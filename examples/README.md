# Examples

Two things live here:

- `config.toml.example` — a commented starting point for sanduk's metadata
  config, including **how to choose a backend in the config file**.
- `project/` — a small runnable project that uses sanduk end to end.

## Quick start

```sh
cd examples/project
make           # list the targets
make setup     # load the dummy values into your vault
make dev       # run the "app" with the secrets injected
```

Nothing in `project/` is a real secret. The values are obvious placeholders, so
running the example is safe; it writes the group `demo` to whichever backend you
have configured, and `make clean` removes it again.

## What the project demonstrates

| File | Purpose |
| --- | --- |
| `.sanduk.toml` | Committed, non-secret: names the entry this project uses, so `sanduk exec -- <cmd>` works from any subdirectory. |
| `.env.example` | Committed template: every variable the project needs, with dummy values. This is what `sanduk doctor` compares against. |
| `demo.env` | The values `make setup` loads. Dummies, and deliberately missing `LEGACY_TOKEN`. |
| `Makefile` | The workflow: `help`, `setup`, `doctor`, `show`, `dev`, `list`, `add-legacy`, `clean`. |
| `setup.sh` | Loads `demo.env` as a group, prints which backend it is about to write to, and shows then approves `.sanduk.toml`. |
| `check.sh` | Stands in for the app: reports which variables the process received, never their values. |
| `.envrc` | Optional direnv integration. It does **not** load secrets into your shell, and explains why. |

## The walkthrough, by hand

The Makefile is a thin wrapper; these are the same steps without it.

```sh
# 1. Load the values. The group name is a vault entry, not a file.
sanduk add-group demo --from-dotenv demo.env --description "sanduk demo project"

# 2. Look at what is stored without reading any value.
sanduk group show demo     # one field name per line
sanduk list                # names, types and env mappings only

# 3. Run something with the secrets injected. They exist only in this child:
#    sanduk replaces itself with the command (execve), so the parent shell,
#    and anything watching it, never holds them. (On Windows, which has no
#    execve, sanduk starts the command as a child and waits for it.)
sanduk exec demo -- ./check.sh

# 4. .sanduk.toml names the entry, so after approving it (read it first — a
#    repository could name any entry) the entry can be left out:
cat .sanduk.toml
sanduk allow
sanduk exec -- ./check.sh

# 5. Prove nothing leaked into your shell.
./check.sh                 # every key reports "not set"

# 6. Report drift against the template. demo.env omits LEGACY_TOKEN, so this
#    exits with status 3 and names it. That is the point of the command.
sanduk doctor demo --template .env.example

# 7. Fix the drift for one key, then check again. --from-env reads each named
#    key from the environment variable of the same name.
export LEGACY_TOKEN=demo-legacy-token-not-secret
sanduk group set demo LEGACY_TOKEN --from-env
unset LEGACY_TOKEN
sanduk doctor demo --template .env.example

# 8. Narrow what a child receives, without touching the vault. An unknown key
#    is an error rather than a silent no-op.
sanduk exec demo --only DB_URL,API_KEY -- ./check.sh

# 9. Put it back the way you found it.
sanduk remove demo
```

## Choosing a backend in the config file

sanduk picks a backend in this order, first match wins:

```
--backend age  >  SANDUK_BACKEND=age  >  backend = "age" in config  >  platform default
```

So the config file is where you set a standing preference, and a flag or an
environment variable overrides it for one command or one shell — useful when one
checkout is used on a Mac and on a Linux CI box.

The config lives at `$XDG_CONFIG_HOME/sanduk/config.toml` (usually
`~/.config/sanduk/config.toml`); override it with `--config` or
`SANDUK_CONFIG`. See `config.toml.example` for the commented version:

```toml
# backend = "keychain"     # macOS login Keychain (the default on macOS)
# keychain = "sanduk"      # macOS: a dedicated keychain, opt in deliberately
# service = "sanduk"       # namespace; also names the age vault file
```

`backend`, `keychain` and `service` are all optional and all default-only: the
config never wins over a flag or an environment variable.

Ask sanduk what it actually resolved, and where each value came from:

```sh
sanduk config
# path:    /Users/you/.config/sanduk/config.toml
# backend: platform default (from default)
# keychain: login (from default)
# service: sanduk (from default)
# flags beat environment variables, which beat this file
```

A flag or an environment variable that wins shows up here as `(from flag)` or
`(from env)`, so there is no guessing about which of the three is in play.

One caveat: `backend` is machine-specific. A config that says `"keychain"`
fails on Linux and a config that says `"secret-service"` fails on macOS, which
is why the example leaves it commented out. Set it on the machine that needs it,
or leave it unset and let the platform decide.

## A note on `keychain-biometric`

The `keychain-biometric` backend is experimental, and every read triggers a
Touch ID prompt. Do not point the example at it, and do not run it
automatically — a prompt blocks anything waiting on the process.
