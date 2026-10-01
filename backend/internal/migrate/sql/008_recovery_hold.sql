CREATE TABLE management_recovery (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 snapshot_sha256 bytea NOT NULL CHECK(octet_length(snapshot_sha256)=32),
 state text NOT NULL CHECK(state IN ('held','reconciled')),
 held_at timestamptz NOT NULL DEFAULT now(),
 reconciled_at timestamptz,
 CHECK((state='held' AND reconciled_at IS NULL) OR (state='reconciled' AND reconciled_at IS NOT NULL))
);
