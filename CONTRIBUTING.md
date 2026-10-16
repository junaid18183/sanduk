# Contributing

Thanks for helping. sanduk is small on purpose, and its value is in a handful
of security properties, so most of the guidance is about not breaking those.

## Before you start

Read [AGENTS.md](AGENTS.md). Despite the name, it is the project's working
guide for anyone, human or agent: the layout, the commands, and the
**architecture invariants**. A change that breaks an invariant is a bug even
when every test passes.

For a security issue, do not open a pull request or issue; follow
[SECURITY.md](SECURITY.md).

## Working on it

```sh
make build    # ./sanduk
make test     # quick
make check    # everything CI runs; run it before you push
```

Tests never touch your real keychain: both test packages force the age backend
into temporary directories. Keep it that way — never test against a real
backend or with a real secret, and use obvious fakes (`sk-test-value`) in
tests, examples and commit messages.

Some guidelines:

- Domain logic goes in `vault/`; `cli/` stays thin.
- A new command needs a test that runs it through `runCmd` (see
  `cli/cmd_test.go`) and a row in the README's command table.
- A new way for a name or value to reach a child or `export` must go through
  `vault.Exec` / `vault.FormatEnv`, which validate every name.
- A behaviour change goes in CHANGELOG.md under "Unreleased"; one that can
  break someone's workflow goes under **Breaking**.
- Keep `gofmt`, `go vet` and `golangci-lint` clean on Linux, macOS and Windows.
