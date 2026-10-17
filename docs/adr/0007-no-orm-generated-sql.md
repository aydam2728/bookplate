# 0007. Hand-written SQL, generated typed accessors

- Status: Accepted
- Date: 2026-10-17

## Context

A large share of this code will be written with AI assistance. The failure mode
to design against is not a syntax error, which the compiler catches, but a
plausible invention: a column that does not exist, a join that silently drops
rows, a query that forgets the scope.

An ORM hides the query behind a fluent API. What reaches the database is then
hard to review and easy to get subtly wrong.

## Decision

**No ORM.** Queries are written as SQL in `.sql` files; typed Go accessors are
generated from them against the real schema.

Migrations are numbered SQL files applied at startup, preceded by an automatic
backup.

## Consequences

- The schema is the source of truth. An invented column fails to generate, so it
  never reaches runtime.
- Reviewers read the actual query, which is the thing that matters for both
  correctness and the scoping rule of ADR 0005.
- Generated code lives in marked directories and is never edited by hand. CI
  fails on drift between source and generated output.
- Verbose compared to an ORM. That verbosity is the point: it is reviewable.
- Full-text indexing is the one exception. The `authors` column of the search
  index is the result of a join, so it is written by the application inside the
  same transaction, with a rebuild command and a consistency test.
