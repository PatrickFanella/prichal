ALTER TABLE deployments ADD COLUMN managed_hostname text;
UPDATE deployments SET managed_hostname=id::text || '.localhost';
ALTER TABLE deployments ALTER COLUMN managed_hostname SET NOT NULL;
ALTER TABLE deployments ADD CONSTRAINT managed_hostname_unique UNIQUE(managed_hostname);
ALTER TABLE deployments ADD COLUMN endpoint_url text;
UPDATE deployments SET endpoint_url='http://' || managed_hostname || ':' || (36000 + (('x' || substr(id::text,1,4))::bit(16)::integer % 10000))::text WHERE kind='agent';
