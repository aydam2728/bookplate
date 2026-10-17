## What changes

<!-- The behaviour that changes, in one or two sentences. Not the file list. -->

## Which test proves it

<!-- Name the test. "Manually checked" is not an answer. -->

## Which security rule it touches

<!-- Authorisation scope, outbound network calls, rendering of external data,
     authentication, container hardening, or "none of them". -->

## Checklist

- [ ] Commits are signed off (`git commit -s`)
- [ ] `make check` passes locally
- [ ] Under 400 changed lines, excluding generated code
- [ ] Vertical slice: transport, service, repository, migration, tests
- [ ] Every new route has its authorisation triplet
- [ ] No new dependency, or it is justified above
- [ ] An ADR was added or updated if this is structural
- [ ] This change was mostly produced by an AI agent: yes / no
