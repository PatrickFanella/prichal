# Local operator capacity

The restricted agent defaults to a two-instance ceiling. `PDS_AGENT_MAXIMUM_SLOTS` can explicitly select one through eight before startup. Measured CPU and memory remain hard clamps. Local identity fixtures reserve 960 MiB and 1.375 CPUs per tenant, plus two GiB and one CPU for host overhead. This is an admission policy, not a storage quota or production sizing guarantee.

Management separately retains an operator ceiling. To change it on a registered local node:

```sh
./scripts/dev.sh node-admin capacity --node NODE_UUID --slots 3 --reason local_isolation_qualification
```

Supported reasons are `local_isolation_qualification`, `planned_capacity_change` and `capacity_reduction`. The command requires explicit local mode, a loopback `pds_local` database and an active or draining registered node. It records the operating-system actor and both ceilings. Repeating the same ceiling adds no duplicate event. No management-owner HTTP endpoint exposes this operation.

A change invalidates the node's fresh measurements. Placement remains blocked until an authenticated agent capacity report refreshes management. Retained unmanaged namespaces continue to consume capacity. Lowering the ceiling prevents additional admission without deleting or stopping current instances. The agent and management ceilings can differ; the lower measured availability controls placement.

For the local three-slot qualification, restart the agent with `PDS_AGENT_LOCAL_PROTOCOL_FIXTURES=true PDS_AGENT_MAXIMUM_SLOTS=3 ./scripts/agent-local.sh run`. Returning to the default requires an audited management reduction and an agent restart without the override. Retain existing data and placements during either change.

Storage quotas, disk-pressure admission, verified production headroom and shared-host load qualification remain open.
