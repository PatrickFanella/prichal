# PDS-07: Expose stable managed PDS hostnames with HTTPS

[Gitea issue #7](https://git.subcult.tv/PatrickFanella/pds-ass/issues/7)

<!-- pds-ass-roadmap:07 -->

## What to build

A customer receives a stable provider-managed PDS URL with valid HTTPS and working protocol/WebSocket routing.

## Acceptance criteria

- [ ] Use a registered PDS domain separate from management/OAuth application domains.
- [ ] Automate supported DNS and Caddy configuration for endpoints and required account-handle hostnames.
- [ ] Issue certificates only for verified service inventory; reject unknown hosts and conflicting names.
- [ ] Handle certificate/DNS failures as explicit progress or failure states rather than marking the server ready.
- [ ] Record endpoint identity independently from node placement so recovery can retain the hostname.

## Verification

Reach a staging PDS through its public URL, validate TLS and WebSockets, reject an unregistered hostname, and retry a failed certificate step.

## Blocked by

- #6: Provision a real PDS through the restricted host agent

## Boundaries

Managed hostnames are the default; customer-owned PDS endpoints are a later issue.

## Planning reference

Phase: M1 - Private deployment alpha. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

## Selected namespace and implementation sequence — October 28, 2026

The owner selected first-level names under `subcult.tv`, with no domain purchase. The [namespace policy](../managed-namespace.md) reserves generated `pds-` endpoint and `at-` account labels. #6 is accepted and closed after PR #202. The [implementation sequence](../managed-https-design.md) starts with strict operator-inventory candidate generation and isolated pinned-Caddy validation. Authoritative inventory export, retained runtime identity, scoped publication and public DNS/TLS/WebSocket failure/retry qualification remain open. Candidate generation does not issue names or enable accounts.
