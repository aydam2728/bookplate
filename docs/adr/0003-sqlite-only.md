# 0003. SQLite is the only required datastore

- Status: Accepted
- Date: 2026-10-17

## Context

Self-hosted software is most often abandoned at install time. Every service a
user must stand up beside the application, a database and a cache in particular,
costs installs.

The target is 20,000 copies and 15 accounts on a single instance per household.

## Decision

**SQLite in WAL mode, in pure Go, is the only required datastore.** Full-text
search uses FTS5 in the same file. No cache service, no message broker, no
separate search engine.

One volume, `/data`, holds the database and the covers, so there is a single
thing to back up.

PostgreSQL may later be offered as an operator option. It will never be a
prerequisite.

## Consequences

- `docker run` with one volume is the whole installation.
- A pure Go driver means no C toolchain and trivial amd64/arm64 builds.
- `STRICT` tables are used so SQLite checks types instead of accepting anything.
- Writes are serialised. At household scale this is irrelevant; it would not be
  at institutional scale, which is out of scope.
- Supporting a second engine later would double the cost of query testing. That
  is the decision to weigh before accepting PostgreSQL.
