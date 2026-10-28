import copy
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('updates', Path(__file__).with_name('publish-managed-staging.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
PREVIOUS = '9a0b24a6-a60e-4097-af0d-50165df7a131'
NEXT = 'aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa'
OTHER = 'bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb'


class PublisherUpdateTests(unittest.TestCase):
    def inventory(self, generation=8):
        return {'version': 1, 'generation': generation, 'expiresAt': (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=10)).isoformat(),
                'endpoints': [{'deploymentId': module.INSTANCE, 'hostname': module.HOST, 'forwardPort': 35000, 'certificateSha256': 'a' * 64}], 'handles': []}

    def additional(self):
        return {'deploymentId': OTHER, 'hostname': 'pds-' + OTHER.replace('-', '') + '.subcult.tv', 'forwardPort': 35001, 'certificateSha256': 'b' * 64}

    def history(self, root, revision=PREVIOUS, generation=7, rolled_back=False):
        work = root / revision
        work.mkdir(mode=0o700)
        inventory = self.inventory(generation)
        data = json.dumps(inventory).encode()
        receipt = {'version': 1, 'revision': revision, 'generation': generation, 'inventorySHA256': module.digest(data),
                   'parentAfter': module.digest(b'parent'), 'routeAfter': module.digest(b'route')}
        for name, value in [('prepared.json', json.dumps(receipt).encode()), ('inventory.json', data), ('published.json', b'{"published":true}')]:
            (work / name).write_bytes(value)
            (work / name).chmod(0o600)
        if rolled_back:
            (work / 'rolled-back.json').write_text('{"rolledBack":true}')
        return receipt

    def custody(self):
        # Retain actual lstat/type/permission/hash checks without requiring root-owned test files.
        original = module.read
        return patch.object(module, 'read', side_effect=lambda p, *args, **kwargs: original(p, *args, private=kwargs.get('private', False)))

    def test_complete_multi_endpoint_inventory_and_legacy_scope(self):
        value = self.inventory()
        value['endpoints'].append(self.additional())
        data = json.dumps(value).encode()
        module.authority(data, module.digest(data))
        with self.assertRaises(ValueError):
            module.authority(data, module.digest(data), fixture_only=True)
        for change in [lambda v: v['endpoints'].append(copy.deepcopy(v['endpoints'][0])),
                       lambda v: v['endpoints'][1].update(forwardPort=35000),
                       lambda v: v.update(version=True), lambda v: v.update(handles=[{'hostname': 'unverified'}])]:
            invalid = copy.deepcopy(value)
            change(invalid)
            data = json.dumps(invalid).encode()
            with self.assertRaises(ValueError):
                module.authority(data, module.digest(data))

    def test_updates_preserve_retained_identity_and_forward(self):
        before, after = self.inventory(7), self.inventory(8)
        after['endpoints'][0]['certificateSha256'] = 'c' * 64
        after['endpoints'].append(self.additional())
        module.validate_transition(before, after)
        for change in [lambda v: v.update(generation=7), lambda v: v.update(endpoints=[self.additional()]),
                       lambda v: v['endpoints'][0].update(forwardPort=35002),
                       lambda v: v['endpoints'][0].update(hostname='foreign.subcult.tv')]:
            invalid = copy.deepcopy(after)
            change(invalid)
            with self.assertRaises(ValueError):
                module.validate_transition(before, invalid)

    def test_history_adoption_checks_digest_and_applied_marker(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.history(root)
            with patch.object(module, 'ROOT', root), patch.object(module, 'directory'), self.custody():
                receipt, value = module.applied_inventory(PREVIOUS)
                self.assertEqual(receipt['generation'], value['generation'])
                (root / PREVIOUS / 'inventory.json').write_bytes(b'{}')
                with self.assertRaises(ValueError):
                    module.applied_inventory(PREVIOUS)

    def test_rollback_does_not_permit_generation_reuse_or_adoption(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.history(root)
            self.history(root, NEXT, 8, rolled_back=True)
            with patch.object(module, 'ROOT', root), patch.object(module, 'directory'), self.custody():
                self.assertEqual(module.highest_published_generation(), 8)
                with self.assertRaises(ValueError):
                    module.applied_inventory(NEXT)
                with self.assertRaises(ValueError):
                    module.validate_history_generation(self.inventory(8))
                module.validate_history_generation(self.inventory(9))

    def test_prepare_cannot_adopt_an_unknown_or_changed_managed_route(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            bundle = root / 'bundle'
            (bundle / 'certificates').mkdir(parents=True)
            certificate = b'public fixture'
            value = self.inventory()
            value['endpoints'][0]['certificateSha256'] = module.digest(certificate)
            data = json.dumps(value).encode()
            (bundle / 'inventory.json').write_bytes(data)
            (bundle / 'certificates' / (module.INSTANCE + '.crt')).write_bytes(certificate)
            for path in bundle.rglob('*'):
                if path.is_file():
                    path.chmod(0o600)
            parent, target = root / 'parent', root / 'target'
            parent.write_bytes(b'parent')
            target.write_bytes(b'foreign managed route')
            with patch.object(module, 'ROOT', root), patch.object(module, 'PARENT', parent), patch.object(module, 'TARGET', target), patch.object(module, 'directory'), self.custody(), patch.object(module, 'caddy') as caddy:
                with self.assertRaises(ValueError):
                    module.prepare(root / 'unused', NEXT, bundle, module.digest(data))
                self.history(root)
                with self.assertRaises(ValueError):
                    module.prepare(root / 'unused', NEXT, bundle, module.digest(data), PREVIOUS)
                caddy.assert_not_called()

    def test_update_preparation_preserves_current_unrelated_baseline_and_live_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base = root / 'config'
            (base / 'sites').mkdir(parents=True)
            (base / 'certs').mkdir()
            parent, target = base / 'sites/prichal.Caddyfile', base / 'sites/prichal-managed.Caddyfile'
            parent_bytes = b'# Existing services\n' + module.IMPORT
            parent.write_bytes(parent_bytes)
            target.write_bytes(b'route')
            main = b'import /etc/caddy/sites/prichal.Caddyfile\n'
            (base / 'Caddyfile').write_bytes(main)
            history = root / 'history'
            history.mkdir(mode=0o700)
            old = self.history(history)
            old['parentAfter'] = module.digest(parent_bytes)
            old['after'] = {'unrelated': 'prior baseline'}
            (history / PREVIOUS / 'prepared.json').write_text(json.dumps(old))
            bundle = root / 'bundle'
            (bundle / 'certificates').mkdir(parents=True)
            inventory = self.inventory(8)
            inventory['endpoints'].append(self.additional())
            for endpoint in inventory['endpoints']:
                cert = endpoint['deploymentId'].encode()
                endpoint['certificateSha256'] = module.digest(cert)
                path = bundle / 'certificates' / (endpoint['deploymentId'] + '.crt')
                path.write_bytes(cert)
                path.chmod(0o600)
            data = json.dumps(inventory).encode()
            (bundle / 'inventory.json').write_bytes(data)
            (bundle / 'inventory.json').chmod(0o600)
            work = history / NEXT
            work.mkdir(mode=0o700)
            current, candidate = {'unrelated': 'current baseline'}, {'unrelated': 'current baseline', 'managed': 'new'}
            def render(arguments, work, label):
                output = Path(arguments[arguments.index('--out') + 1])
                output.write_bytes(b'next route')
                output.chmod(0o600)
                return b'', b''
            def caddy(work, command, path):
                return json.dumps(current if path == '/etc/caddy/Caddyfile' else candidate).encode()
            with patch.object(module, 'ROOT', history), patch.object(module, 'BASE', base), patch.object(module, 'PARENT', parent), patch.object(module, 'TARGET', target), patch.object(module, 'directory'), self.custody(), patch.object(module, 'sources', return_value={'Caddyfile': module.digest(main)}), patch.object(module, 'active', return_value=(current, 'tag')), patch.object(module, 'caddy', side_effect=caddy), patch.object(module, 'run', side_effect=render), patch.object(module, 'verify_upstreams') as verify:
                module.prepare(work, NEXT, bundle, module.digest(data), PREVIOUS)
                receipt = json.loads((work / 'prepared.json').read_text())
                self.assertEqual(receipt['before'], current)
                self.assertEqual(receipt['after'], candidate)
                self.assertEqual(receipt['previousRevision'], PREVIOUS)
                self.assertEqual(parent.read_bytes(), parent_bytes)
                self.assertEqual(target.read_bytes(), b'route')
                self.assertEqual(len(verify.call_args.args[1]['endpoints']), 2)
                failed_revision = 'cccccccc-cccc-4ccc-accc-cccccccccccc'
                failed = history / failed_revision
                failed.mkdir(mode=0o700)
                verify.side_effect = ConnectionError('second endpoint unavailable')
                with self.assertRaises(ConnectionError):
                    module.prepare(failed, failed_revision, bundle, module.digest(data), PREVIOUS)
                self.assertFalse((failed / 'prepared.json').exists())
                self.assertEqual(parent.read_bytes(), parent_bytes)
                self.assertEqual(target.read_bytes(), b'route')

    def test_old_rollback_refuses_a_newer_installed_route(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            parent, target = root / 'parent', root / 'target'
            parent.write_bytes(b'parent')
            target.write_bytes(b'newer route')
            receipt = {'parentBefore': module.digest(b'parent'), 'parentAfter': module.digest(b'parent'),
                       'routeBefore': module.digest(b'old'), 'routeAfter': module.digest(b'current')}
            with patch.object(module, 'PARENT', parent), patch.object(module, 'TARGET', target), self.custody(), patch.object(module, 'check_sources'), patch.object(module, 'replace') as replace:
                with self.assertRaises(ValueError):
                    module.rollback(root / 'unused', receipt)
                replace.assert_not_called()

    def test_expired_or_wrong_preceding_tls_refuses_before_rollback_write(self):
        receipt = {'previousRevision': PREVIOUS, 'before': {'old': True}, 'after': {'new': True}, 'routeBefore': 'old'}
        with patch.object(module, 'check_sources'), patch.object(module, 'owned_state'), patch.object(module, 'active', return_value=({'new': True}, 'tag')), patch.object(module, 'applied_inventory', return_value=({'routeAfter': 'old'}, self.inventory(7))), patch.object(module, 'verify_upstreams', side_effect=ValueError('TLS no longer valid')), patch.object(module, 'replace') as replace:
            with self.assertRaises(ValueError):
                module.rollback(Path('/unused'), receipt)
            replace.assert_not_called()

    def test_cli_confirmation_and_previous_revision_are_checked_before_host_state(self):
        cases = [ ['prepare', '--revision', NEXT],
                 ['prepare', '--revision', NEXT, '--confirm-staging-02', '--confirm-managed-inventory'],
                 ['prepare', '--revision', NEXT, '--previous-revision', NEXT, '--confirm-managed-inventory'],
                 ['publish', '--revision', NEXT, '--bundle', '/foreign', '--confirm-managed-inventory'] ]
        before = os.umask(0o077)
        try:
            for args in cases:
                with patch.object(sys, 'argv', ['publisher'] + args), patch.object(module.os, 'geteuid', return_value=0), patch.object(module.socket, 'gethostname', return_value='almaz'), patch.object(module.Path, 'mkdir') as mkdir:
                    with self.assertRaises(ValueError):
                        module.main()
                    mkdir.assert_not_called()
        finally:
            os.umask(before)


if __name__ == '__main__':
    unittest.main()
