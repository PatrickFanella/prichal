import importlib.util
from pathlib import Path
import sqlite3
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location('managed_conversion', Path(__file__).with_name('convert-staging-02-managed.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ConversionBoundaryTests(unittest.TestCase):
    def test_missing_inventory_is_distinct_from_unavailable_daemon(self):
        missing = SimpleNamespace(returncode=1, stderr='error: no such object: fixed-name', stdout='[]')
        with patch.object(module.subprocess, 'run', return_value=missing), patch.object(module, 'run'):
            self.assertIsNone(module.inspect('fixed-name'))
        unavailable = SimpleNamespace(returncode=1, stderr='daemon permission denied', stdout='')
        with patch.object(module.subprocess, 'run', return_value=unavailable), patch.object(module, 'run'):
            with self.assertRaises(RuntimeError):
                module.inspect('fixed-name')
    def test_archive_rejects_links_and_foreign_paths(self):
        for name in ['/etc/shadow', '../data/account.sqlite', 'state/../../foreign', 'foreign/data']:
            with self.assertRaises(ValueError):
                module.validate_members([tarfile.TarInfo(name)])
        for kind in [tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.CHRTYPE]:
            member = tarfile.TarInfo('data/account.sqlite')
            member.type = kind
            with self.assertRaises(ValueError):
                module.validate_members([member])
        module.validate_members([tarfile.TarInfo('data/account.sqlite')])

    def test_cold_restore_requires_sqlite_integrity_and_zero_accounts(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root)
            db = sqlite3.connect(path / 'account.sqlite')
            db.execute('CREATE TABLE actor(did text)')
            db.commit()
            module.verify_sqlite(path)
            db.execute("INSERT INTO actor VALUES('fixture')")
            db.commit()
            with self.assertRaises(ValueError):
                module.verify_sqlite(path)
            db.close()

    def test_inventory_rejects_symlink_custody(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root)
            (path / 'regular').write_text('retained')
            before = module.inventory(path)
            self.assertEqual(set(before), {'regular'})
            (path / 'alias').symlink_to(path / 'regular')
            with self.assertRaises(ValueError):
                module.inventory(path)

    def test_wrong_container_cannot_be_selected(self):
        for value in [None, {'Id': 'foreign', 'Config': {'Image': module.PDS_IMAGE}}]:
            with self.assertRaises(ValueError):
                module.validate_container(value, 'native')

    def test_predecessor_cannot_restart_without_operator(self):
        value = {'Id': '38cefcb10934', 'Config': {'Image': module.EDGE_IMAGE,
                 'Labels': {module.LABEL: module.DEPLOYMENT}},
                 'HostConfig': {'RestartPolicy': {'Name': 'no'}}, 'State': {'Running': False}}
        module.validate_container(value, 'edge', False)
        for policy in ['always', 'unless-stopped', 'on-failure', '']:
            value['HostConfig']['RestartPolicy']['Name'] = policy
            with self.assertRaises(ValueError):
                module.validate_container(value, 'edge', False)


if __name__ == '__main__':
    unittest.main()
