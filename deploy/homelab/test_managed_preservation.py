import copy
import importlib.util
import json
from pathlib import Path
import sqlite3
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('preservation', Path(__file__).with_name('preserve-staging-02-managed.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PreservationTests(unittest.TestCase):
    def containers(self):
        return [{'Id': identity, 'Config': {'Labels': {'tv.subcult.pds-ass.instance': module.INSTANCE},
                                          'Env': ['PDS_HOSTNAME=' + module.HOST, 'PDS_INVITE_REQUIRED=true']},
                 'HostConfig': {'RestartPolicy': {'Name': 'no'}}, 'State': {'Running': index < 2}}
                for index, identity in enumerate(module.IDS)]

    def test_changed_runtime_and_running_predecessor_refuse(self):
        original = self.containers()
        for change in [lambda v: v[0].update(Id='foreign'),
                       lambda v: v[3]['State'].update(Running=True),
                       lambda v: v[0]['HostConfig']['RestartPolicy'].update(Name='always'),
                       lambda v: v[0]['Config'].update(Env=['PDS_HOSTNAME=foreign'])]:
            values = copy.deepcopy(original)
            change(values)
            with patch.object(module, 'run', return_value=json.dumps(values).encode()):
                with self.assertRaises(ValueError):
                    module.inspect()

    def test_populated_fixture_refuses_before_stop_or_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            work = Path(temporary) / 'not-created'
            with patch.object(module, 'inspect', return_value=self.containers()), patch.object(module, 'run', return_value=b'1\n') as run:
                with self.assertRaises(ValueError):
                    module.preserve(work)
                self.assertEqual(run.call_count, 1)
                self.assertEqual(run.call_args.args[0][:2], ['docker', 'exec'])
                self.assertFalse(work.exists())

    def test_real_cold_archive_restores_sqlite_state_keys_and_blobs(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            roots = {name: root / name for name in ['state', 'data', 'blobs', 'node']}
            for path in roots.values():
                path.mkdir()
            connection = sqlite3.connect(roots['data'] / 'account.sqlite')
            connection.execute('CREATE TABLE actor(did TEXT PRIMARY KEY)')
            connection.commit()
            connection.close()
            (roots['state'] / 'credentials.json').write_bytes(b'private fixture state')
            (roots['state'] / '.native-owner.lock').write_bytes(b'')
            (roots['node'] / 'node.key').write_bytes(b'private fixture node key')
            (roots['blobs'] / 'blob').write_bytes(b'fixture blob')
            work = root / 'backup'
            work.mkdir()
            with patch.object(module, 'STATE', roots['state']):
                before = {name: module.inventory(path) for name, path in roots.items()}
                report = module.cold_copy(work, roots)
                self.assertEqual(report['filesVerified'], 4)
                self.assertTrue(report['sourceUnchanged'])
                self.assertEqual(report['sqliteIntegrity'], 'ok')
                for name, path in roots.items():
                    self.assertEqual(module.inventory(path), before[name])
                    self.assertEqual(module.inventory(work / 'restored' / name), before[name])
                self.assertFalse((work / 'restored/state/.native-owner.lock').exists())
                with self.assertRaises(FileExistsError):
                    module.cold_copy(work, roots)

    def test_populated_restored_sqlite_and_unsafe_archive_refuse(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            connection = sqlite3.connect(root / 'account.sqlite')
            connection.executescript("CREATE TABLE actor(did TEXT); INSERT INTO actor VALUES('did:plc:fixture');")
            connection.close()
            with self.assertRaises(ValueError):
                module.verify_sqlite(root)
            (root / 'link').symlink_to(root / 'account.sqlite')
            with self.assertRaises(ValueError):
                module.inventory(root)
        for name, kind in [('data/../../escaped', tarfile.REGTYPE), ('/data/file', tarfile.REGTYPE),
                           ('foreign/file', tarfile.REGTYPE), ('data/link', tarfile.SYMTYPE)]:
            member = tarfile.TarInfo(name)
            member.type = kind
            with self.assertRaises(ValueError):
                module.validate_members([member])

    def test_wal_aware_integrity_read_keeps_comparison_restore_unchanged(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data = root / 'data'
            data.mkdir()
            connection = sqlite3.connect(data / 'account.sqlite')
            try:
                connection.execute('PRAGMA journal_mode=WAL')
                connection.execute('CREATE TABLE actor(did TEXT)')
                connection.commit()
                work = root / 'backup'
                work.mkdir()
                before = module.inventory(data)
                self.assertIn('account.sqlite-wal', before)
                module.cold_copy(work, {'data': data})
                self.assertEqual(module.inventory(data), before)
                self.assertEqual(module.inventory(work / 'restored/data'), before)
            finally:
                connection.close()


if __name__ == '__main__':
    unittest.main()
