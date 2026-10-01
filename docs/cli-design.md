# Customer CLI command contract

The standalone Go binary uses browser approval for a ten-minute device request. The browser shows the code and permissions before approval. A credential lasts 24 hours, reads the owner's organizations and servers, and submits deployments. It cannot create organizations, obtain PDS invitations, change billing, or perform host administration. Credentials can be revoked individually or with all devices. Restoring management state revokes copied credentials and grants.

This is a management-specific JSON protocol informed by RFC 8628, not a general OAuth authorization server. Device and credential secrets remain outside URLs, command arguments, and terminal output. Local HTTP is permitted only on loopback; a live endpoint requires verified HTTPS.

Command help will describe these commands and flags:

- `pds-ass login --api-url URL [--config-dir DIR] [--no-browser]`: print the verification URL and human code to stderr, wait for approval, save the credential privately, and emit a secret-free confirmation.
- `pds-ass organizations list [--config-dir DIR]`: list owned organizations as JSON.
- `pds-ass deploy --organization UUID --name NAME --idempotency-key UUID [--kind agent|test] [--wait] [--config-dir DIR]`: persist the request before submission; reuse the same key and payload after interruption. Prompt for missing values only in an interactive terminal.
- `pds-ass list --organization UUID [--config-dir DIR]`: list owned deployments as JSON.
- `pds-ass status --deployment UUID [--wait] [--config-dir DIR]`: inspect or wait for server-side work without cancelling it on Ctrl+C.
- `pds-ass logout [--config-dir DIR]`: revoke the current credential before removing local credentials.
- `pds-ass completion bash|zsh|fish`: emit shell completions.
- `pds-ass --help`, command `--help`, and `pds-ass --version`: work without credentials or network access.

stdout contains JSON results or explicitly requested help/completions. Diagnostics go to stderr. Exit codes: 0 success, 1 request or authentication failure, 2 invalid arguments, 3 deployment pending, 4 deployment failed, 130 interruption. No ANSI output is needed. An interrupted POST is reconciled by repeating its stable key, not by generating a new deployment.

Platform builds and checksums can be prepared locally. Runtime testing and publishing each supported platform remain release gates. No public management domain has been selected.
