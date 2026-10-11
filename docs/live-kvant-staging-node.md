# Kvant staging node qualification, October 11, 2026

The owner authorized the homelab selection and bootstrap in issue #3. The dedicated Ubuntu guest `prichal-staging-01` now runs on Kvant, separately from Kuznya. No VPS was created and incremental provider spending is $0. The existing Dozor pilot and NUC SUBCULT PDS were not moved.

## Installed node and private transport

The dated Ubuntu 24.04 image is `noble/20260926/noble-server-cloudimg-amd64.img`, SHA-256 `6a81c37564db9b1ee84e141922625e1d7c5b389b99bb3c572e0243607d5bb4d2`. Its checksum file passed GPG verification against the Ubuntu cloud signing fingerprint `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`, identified in [Ubuntu's checksum verification documentation](https://ubuntu.com/docs/public-images/public-images-how-to/verify-image-checksum/). The booted guest reports Ubuntu 24.04.5 LTS.

The guest has **three vCPUs, 4 GiB RAM and an independent 40 GiB qcow2 disk**. The original two-vCPU proposal would produce zero slots under the current agent's CPU-overhead formula; three vCPUs admit one slot. Docker capacity reported three CPUs, about 3.8 GiB RAM, one maximum slot and no instance namespaces. This is a verified empty staging node, not a deployed native PDS.

State lives at `/mnt/fast1/apps/prichal-staging`, outside T3 worktrees, with mode 0700 and private disk/seed/key files. Startup refuses a missing or incorrect SSD UUID and verifies the independent disk's virtual size. The enabled user service `prichal-staging-vm.service` uses the copied `vm-control.py`; it requests ACPI shutdown on stop. Kvant user lingering is enabled. Host power-loss/physical reboot recovery remains untested.

QEMU publishes only SSH at `127.0.0.1:22250`. `prichal-staging-agent-tunnel.service` uses the dedicated operator key and pre-generated, pinned host key to forward host `127.0.0.1:22251` to guest loopback `127.0.0.1:9091`. The agent refuses non-loopback listeners and requires mutual TLS. The guest's firewall denies inbound/routed traffic by default and admits administration from the QEMU host gateway; no host public listener, public PDS route or database publish was created. Docker has no PDS containers or published ports in this guest.

The versioned [VM operator](../deploy/homelab/staging-vm.py), [guest Ansible configuration](../deploy/homelab/guest.yaml), [restricted agent unit](../deploy/homelab/staging-agent.service) and [private registry Compose declaration](../deploy/homelab/registry.compose.yaml) describe this slice. QEMU is the existing-hardware provisioner for this owner-approved VM; OpenTofu/cloud provisioning remains deferred. Ansible core 2.19.5 configured Docker, the non-login `prichal-agent` account and firewall. Repeating convergence on the final three-vCPU guest produced `changed=0`, `unreachable=0`, `failed=0`. Repeating VM bootstrap retained the disk, keys and VM identity.

The initial installed filename `operator.py` shadowed Python's standard-library module and prevented boot. It was replaced with `vm-control.py` before the first guest boot. Its private pre-change receipt and source are retained. The corrected guest subsequently booted and recovered after a clean stop/start with unchanged enrolled identity.

## Enrollment and inventory boundary

Node identity is `1ef77d45-c6d5-4bdb-b77b-ba1103b45a66`. The private key was generated inside the guest; the signer retained only public enrollment information in management PostgreSQL. The agent and enrollment binaries were built with pinned Go 1.27.1 from commit `413ffc5b5347b100d5304b97ba6692c15508deb1`; subsequent changes here are infrastructure operators and evidence.

The separate qualification registry is `prichal-staging-registry-postgres-1`, bound only at host `127.0.0.1:35435`, with an independent volume and 512 MiB/one-CPU limits. Its database deliberately remains named `pds_local`: node administration still enforces local mode and that database name. The authority is `prichal-staging-enrollment.service`, listening at `127.0.0.1:9092`. Its private signer and configuration live under `~/.local/state/prichal-staging-control/`; the public API and guest agent receive no CA private key or host credentials.

An SSH reverse forward temporarily connected the guest's loopback enrollment client to that authority. One-time enrollment succeeded. Inventory retained exactly one active node, its endpoint `https://127.0.0.1:22251`, operator/slot ceiling one and public-key fingerprint. Unknown node IDs, unknown tokens and replay of the consumed credential returned 401. Retrying with the same node key retrieved the existing certificate without creating another identity. The retained certificate SHA-256 is `c1106e768272de03b001e87fe74aab629163f835c7fd55f25264cbee93da4eae`.

Authenticated agent capacity succeeded; a client without a certificate failed at TLS. A Python verification probe encountered stricter certificate-extension requirements, so the live enrollment rejection probes used curl's verified HTTPS transport rather than disabling TLS verification. The existing Go enrollment/agent paths and curl verified the dedicated trust root. PKI profile/rotation compatibility remains part of subsequent staging enrollment work.

**The node certificate expires October 12, 2026 at 18:48:30 UTC.** The qualification authority/worker credentials have seven-day lifetimes. Certificate renewal is not installed for this node. Do not call this unattended hosting or connect public accounts while those lifecycle gates remain open. No production management UI was redeployed or made capable of provisioning this guest; inventory verification is against the separate qualification registry.

## Independent encrypted recovery

A cleanly stopped VM and a consistent logical management database dump were archived with operator/PKI state and unit files, then encrypted with AES-256-GCM using the versioned [recovery operator](../deploy/homelab/recovery-crypto.py). Private plaintext is published only after tag and expected archive digest verification.

The independent ciphertext is on Dozor at `/srv/recovery/kvant/prichal-staging/staging-20261011.pvm`, mode 0600 in a dedicated 0700 directory. Its ciphertext SHA-256 is `02b5b21a177efaf31ce2fb45b7f63243d16163ba425daabbf8713035aa112389`. The separate recovery key is retained in private `~/.local/state/prichal-staging-recovery/keys/node-1ef77d45.key` on Kvant and Dozor; no key value is in Git or this evidence. The authenticated plaintext archive digest is `708fa47735ecd6ba30194e03281b998dabe4ddb593149cf1b5800d3018b69fd0`.

The archive was retrieved from Dozor, decrypted and checked with `qemu-img check`. Its restored guest booted under a transient qualification service with QEMU `restrict=yes`, no public listener and no agent connection to management. The same certificate hash, Docker and agent services were present, and there were no PDS containers. A separate PostgreSQL restore retained the active node, one-slot ceiling and consumed bootstrap credential. The temporary restored database was removed after checking. The restored VM was shut down; its private disk fixture remains outside production paths. The live staging VM recovered and reported one usable slot afterward.

This proves recovery of the empty VM/enrollment registry. It does not prove populated PDS recovery, scheduled backups, full host-loss cutover or new public-domain routing. Native PDS/management data must remain separate in subsequent recovery procedures. Stop the tunnel before stopping the VM for disk snapshots; otherwise its restart dependency can bring the VM back up.

## Verification and remaining issue gates

- Seven local operator tests cover wrong mount/foreign state refusal, retained disk/key retries, encrypted round trips, overwrite refusal, corruption, wrong archive scope and key permissions.
- Guest Ansible syntax/convergence and live verified enrollment/authentication/capacity checks passed.
- Full `./scripts/dev.sh smoke` passed with 44 Node checks, frontend build, Go vet and all race-enabled packages against the separate disposable qualification database.
- The installed VM, tunnel and authority are independent of this worktree. No existing Docker service, volume or CI guest was replaced. Existing Kuznya PID `930757` was retained through this slice.

Issue #3 remains open for staging management integration beyond the local qualification registry, certificate renewal/PKI profile review, public staging domain selection, scheduled recovery and physical boot qualification. Domains remain unset; neither existing PDS hostname is reassigned. The original OpenTofu acceptance wording is superseded for this owned-hardware bootstrap by the owner-approved QEMU/Ansible path; cloud adapters remain future work.

For a bounded pause, stop `prichal-staging-agent-tunnel.service`, then `prichal-staging-vm.service` and `prichal-staging-enrollment.service`; retain disks, registry volume and private keys. Do not remove or reset Kuznya, Dozor or NUC resources. Resume the named services after checking certificate validity, mount identity and registry custody. Restoring old keys or database state over a running node requires a separate reconciliation decision.
