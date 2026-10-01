# Local tenant resource pressure

`qualify-resource-pressure.mjs` runs fixed, bounded probes in the second of two populated disposable local tenants. It requires UUID localhost endpoints, the pinned image, expected labels, private fixture custody and enforced cgroup limits. It never changes Docker limits, uses another image or accepts a shell command.

```sh
node scripts/qualify-resource-pressure.mjs \
  .local/owner-enrollment/ALICE_JOURNEY_UUID \
  .local/owner-enrollment/BOB_JOURNEY_UUID
```

The CPU probe runs two worker threads for five seconds within the tenant's one-CPU quota. The memory probe touches 16 MiB buffers, stopping after ten seconds or 110 allocations. It increases only its own OOM selection score so the kernel can prefer killing the probe. The cgroup bounds RAM to 768 MiB and swap to 768 MiB. Docker's default swap allowance is additional to the RAM limit; see [Docker resource constraints](https://docs.docker.com/engine/containers/resource_constraints/). The script requires these actual limits and records cgroup throttle/limit/OOM counter deltas. It does not disable the OOM killer.

While each probe runs, the neighbor logs in once and repeatedly reads its authenticated session through the retained TLS endpoint. Before and after, a read-only operator helper validates signed repositories and every referenced blob. Container identities and protected credential, TLS, admission and lifecycle files must match. An interrupted drill starts the same target container if it stopped; finite probe lifetimes bound any surviving exec process. The drill does not add native records or change lifecycle intent.

## Actual evidence

The October 1 local drill observed CPU throttling and a cgroup OOM kill with probe exit 137. Alice stayed available during both probes. An initial repeat hit the native login rate limit before pressure; the final drill reuses one session instead of repeatedly logging in. Both tenants retained their signed repositories, referenced media, original containers and protected host state. Private reports are stored under `.local/resource-pressure/` and the exact measurement is recorded in [progress](progress/local-m1-alpha.md).

This proves these configured CPU and memory limits under fixed operator probes on the local Docker Desktop VM. It does not prove isolation against an adversarial tenant, unbounded requests, disk exhaustion or a shared-host kernel/daemon failure. The 128-PID ceiling is checked, not exhausted. Storage/account enforcement, realistic sustained load, network bandwidth controls and independent host qualification remain open for #16 and #19. Existing cross-tenant authorization/network/data checks remain separate evidence.
