# pds-ass

A standalone managed AT Protocol PDS hosting service. A customer signs in, names their server, and deploys it on our infrastructure without administering a machine. Browser onboarding is the first delivery path; a CLI can later use the same management API.

The initial target is a usable PDS within 15 minutes on a provider-managed domain. This is a target to measure, not a verified service guarantee. Custom DNS and existing-account migration have separate completion criteria.

## Planning

- [Implementation plan](docs/implementation-plan.md)
- [Roadmap and release gates](docs/roadmap.md)
- [Issue index](docs/issues/README.md)
- [Gitea issues](https://git.subcult.tv/PatrickFanella/pds-ass/issues)

The planned stack is Go, PostgreSQL, River jobs, React with TypeScript, the official Bluesky PDS container, Docker, Caddy, OpenTofu, and Ansible. PDS SQLite databases and blobs remain separate from the management database.

The repository now includes a local Go API, React interface, PostgreSQL schema, OIDC owner sign-in, organization isolation and smoke checks. See [local development](docs/local-development.md) for startup and verification, and [local PDS qualification](docs/local-pds-runtime.md) for the separate upstream runtime. Public PDS hosting is not available yet. Earlier research explored hosting Subcult OS and Patchwork applications. The implementation plan supersedes that product scope: this service has its own codebase, authentication, database, and deployment lifecycle.
