# PDS-24: Finish accessible browser onboarding and lifecycle guidance

[Gitea issue #24](https://git.subcult.tv/PatrickFanella/pds-ass/issues/24)

<!-- pds-ass-roadmap:24 -->

## What to build

A new customer can understand plans, deploy, return to progress, manage a domain, inspect usage and find export/cancellation actions on a phone or desktop.

## Acceptance criteria

- [ ] Use plain product language and make pending, retryable and operator-required states distinct.
- [ ] Support keyboard navigation, labeled controls, visible focus and screen-reader announcements for progress.
- [ ] Validate desktop, tablet, phone and landscape layouts with real overflow checks.
- [ ] Show account enrollment, ownership, domain and recovery boundaries at the relevant decision point.
- [ ] Exercise failure recovery and refreshed/reopened browser sessions without losing deployment context.

## Verification

Run the supported end-to-end browser journeys at representative widths and with keyboard/accessibility checks; include DNS, payment and provisioning failures.

## Blocked by

- #10: Attach and verify a customer-owned domain handle
- #19: Show usage and enforce plan limits without surprise loss
- #21: Export data and cancel hosting with a safe exit window

## Boundaries

No framework rewrite or general website builder; this refines the minimal flows already delivered.

## Planning reference

Phase: M4 - Public launch. Priority: P1.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
