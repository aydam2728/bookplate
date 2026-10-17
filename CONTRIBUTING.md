# Contributing

Thanks for considering it. This project is young, so please read the two
documents that decide whether a change can be accepted at all: `docs/scope.md`,
which lists what this project refuses to do, and `ARCHITECTURE.md`.

Before milestone P3 the domain model is still moving. Contributions are welcome
but may be rejected for reasons that have nothing to do with their quality. If
you want to avoid wasted effort, open an issue first.

## Certificate of origin, not a CLA

Every commit carries a Signed-off-by line:

```
git commit -s -m "your message"
```

That line is the Developer Certificate of Origin (https://developercertificate.org/):
you state that you may submit this work under the project's licence. There is no
copyright assignment, and no licence change is being prepared.

It also means something stricter here: you have read and understand the code you
are signing for, whether or not you wrote it.

## Language

English, everywhere in the repository: code, comments, commit messages, issues
and developer documentation. This is the condition for receiving contributions
from outside France. User-facing documentation is bilingual English and French,
and French is welcome in discussions.

## What will be checked

Every one of these is blocking. None of them is advisory.

- Formatting and linting, applied by a single tool per side
- Unit tests of the domain, integration tests against a real SQLite database
- The authorisation matrix: legitimate actor, actor from another library, anonymous
- Static analysis, dependency composition, secret scanning, image scanning
- End-to-end runs of the five load-bearing use cases
- Accessibility checks and the frontend performance budget
- No drift in generated code

Run `make check` locally. It runs exactly what CI runs, so "it works on my
machine" should be technically impossible.

## Pull requests

The full branching, commit and release model is in docs/git-workflow.md. In short:

- **Short-lived branches, linear history, rebase and merge.**
- **Atomic commits in Conventional Commits format.** One commit, one reversible change; the changelog is generated from them.
- **Under 400 changed lines**, excluding generated code.
- **A vertical slice**: transport, service, repository, migration and tests.
- **Anything structural needs an ADR first.** Discussing intent costs ten
  minutes; undoing an implementation costs ten days.
- Say in the description: what behaviour changes, which test proves it, and
  which security rule it touches.

Maintainers do not merge their own code, founder included.

## Dependencies

Adding a direct dependency is a decision, not a detail. Justify it in the pull
request: what it does, why the standard library is not enough, how maintained it
is. Expect it to be refused.
