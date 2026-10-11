# Homelab staging node for issue #3

Use **Kvant**, with a dedicated Ubuntu KVM guest named `prichal-staging-01`, for the next staging hosting node. The owner selected existing homelab hardware instead of a VPS on October 11, 2026. Incremental provider spending is capped at $0; electricity and existing equipment remain household costs. The location is the home LAN. This selection does not enable commercial hosting.

The versioned [node selection](../deploy/homelab/staging-node.json) records the resource ceiling, disk identity, private network boundary and proposed recovery destination. Its status is `selected-not-provisioned`: it is a declaration for subsequent infrastructure work, not an executable provisioner or enrollment receipt.

## Host evidence

Read-only SSH and local observations at October 11, 2026, 13:34 CDT produced:

| Host | Logical CPUs | Available RAM | Relevant free storage | Load average, 1/5/15 minutes | Decision |
| --- | ---: | ---: | --- | --- | --- |
| Kvant | 24 | 36.9 GiB | 420 GiB on `/mnt/fast1`; 1.9 TiB on `/mnt/data2` | 1.24 / 0.79 / 1.11 | Selected; greatest observed compute and SSD headroom, permanent headless application role, working KVM access. |
| Dozor | 4 | 8.3 GiB | 59 GiB on root and `/srv` | 0.88 / 0.71 / 0.71 | Keep the existing qualified PDS pilot here; additional staging competes with monitoring and application services. |
| NUC | 4 | 12.4 GiB | 73 GiB on `/srv` | 2.69 / 2.87 / 3.80 | Preserve the existing SUBCULT PDS and resident applications. |
| Almaz | 8 | 16.9 GiB | 205 GiB on root | 27.66 / 20.56 / 14.39 | Preserve Gitea and public ingress; observed load exceeded CPU count and swap was effectively full. |

These are dated snapshots, not sustained workload benchmarks. Available RAM includes reclaimable caches; free storage does not establish an allocation entitlement. Recheck capacity, mount identity and competing workload limits immediately before provisioning. Kvant's CPU and memory pressure averages were zero in this sample; other workloads, including its CI guest, remain independently owned.

Kvant runs Omarchy and Dozor runs Fedora. NUC and Almaz run Ubuntu. A dedicated Ubuntu guest on Kvant satisfies the intended guest OS without reinstalling an existing host. `/dev/kvm` and `qemu-system-x86_64` are available. The existing `kvant-ci-vm.service` is active; it belongs to Kuznya and must not be reused as a persistent PDS node.

## Initial allocation and boundaries

Start with two virtual CPUs, 4 GiB RAM, a 40 GiB virtual disk and an operator ceiling of one PDS slot. The slot ceiling is conservative policy, not measured tenant capacity. Add slots only after measured workload, disk enforcement and restore qualification. Provision the disk under `/mnt/fast1/apps/prichal-staging` only when the mounted filesystem UUID is `6d24966a-f7db-42e5-b136-e3a6f6c3ac01` and its allocation budget admits the disk. Refuse startup when the intended mount is absent.

Use a dedicated service, disk, bootstrap identity and private state directory outside T3 worktrees. Keep it separate from Kuznya, Subcult OS, existing PDS storage and management PostgreSQL. Protect infrastructure state and bootstrap material with private directories and files; never store secrets in the selection JSON or Git. A second application of configuration must inspect and converge the same named guest, not create another node.

The initial guest network uses QEMU user NAT. Administration and the agent are reachable only through reserved loopback forwards over pinned SSH. The agent requires dedicated mutual TLS; neither the public management API nor a deployment worker receives Docker access or host credentials. Database listeners stay inside the private network. Public ingress, when separately qualified, continues through Almaz and HTTPS. Review host and guest firewall behavior, including Docker forwarding, before exposure; an SSH listener on a homelab address is not evidence of public reachability.

The proposed independent encrypted recovery destination is Dozor's `/srv/recovery/kvant/prichal-staging`. The underlying recovery filesystem reported 763 GiB free. Its existing parent is owned by the retained `containers` account with mode 0750, so a scoped destination directory and backup principal must be configured before writing; do not change the parent owner or existing recovery consumers. Record the encryption key's independent custody and prove a stopped restore before public use. A VM disk copy alone does not establish native PDS or management PostgreSQL consistency.

## Integration sequence and issue status

1. Pin an Ubuntu image and verify its provenance/checksum. Produce a reviewed versioned plan for the single guest and a repeatable Ubuntu/Docker/private administration/firewall/agent configuration. The current repository has no OpenTofu or Ansible implementation; the manifest does not satisfy that acceptance criterion. If retaining QEMU instead of OpenTofu for existing hardware, explicitly revise the infrastructure contract before marking it complete.
2. Reserve private forwards, establish mount checks and a separately supervised guest, then converge twice. Verify the same VM disk, node identity and resources survive the second run. Inspect the running guest, effective firewall and absence of public database/administration exposure.
3. Connect private enrollment to the intended staging management inventory. `node-admin` and `node-enroll` currently require `PDS_MODE=local`, and administration requires the disposable `pds_local` database. Preserve these safeguards. Do not point qualification credentials at production or register the current Dozor pilot through an unsupported adoption path.
4. Verify one-time bootstrap expiry, unknown credentials, replay rejection, retained-key recovery after a lost response and authenticated subsequent agent calls. Confirm inventory displays the enrolled node and its one-slot ceiling.
5. Configure and restore an independent encrypted backup. Select distinct management/PDS staging domains and qualify DNS, HTTPS, mail and federation before opening public enrollment. Domain fields remain explicitly unset until this routing decision; neither existing PDS hostname is reassigned.

Issue #3 stays open. Host selection, spend ceiling, initial allocation and proposed backup destination are recorded. Guest provisioning, repeatable configuration, live enrollment/inventory, backup custody/restore and new public domain selection remain unverified.

The Dozor pilot at `prichal-pds.subcult.tv` remains the current live test service. Its agent was active during inspection. No VM, disk, firewall, service or route was created or changed for this assessment. The NUC service at `pds.subcult.tv` and existing account identities remain in place. Retiring or moving the Dozor pilot requires its own recovery and cutover evidence after the new staging node is qualified.
