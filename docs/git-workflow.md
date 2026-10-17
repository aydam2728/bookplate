# Git workflow

Two rules carry everything else: **every commit on `main` is atomic**, and
**history stays linear**. Together they keep `git bisect` usable, which is the
practical reason to care — when a regression appears six months from now, a
bisect over atomic commits finds it in minutes.

## Commits

A commit is **one reversible logical change**. It builds and passes tests on its
own. If the subject line needs the word "and", it is two commits.

```
type(scope): imperative subject, lowercase, no final period

Why this change exists, not what it does. The diff already says what.
Wrapped at 72 columns.

Refs #12
Signed-off-by: Jane Doe <jane@example.org>
```

| Type | For |
| --- | --- |
| `feat` | New user-visible behaviour |
| `fix` | A bug in released behaviour |
| `docs` | Documentation only |
| `refactor` | Same behaviour, different structure |
| `test` | Tests only |
| `perf` | Measurable performance change |
| `build` | Build, Docker image, dependencies |
| `ci` | Workflows and automation |
| `chore` | Everything else, including repository plumbing |
| `revert` | Reverts a previous commit, naming its hash |

Scopes are the domain packages: `auth`, `catalog`, `places`, `rating`,
`metadata`, `platform`, `web`, plus `ci` and `docs`. Breaking changes carry a
`BREAKING CHANGE:` footer. The subject stays under 72 characters, in English.

**Commit atomically as you work.** Do not commit a mess and tidy it later with an
interactive rebase: the cleanup is error-prone, and in some environments
`git rebase -i` is not even available.

## Branches

`main` is protected, always releasable, and never committed to directly.

```
feat/catalog-isbn-lookup
fix/places-subtree-counter-drift
docs/adr-0009-backup-format
chore/renovate-config
```

Prefix with the commit type, then the scope, then a short description in
kebab-case. Branches are **short-lived**: cut from `main`, merged within a few
days. A branch alive for two weeks has become a fork.

Rebase onto `main` before merging; never merge `main` into your branch.

## Merging

**Rebase and merge.** Not squash, not a merge commit.

- Squash would collapse a carefully built series into one lump, which defeats
  atomic commits.
- A merge commit would break linear history and make bisect noisy.

This puts the burden where it belongs: the branch must *already* be a clean
series of atomic commits before it is merged. A branch whose commits do not each
build is not ready, regardless of whether its final state works.

A maintainer does not merge their own branch.

## Releases

Semantic versioning. `vX.Y.Z` tags trigger publication of the signed multi-arch
image.

- Minor and major releases are tagged **on `main`**.
- At the first `X.Y.0`, a `release/X.Y` branch is cut. Patch releases for that
  line are tagged there.
- **Fixes land on `main` first**, then are cherry-picked onto the release
  branch. Never the reverse: a fix that exists only on a release branch comes
  back as a regression in the next minor.

`SECURITY.md` promises security fixes for the latest minor version. The
`release/X.Y` branch is what makes that promise keepable.

## What AI agents must do

Same rules, no exemption. Specifically:

- One commit per logical change, written as you go.
- Never commit directly to `main`.
- Never rewrite published history.
- A commit mostly produced by an agent carries a `Co-Authored-By:` trailer
  naming it, which is how the disclosure rule in `AGENTS.md` is satisfied at
  commit level.
