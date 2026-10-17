# Scope

The target is **a household, not an institution**: 50 to 20,000 books, 1 to 15
accounts, one instance per home.

## In scope

1. **Cataloguing** — EAN-13 / ISBN-10 / ISBN-13 with check-digit validation,
   bulk scan mode, USB barcode scanners, camera scanning, and full manual entry
   for books that have no ISBN at all.
2. **Places** — an arbitrarily deep tree, bulk moves, and a clear separation
   between a *physical place* (one per copy) and a *logical collection* (many).
3. **External metadata** — an ordered, configurable provider chain, with
   per-field provenance so a refresh never overwrites a human correction.
4. **Ratings and reading** — half-star rating 0.5 to 5 per person per work,
   optional review, reading status, private or household visibility.
5. **Multi-user** — instance, library (a household), member. Strict isolation
   between libraries.
6. **Search** — full text, accent-insensitive, with combinable filters.
7. **Export and backup** — complete, usable without this software.

## Out of scope — permanently

These are refused in review. Pointing at this list is the expected way to close
such a request.

- **Reading ebooks.** No EPUB reader, no DRM handling, no format conversion.
- **Acquiring files.** No downloader, tracker or indexer integration.
- **Federated social network.** No ActivityPub, no public feed.
  [BookWyrm](https://joinbookwyrm.com/) already does this well.
- **Professional library system.** No imposed Dewey classification, no fines,
  no Z39.50 server, no full MARC cataloguing.
- **Algorithmic recommendation.** Out of scope, and incompatible with the
  promise that no reading data leaves the instance.
- **Large-scale multi-tenant hosting.** Isolation between libraries is designed
  properly, but this is not a service for strangers.

## Deferred, but designed for

Lending (who has which book, since when) and imports (Goodreads, LibraryThing,
Calibre CSV) are not in 1.0. The data model must accommodate them without a
painful migration.
