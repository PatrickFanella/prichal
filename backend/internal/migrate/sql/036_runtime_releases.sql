CREATE TABLE instance_runtime_releases (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 image_ref text CHECK(length(image_ref)<=256),
 version text CHECK(length(version)<=128),
 image_id text CHECK(image_id ~ '^sha256:[a-f0-9]{64}$'),
 checked_at timestamptz NOT NULL DEFAULT now(),
 CHECK((image_ref IS NULL) = (image_id IS NULL))
);
