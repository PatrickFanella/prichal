CREATE TABLE enrollment_admission (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 open boolean NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Preserve the existing disposable local workflow. Production enrollment
-- remains independently disabled by its HTTP route.
INSERT INTO enrollment_admission(singleton,open) VALUES(true,true);
CREATE TABLE enrollment_admission_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 reason text NOT NULL CHECK(reason IN ('maintenance','safety_review','capacity_review','local_qualification')),
 previous_open boolean NOT NULL,
 requested_open boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(previous_open<>requested_open)
);
