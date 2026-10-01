CREATE TABLE owners (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer text NOT NULL,
    subject text NOT NULL,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);
-- Earlier versions have no organization creation endpoint. Refuse to assign
-- any manually inserted organization to an arbitrary new owner.
ALTER TABLE organizations ADD COLUMN owner_id uuid NOT NULL REFERENCES owners(id);
CREATE INDEX organizations_owner_id ON organizations(owner_id);
CREATE TABLE login_attempts (
    state_hash bytea PRIMARY KEY,
    binding_hash bytea NOT NULL,
    nonce text NOT NULL,
    pkce_verifier text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX login_attempts_expiry ON login_attempts(expires_at);
CREATE TABLE management_sessions (
    token_hash bytea PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES owners(id),
    csrf_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX management_sessions_owner ON management_sessions(owner_id);
CREATE INDEX management_sessions_expiry ON management_sessions(expires_at);
