# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-10-17

## Context

This project expects contributions from strangers, and a large share of its code
will be written with AI assistance. Both make the same thing expensive: a
reviewer who cannot tell whether a change respects the intent of the design.

Intent that lives only in a maintainer's head cannot be reviewed against. It also
cannot be read by an agent, which will happily produce code that is locally
correct and structurally wrong.

## Decision

Every structural decision gets a numbered record in `docs/adr/`, written **before
the code**, using Status, Context, Decision, Consequences.

Records are never deleted or rewritten. A reversal is a new record that
supersedes the old one.

## Consequences

- Review can point at a record instead of arguing from taste.
- `AGENTS.md` can cite records rather than restating them.
- Small cost per decision, paid by whoever proposes it.
- Risk: records going stale. Mitigated by requiring one for anything structural,
  which keeps them few and load-bearing.
