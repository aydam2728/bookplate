# 0006. AGPL-3.0-or-later

- Status: Accepted
- Date: 2026-10-17

## Context

The software is used over a network. Under a permissive licence, anyone could
take the accumulated community work and run it as a closed hosted service, which
would hollow out the promise that your data stays with you.

Alternative considered: Apache-2.0, for maximum reuse of the code by other
projects.

## Decision

**AGPL-3.0-or-later**, together with the Developer Certificate of Origin rather
than a contributor licence agreement (no copyright assignment).

## Consequences

- Modifications to a hosted instance must be published. Nothing is restricted for
  an individual self-hosting at home.
- The usual objection, that some companies forbid their employees from
  contributing to AGPL code, carries little weight here: the expected
  contributors are self-hosters, not vendor teams.
- **Real cost accepted:** no permissively licensed project can reuse this code.
  If that becomes a problem, the genuinely generic pieces — ISBN normalisation,
  the metadata provider chain — can be extracted into separate libraries under
  Apache-2.0, keeping the server under AGPL.
- Because of the DCO, relicensing later would require every contributor's
  agreement, which is impossible in practice. This decision is final and had to
  be made before the first external contribution.
