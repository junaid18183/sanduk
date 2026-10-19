# Security policy

sanduk holds secrets, so a security bug in it can expose every secret it
stores. Please report one privately rather than in a public issue.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting: on the repository page, open
**Security → Report a vulnerability**. That keeps the report visible only to
the maintainers until a fix is out.

Include what you can of:

- the sanduk version (`sanduk version`) and OS, and the backend in use;
- the steps to reproduce, using **fake** values (`sk-test-value`), never a real
  secret;
- what an attacker gains, and what they need first (a malicious repository, a
  process running as the same user, …).

You should get an acknowledgement within a week. A fix ships as the next
`0.0.x` release, credited to you unless you would rather not be named.

## Supported versions

sanduk is pre-1.0. Only the latest release receives fixes.

## Scope

The README's [Threat model](README.md#threat-model) describes what sanduk
defends against. In scope, for example: a cloned repository choosing which
secrets its scripts receive, a secret reaching argv, logs, a pipe or a child
it was not meant for, a name or value that breaks out of `export`'s quoting,
and anything that lets one store's secrets end up in another.

Out of scope: an attacker who can already run arbitrary commands as you (they
can run `sanduk` too), root, and reading the memory of a process you started
with the secret.

## Verifying a release

Release checksums are signed with keyless cosign, and each archive has a
GitHub build-provenance attestation. The README's Install section shows how to
verify both.
