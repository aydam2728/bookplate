# 0002. Go server, TypeScript and React client

- Status: Accepted
- Date: 2026-10-17

## Context

The stack had to satisfy three constraints at once: produce code that an AI
assistant writes correctly on the first attempt, stay light on both the server
and the client, and be reviewable by volunteers.

Options considered: TypeScript end to end, Go plus a TypeScript client, Python
with FastAPI, Rust with Axum.

The reference machine is a Raspberry Pi 4 with 4 GB of RAM. Rough server memory:
Go 40-80 MB, TypeScript 150-250 MB, Python 200-350 MB, Rust 30-60 MB.

## Decision

**Go for the server, TypeScript and React for the web client.**

The client is served by the same binary, with its static files embedded at build
time, so there is one artefact to publish, sign and verify.

## Consequences

- Go has no decorators, no reflection-based injection and no implicit magic.
  There is very little an agent can get subtly wrong, and what it does get wrong
  the compiler and linter stop before review.
- Compilation in seconds keeps the feedback loop tight.
- A static binary of roughly 30 MB cross-compiles to arm64 without a C toolchain,
  provided the SQLite driver is pure Go (see ADR 0003).
- **Two languages means no shared types.** Handled by the contract: the server
  publishes a generated OpenAPI description, and the TypeScript client is
  generated from it. An API change breaks the client build in CI.
- Accepted cost: two toolchains, two linter configurations, and frontend
  contributors who will never touch the server.
- Rejected: Rust, because too few volunteer reviewers can judge it; Python,
  because optional typing lets AI mistakes reach production.
