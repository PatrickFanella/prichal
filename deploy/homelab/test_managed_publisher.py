import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('publisher', Path(__file__).with_name('publish-managed-staging.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PublisherTests(unittest.TestCase):
    def inventory(self):
        return {'version': 1, 'generation': 7, 'expiresAt': (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=10)).isoformat(),
                'endpoints': [{'deploymentId': module.INSTANCE, 'hostname': module.HOST, 'forwardPort': 35000,
                               'certificateSha256': 'a' * 64}], 'handles': []}

    def test_only_fresh_complete_designated_export_is_accepted(self):
        value = self.inventory()
        data = json.dumps(value).encode()
        module.authority(data, module.digest(data))
        changes = [lambda v: v.update(generation=0), lambda v: v.update(generation=True),
                   lambda v: v.update(expiresAt='2000-01-01T00:00:00Z'),
                   lambda v: v.update(expiresAt='2999-01-01T00:00:00Z'),
                   lambda v: v['endpoints'][0].update(hostname='prichal.subcult.tv'),
                   lambda v: v['endpoints'][0].update(forwardPort=34945),
                   lambda v: v.update(handles=[{'hostname': 'unverified.subcult.tv'}]),
                   lambda v: v.update(source='customer'), lambda v: v.update(endpoints=[])]
        for change in changes:
            value = self.inventory()
            change(value)
            data = json.dumps(value).encode()
            with self.assertRaises((ValueError, TypeError)):
                module.authority(data, module.digest(data))
        with self.assertRaises(ValueError):
            module.authority(data, 'b' * 64)

    def test_parent_bytes_are_preserved_and_unknown_imports_refused(self):
        before = b'# Existing pilot\nhttp://pilot.subcult.tv { respond 200 }\n'
        after = module.parent_after(before)
        self.assertTrue(after.startswith(before))
        self.assertEqual(after.count(module.IMPORT), 1)
        self.assertEqual(module.parent_after(after), after)
        for value in [b'# prichal-managed custom config\n', module.IMPORT * 2]:
            with self.assertRaises(ValueError):
                module.parent_after(value)

    def test_safe_custody_refuses_links_and_public_proofs(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'proof'
            path.write_bytes(b'private')
            path.chmod(0o600)
            self.assertEqual(module.read(path, private=True), b'private')
            alias = path.with_name('alias')
            alias.symlink_to(path)
            with self.assertRaises(ValueError):
                module.read(alias)
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                module.read(path, private=True)

    def test_dangling_owned_route_is_not_treated_as_absent(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            parent = root / 'parent'
            parent.write_bytes(b'previous')
            target = root / 'target'
            target.symlink_to(root / 'missing')
            receipt = {'parentBefore': module.digest(b'previous'), 'parentAfter': module.digest(b'after'),
                       'routeBefore': None, 'routeAfter': module.digest(b'route')}
            with patch.object(module, 'PARENT', parent), patch.object(module, 'TARGET', target), patch.object(module.os, 'getuid', return_value=0):
                # Avoid requiring root-owned fixture files while retaining the actual symlink read boundary.
                original = module.read
                with patch.object(module, 'read', side_effect=lambda p, **kwargs: original(p)):
                    with self.assertRaises(ValueError):
                        module.owned_state(receipt)

    def test_active_drift_refuses_before_reload(self):
        with patch.object(module, 'active', return_value=({'changed': True}, 'tag')), patch.object(module, 'run') as run:
            with self.assertRaises(ValueError):
                module.load(Path('/unused'), '/etc/caddy/after.json', {'original': True}, {'managed': True})
            run.assert_not_called()

    def test_only_automatic_staged_main_hiding_is_translated(self):
        stage = '/etc/caddy/sites/.prichal-managed-fixed/Caddyfile'
        before = {'routes': [{'handler': 'file_server', 'hide': [stage, '/existing/private']},
                             {'handler': 'other', 'hide': [stage]}, {'path': stage}]}
        after = module.production_hide_paths(before, stage)
        self.assertEqual(after['routes'][0]['hide'], ['/etc/caddy/Caddyfile', '/existing/private'])
        self.assertEqual(after['routes'][1:], before['routes'][1:])
        self.assertEqual(before['routes'][0]['hide'][0], stage)

    def test_reload_has_atomic_root_etag_precondition(self):
        with patch.object(module, 'active', return_value=({'original': True}, '"/config/ retained"')), patch.object(module, 'read', return_value=b'{"managed":true}'), patch.object(module, 'run') as run:
            module.load(Path('/unused'), '/etc/caddy/after.json', {'original': True}, {'managed': True})
            arguments = run.call_args.args[0]
            self.assertIn('--header=If-Match: "/config/ retained"', arguments)
            self.assertIn('--post-file=/etc/caddy/after.json', arguments)
            self.assertEqual(arguments[-1], 'http://127.0.0.1:2019/config/')

    def test_changed_reload_payload_is_refused(self):
        with patch.object(module, 'active', return_value=({'original': True}, 'tag')), patch.object(module, 'read', return_value=b'{"foreign":true}'), patch.object(module, 'run') as run:
            with self.assertRaises(ValueError):
                module.load(Path('/unused'), '/etc/caddy/after.json', {'original': True}, {'managed': True})
            run.assert_not_called()

    def test_rollback_refuses_unrelated_active_change_without_file_write(self):
        receipt = {'before': {'original': True}, 'after': {'managed': True}}
        with patch.object(module, 'check_sources'), patch.object(module, 'owned_state'), patch.object(module, 'active', return_value=({'other': True}, 'tag')), patch.object(module, 'replace') as replace:
            with self.assertRaises(ValueError):
                module.rollback(Path('/unused'), receipt)
            replace.assert_not_called()


if __name__ == '__main__':
    unittest.main()
