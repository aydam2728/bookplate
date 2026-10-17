# 0008. Only three denormalised counters

- Status: Accepted
- Date: 2026-10-17

## Context

Several screens show counts: copies per work, household rating average, books
per place in the tree. Denormalised counters are fast to read and drift
silently, which is the worst failure mode: nobody notices until the numbers are
visibly absurd.

At 20,000 copies, a `COUNT(*)` on a proper index costs under a millisecond.

## Decision

**Three denormalised counters, no more.** They share one justification: each is
read once per row of a list, so computing them on the fly turns one query into
fifty.

- `work.copy_count`
- `work.rating_count` and `work.rating_sum`
- `place.subtree_copy_count`

Everything else is counted at read time.

`place.path` is a materialised path, so the ancestors of a place are exactly the
places whose path is a prefix of its own. Walking up the tree is therefore one
indexed query, not recursion.

## Consequences

Any denormalised counter ships with three things, or it does not ship:

1. **Triggers covering INSERT, UPDATE and DELETE**, including soft-delete
   transitions. One forgotten case and the counter drifts unnoticed.
2. **A reconciliation query** that must always return zero rows, runnable on
   demand from the administration screen.
3. **A test** that applies a random series of operations — adds, moves, deletes,
   restores — then compares every counter to the matching `COUNT(*)`.

Moving a *place* is deliberately not handled by a trigger: it rewrites the path
of a whole subtree, so application code does it inside a transaction and
recomputes the affected counters. Doing that in a trigger would produce SQL
nobody can read six months later.
