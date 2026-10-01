CREATE TABLE owner_admission (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 open boolean NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Preserve existing management sign-in behavior. Live signup opening remains
-- a separate release decision; this policy creates no hosting entitlement.
INSERT INTO owner_admission(singleton,open) VALUES(true,true);
CREATE TABLE owner_admission_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 reason text NOT NULL CHECK(reason IN ('maintenance','safety_review','capacity_review','local_qualification')),
 previous_open boolean NOT NULL,
 requested_open boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(previous_open<>requested_open)
);
