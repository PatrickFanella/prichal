CREATE TABLE cli_device_grants (
    device_hash bytea PRIMARY KEY CHECK(octet_length(device_hash)=32),
    user_hash bytea UNIQUE NOT NULL CHECK(octet_length(user_hash)=32),
    requester_hash bytea NOT NULL CHECK(octet_length(requester_hash)=32),
    owner_id uuid REFERENCES owners(id),
    state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','denied','consumed')),
    interval_seconds integer NOT NULL DEFAULT 5 CHECK(interval_seconds BETWEEN 5 AND 600),
    last_poll_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes',
    CHECK(state='pending' OR owner_id IS NOT NULL)
);
CREATE INDEX cli_device_requester ON cli_device_grants(requester_hash,created_at);
CREATE TABLE cli_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea UNIQUE NOT NULL CHECK(octet_length(token_hash)=32),
    owner_id uuid NOT NULL REFERENCES owners(id),
    scope text NOT NULL DEFAULT 'deployments' CHECK(scope='deployments'),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours'
);
CREATE INDEX cli_credentials_owner ON cli_credentials(owner_id);
CREATE TABLE cli_approval_attempts (
    owner_id uuid NOT NULL REFERENCES owners(id),
    attempted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cli_approval_attempts_owner ON cli_approval_attempts(owner_id,attempted_at);
