# PDS-03: Bootstrap and register a staging hosting node

<!-- pds-ass-roadmap:03 -->

## What to build

An operator creates a staging node from versioned infrastructure configuration and sees its verified identity and usable capacity in management inventory.

## Acceptance criteria

- [ ] Record provider, region, spend ceiling, management/PDS domains and backup destination before provisioning.
- [ ] OpenTofu creates only declared resources; Ansible configures Ubuntu, Docker, private administration, firewall and a restricted agent.
- [ ] Only HTTPS/required certificate validation is public; database and host administration remain private.
- [ ] Node enrollment validates a one-time bootstrap credential and authenticates subsequent agent connections.
- [ ] Infrastructure state and bootstrap secrets are protected; rerunning configuration does not create duplicate nodes.

## Verification

Review the infrastructure plan, bootstrap an authorized staging node, register it and rerun convergence; reject an unknown or replayed node enrollment.

## Blocked by

- PDS-01: Run the standalone local stack and CI smoke journey

## Boundaries

This issue establishes one provider and region; executing paid provisioning requires actual account credentials and an approved budget.

## Planning reference

Phase: M0 - Runnable foundation. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
