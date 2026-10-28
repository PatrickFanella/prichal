import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('routing_recovery', Path(__file__).with_name('qualify-managed-routing-recovery.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class RecoveryScopeTests(unittest.TestCase):
    def test_qualification_workspace_cannot_escape_private_root(self):
        for value in ['../outside', '/etc', 'managed-conversion/../other']:
            with self.assertRaises(ValueError):
                module.qualified_root(value)
        self.assertEqual(module.qualified_root('managed-conversion'), module.PARENT / 'managed-conversion')
        self.assertEqual(module.qualified_root('managed-readiness'), module.PARENT / 'managed-readiness')
        with self.assertRaises(ValueError):
            module.retained_snapshot('management-pre-upgrade.dump')
        for value in ['../snapshot.dump', '/etc/shadow', 'arbitrary.dump']:
            with self.assertRaises(ValueError):
                module.retained_snapshot(value)
    def test_source_and_foreign_identifiers_cannot_be_dropped(self):
        for value in ['pds_local', 'postgres', 'pds_recovery_other', 'pds_recovery_' + 'a' * 32 + ';DROP DATABASE postgres']:
            with self.assertRaises(ValueError):
                module.recovery_identifier(value)
        self.assertEqual(module.recovery_identifier('pds_recovery_' + 'a' * 32), 'pds_recovery_' + 'a' * 32)

    def test_wrong_registry_address_or_container_refused(self):
        information = {'Config': {'Image': module.IMAGE, 'Labels': {'com.docker.compose.project': 'prichal-staging-02-registry', 'com.docker.compose.service': 'postgres'}}}
        module.validate_scope(information, 'postgres://pds_staging:synthetic@127.0.0.1:35437/pds_local?sslmode=disable')
        for url in ['postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local', 'postgres://pds_staging:synthetic@production.invalid:35437/pds_local', 'postgres://pds_staging:synthetic@127.0.0.1:35437/production', 'postgres://pds_staging:synthetic@127.0.0.1:35437/pds_local?host=production.invalid']:
            with self.assertRaises(ValueError):
                module.validate_scope(information, url)
        with self.assertRaises(ValueError):
            module.validate_scope({'Config': {}}, 'postgres://pds_staging:synthetic@127.0.0.1:35437/pds_local')
