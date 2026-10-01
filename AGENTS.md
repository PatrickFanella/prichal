# Development instructions

Read `/Users/onnwee/.homelab/HOMELAB.md` when available. Use the Blacktower routing skill for substantive coding and apply unslop to prose.

The service is standalone. Do not import Subcult OS or Patchwork application code. Management PostgreSQL is separate from every PDS's SQLite databases and blobs. API code must not acquire Docker access or host credentials; host execution belongs in a restricted agent.

Use the pinned toolchains in `backend/go.mod`, `.node-version` and `web/package.json`. The local verification gate is `./scripts/dev.sh smoke`; it needs the separate local PostgreSQL started by `./scripts/dev.sh db`. `PDS_TEST_ADMIN_URL` must target only disposable local/CI infrastructure, never a production database.

Keep credentials, runtime files and developer logs in ignored `.local/`. Never commit environment files. Preserve unrelated running containers and volumes. Local runtime checks do not prove public DNS, TLS, email or federation. Record evidence and remaining gates in `docs/progress/local-m1-alpha.md`.
