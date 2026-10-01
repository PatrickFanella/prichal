ALTER TABLE service_operations ADD COLUMN kind text NOT NULL DEFAULT 'service' CHECK(kind IN ('service','account'));
