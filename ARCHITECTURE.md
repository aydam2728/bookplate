# Architecture

A **modular monolith in a single container**, SQLite by default. Architectures
that need a separate database and cache are the main reason self-hosted software
is abandoned at install time.

## The two bottlenecks

Everything hangs off two deliberate chokepoints. They are the only places that
need fine-grained audit in review:

1. **Authorisation.** Every inbound request passes through it before reaching the
   domain. Every repository method takes a call context carrying the user and the
   library, so forgetting the scope is a compile error rather than a leak.
2. **The metadata gateway.** Every outbound request passes through it: queue,
   rate limiting, domain allow-list, cache. The server calls no other host.

## Layers

```
transport (HTTP)  ->  service (domain)  ->  repository (data)
```

Dependencies point inward. The service layer knows nothing about HTTP or SQL.
No layer skipping; the linter enforces the permitted imports between packages.

## Package by domain, not by technical layer

```
internal/
  auth/        accounts, sessions, invitations, roles
  catalog/     works, editions, copies, identifiers
  places/      the place tree and its counters    <- reference package, copy this one
  rating/      the per-user layer
  metadata/    provider chain, fetching, covers
  platform/    database, migrations, logging, configuration
web/           TypeScript and React client, embedded into the binary at build time
```

`places/` is small and complete. It is the designated template: new domains are
built to look like it.

## Stack

| Layer | Choice | Why |
| --- | --- | --- |
| Server | Go | Explicit, no magic, compiles in seconds, 40-80 MB RAM |
| Data access | Hand-written SQL, generated typed accessors | Reviewable queries; an invented column does not compile |
| Database | SQLite in pure Go | No C toolchain, trivial amd64/arm64 cross-compilation |
| Client | TypeScript, React, Vite | Largest corpus for AI assistance, within a 250 KB budget |
| Contract | Generated OpenAPI 3.1, generated TS client | Typing crosses the language boundary |

See [ADR 0002](docs/adr/0002-go-server-typescript-client.md) and
[ADR 0003](docs/adr/0003-sqlite-only.md).

## Domain model

Three distinct levels: a **work** (what you read), an **edition** (the editorial
object with an ISBN), a **copy** (the object on the shelf). Conflating them makes
"I own two of these" and "I have the paperback and the hardback" impossible.

Ratings attach to the *work*. Place and condition attach to the *copy*.
Everything is scoped by `library_id` — see [ADR 0005](docs/adr/0005-library-scoping.md).

## Non-negotiable targets

| Target | Value |
| --- | --- |
| Reference machine | Raspberry Pi 4, 4 GB |
| Catalogue size | 20,000 copies |
| Search | p95 under 300 ms at full size |
| Resident memory | under 256 MB |
| Image size | under 150 MB compressed, per architecture |
| Initial JS bundle | under 250 KB gzipped |
| Required external services | zero |
