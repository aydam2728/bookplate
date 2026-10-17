# Security policy

## Reporting a vulnerability

**Do not open a public issue.** Use the private vulnerability reporting form on
this repository, under Security, then Report a vulnerability.

- Acknowledgement within **72 hours**
- An assessment and a planned fix date within **7 days**
- Credit in the release notes if you want it

This is a volunteer project with no bug bounty. It will still treat your report
seriously, because instances run in people's homes.

## Supported versions

Pre-alpha: nothing is supported yet, because nothing is released yet.
From 1.0 on, the latest minor version receives security fixes.

## Threat model

The working assumption is the most hostile realistic one: **the instance is
published on the internet** behind a reverse proxy, because checking a book from
a bookshop requires it, and household passwords are weak. A LAN-only instance is
then safe as a by-product, never the other way round.

In scope:

- Cross-library data access and leakage of private reviews
- Server-side request forgery through cover fetching and metadata providers
- Stored cross-site scripting through metadata fetched from public, publicly
  editable APIs
- Authentication: brute force, credential stuffing, session handling
- Privilege escalation through invitations
- Formula injection in CSV export
- Container escape and host compromise

Out of scope:

- Attacks requiring physical access to the server
- Denial of service through sheer bandwidth
- An instance deliberately run without TLS or with a shared admin password

## Verifying what you run

Every published image carries a provenance attestation signed through Sigstore
and bound to the workflow that built it. No maintainer holds a signing key, so
there is no key to steal.

Before running an image, check that the digest you are about to pull really
came from this repository:

```
gh attestation verify oci://ghcr.io/aydam2728/bookplate:... \n  --repo aydam2728/bookplate
```

A software bill of materials is attached to the image itself rather than to a
release asset, so it travels with whatever you actually pulled.

Nothing is published yet. These commands become useful at the first release.

## Hardening expectations

The shipped compose file is hardened by default: non-root user, read-only root
filesystem, all capabilities dropped, no-new-privileges. TLS is delegated to a
reverse proxy and is **required**: camera scanning does not work without a
secure context, which is deliberate.
