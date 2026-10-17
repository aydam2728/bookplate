# 0005. Every content table carries library_id

- Status: Accepted
- Date: 2026-10-17

## Context

The instance is assumed to be published on the internet. The highest-impact bug
class in a multi-tenant catalogue is direct object access: a member of one
household reading another household's data by changing an identifier.

Guarding against it case by case, in every handler, fails eventually. One missed
query is a leak.

## Decision

**A library is the isolation boundary.** Every content table carries
`library_id`, even when it could be derived through a join. Fourteen of the
eighteen tables do; only accounts, libraries, memberships and sessions do not.

**Every repository method takes a call context** carrying the user and the
library. The signature makes omission impossible.

The bibliographic reference data is duplicated per library rather than shared
between libraries.

## Consequences

- The scoping clause is uniform, indexable and testable. Every index starts with
  `library_id`.
- Required: one authorisation triplet per route, blocking in CI — a legitimate
  actor succeeds, an actor from another library fails, an anonymous caller fails.
- Direct data-layer access from a handler is a reason to reject a change, even
  when the code works.
- Duplicating bibliographic rows costs some storage. Since the scope is one
  instance per household, that is cheaper than sharing a reference catalogue, and
  it removes an entire class of cross-household leaks.
- An instance administrator is deliberately not granted access to library
  contents or private reviews. Their power is operational, not documentary.
