-- Local-only endpoint correction before identity enrollment. Hostname and port remain stable.
UPDATE deployments SET endpoint_url=regexp_replace(endpoint_url,'^http:','https:'),updated_at=now() WHERE kind='agent' AND region='local';
INSERT INTO deployment_events(deployment_id,state,message) SELECT id,state,'Local endpoint now requires fixture TLS. Identity enrollment remains disabled pending endpoint qualification.' FROM deployments WHERE kind='agent' AND region='local';
