# 0004. Work, edition and copy are three distinct entities

- Status: Accepted
- Date: 2026-10-17

## Context

Most book software has one "book" entity. That makes two ordinary household
situations impossible to express: "I own two of these" and "I have the paperback
and the hardback".

It also puts ratings in the wrong place. Nobody rates a binding.

## Decision

Three levels:

- **work** — what was read: title, series, summary, authors
- **edition** — the editorial object identified by an ISBN: publisher, year,
  format, language, page count
- **copy** — the physical object on a shelf: place, condition, price, notes

Ratings and reading status attach to the **work**. Place and condition attach to
the **copy**. A copy has exactly one place and any number of logical
collections.

## Consequences

- Two extra joins on the hottest read path, mitigated by indexes and by the
  counters of ADR 0008.
- Deduplication becomes meaningful: the same ISBN is the same edition, a second
  physical object is a second copy.
- Books with no ISBN are first-class. A 1890 edition must catalogue as easily as
  a 2025 reprint; this is a structural requirement, not an edge case.
- Contributors find the model heavier than expected. Mitigated by the annotated
  schema, this record, and `internal/places/` as a worked example.
