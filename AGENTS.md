# Rules for AI coding agents

This file is the operating contract for any AI agent working in this repository.
Read it before writing code. It is short on purpose; keep it that way.

A repository that an agent can navigate is a repository a new contributor can
navigate. There is no trade-off between the two audiences.

## Commands

Not yet available: the build does not exist before milestone P1. When it does,
every command below must run locally and produce exactly what CI produces.

```
make check    # everything CI runs: format, lint, types, tests, generated-code drift
make test     # tests only
make generate # regenerate SQL accessors and the OpenAPI client
make run      # local instance on http://localhost:8080
```

## Where things go

See ARCHITECTURE.md for the full map. Short version: `internal/<domain>/` for
server code, `web/` for the client, `docs/adr/` for decisions.
**`internal/places/` is the reference package. Build new domains to look like it.**

## Hard rules

Breaking one of these is a reason to reject a change even if it works.

1. **No layer skipping.** transport to service to repository, dependencies
   inward. A handler never touches the data layer directly.
2. **No global state, no singletons, no DI container.** Dependencies arrive
   through constructors. A service that cannot be instantiated in a test without
   a database is built wrong.
3. **Every repository method takes a call context** carrying the user and the
   library. Never query without scope.
4. **No ORM.** SQL is written by hand; typed accessors are generated from it.
5. **Never edit generated code.** Change the source, regenerate, and let CI
   verify there is no drift.
6. **No new dependency.** Adding one is a human decision, justified in the pull
   request. This is where supply-chain security and AI assistance intersect, and
   it is not delegable.
7. **No new outbound network call** without an explicit entry in the provider
   allow-list. A test fails if an unknown host appears.
8. **External data is text, never rendered HTML.** Metadata from public APIs is
   edited by strangers.
9. **One word per concept.** work, edition, copy, place, collection, rating.
   Never book, item or volume for the same thing. The display name of the
   project is BookPlate; everywhere else, module path, image name, packages,
   files and identifiers, it is `bookplate`, lowercase.
10. **English everywhere** in the repository: code, comments, commits, issues.

## What a change looks like

- **A vertical slice**: transport, service, repository, migration and tests
  together. A change that adds only one layer cannot be verified.
- **Under 400 changed lines**, excluding generated code. Beyond that, review
  becomes a rubber stamp: split it, or justify the size.
- **One file, one concept, 400 lines maximum.** Comments say why, never what.
- **Tests against the real SQLite engine**, never a stub. The interesting bugs
  live in queries and transactions.
- **One commit per logical change**, written as you go, on a branch cut from
  `main`. Never commit to `main`, never merge into `main`, never rewrite
  published history. Push the branch and stop: whether and when it lands is
  the maintainer's decision, not yours. See `docs/git-workflow.md`.
- **Every route ships its authorisation triplet**: a legitimate actor succeeds,
  an actor from another library fails, an anonymous caller fails.

## What stays human

- **Review.** The Signed-off-by line means a person has read and understands the
  code, whether or not they wrote it. Signing without reading is the one
  genuinely serious offence in this project.
- **Architecture decisions**, recorded in `docs/adr/` before the code.
- **Scope judgements.** The list of things this project will never do is not
  delegable.

## Disclosure

A pull request mostly produced by an agent says so. Not to penalise it, but to
calibrate review effort. Three hundred lines written in three minutes do not get
read like three hundred lines written in three hours.
