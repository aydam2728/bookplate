# BookPlate

Self-hosted catalogue for the **physical** books in your home. Scan a barcode,
know which shelf the book is on, and let everyone in the household keep their own
ratings and reading history.

> **Status: pre-alpha, no working software yet.** This repository currently holds
> the scope, the architecture decisions and the project rules. Application code
> starts at milestone P1. Nothing here is deployable.

## What it does

- **Catalogue fast.** Scan EAN/ISBN in bulk from a phone camera or a USB scanner;
  metadata and covers are fetched automatically, and corrections are never
  overwritten by a later refresh.
- **Answer "where is it?"** An arbitrarily deep tree of places —
  `House > Living room > Ikea bookcase > Shelf 3`. No fixed levels imposed.
- **Answer "do we already own it?"** From a phone, standing in a bookshop.
- **One catalogue, one private layer per person.** Shared books, individual
  ratings, reviews and reading status.
- **One container, one volume, one command.** amd64 and arm64, runs on a
  Raspberry Pi 4, no database or cache service required.

## What it will never do

Read ebooks, download files, federate as a social network, or act as a
professional library system. The exclusions in [docs/scope.md](docs/scope.md) are
binding: they are quoted when closing out-of-scope requests, so that refusals are
predictable rather than arbitrary.

## Why another one

Mature self-hosted tools manage *files* (Calibre-Web, Komga, Kavita). Tools that
handle physical books are either hosted services (LibraryThing, Libib), built
around a single reader, or do not track where the object physically sits.
BookPlate is for a household with shelves.

## Documentation

| Document | What it covers |
| --- | --- |
| [docs/scope.md](docs/scope.md) | What is in, what is out, and what is deferred |
| [ARCHITECTURE.md](ARCHITECTURE.md) | How the code is laid out and why |
| [docs/adr/](docs/adr/) | Numbered, dated architecture decisions |
| [docs/git-workflow.md](docs/git-workflow.md) | Commits, branches, merges and releases |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to propose a change and what will be checked |
| [SECURITY.md](SECURITY.md) | How to report a vulnerability |
| [AGENTS.md](AGENTS.md) | Rules for AI coding agents working in this repository |

## Privacy

No telemetry, no phone-home, no analytics. What a person reads can reveal their
convictions; that data never leaves the instance.

## Licence

[AGPL-3.0-or-later](LICENSE). See [ADR 0006](docs/adr/0006-agpl-licence.md) for
the reasoning and the trade-off accepted.
