# Architecture decision records

One file per structural decision, numbered and dated, never deleted. A decision
that turns out wrong gets a new record that supersedes it; the old one stays so
the reasoning remains readable.

Without these, a volunteer reviewer cannot judge whether a contribution respects
the intent of the project, and review degenerates into an argument about style.

Format: Status, Context, Decision, Consequences. Keep each under a page.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-go-server-typescript-client.md) | Go server, TypeScript and React client | Accepted |
| [0003](0003-sqlite-only.md) | SQLite is the only required datastore | Accepted |
| [0004](0004-work-edition-copy.md) | Work, edition and copy are three distinct entities | Accepted |
| [0005](0005-library-scoping.md) | Every content table carries library_id | Accepted |
| [0006](0006-agpl-licence.md) | AGPL-3.0-or-later | Accepted |
| [0007](0007-no-orm-generated-sql.md) | Hand-written SQL, generated typed accessors | Accepted |
| [0008](0008-three-denormalised-counters.md) | Only three denormalised counters | Accepted |
