import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('withdrawal', Path(__file__).with_name('publish-managed-staging.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
INITIAL = '9a0b24a6-a60e-4097-af0d-50165df7a131'
APPLIED = 'aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa'
WITHDRAWAL = 'cccccccc-cccc-4ccc-accc-cccccccccccc'
OTHER = 'bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb'
ORIGINAL_PARENT = b'existing.subcult.tv {\n\trespond 200\n}\n'
ROUTE = ('pds-' + module.INSTANCE.replace('-', '') + '.subcult.tv {\n\treverse_proxy 172.20.0.1:35000\n}\n').encode()


class FakeCaddy:
    """Adapts the owned import/route pair into a host list; unrelated inputs stay pinned by real source hashes."""

    def __init__(self, base):
        self.base = base
        self.fail_load = False
        self.loads = 0
        self.config = self.adapt_files(module.PARENT.read_bytes(), module.TARGET.exists())

    def adapt_files(self, parent, route):
        return {'apps': {'http': {'hosts': [module.HOST] if b'prichal-managed' in parent and route else ['existing.subcult.tv']}}}

    def caddy(self, work, command, config):
        if config == '/etc/caddy/Caddyfile':
            value = self.adapt_files(module.PARENT.read_bytes(), module.TARGET.exists())
        else:
            stage = self.base / Path(config).parent.relative_to('/etc/caddy')
            parent = (stage / 'parent.Caddyfile').read_bytes()
            value = self.adapt_files(parent, b'route.Caddyfile' in parent)
        return json.dumps(value).encode()

    def active(self, work):
        return json.loads(json.dumps(self.config)), '"tag"'

    def load(self, work, path, expected, desired):
        self.loads += 1
        if self.config != expected:
            raise ValueError('stale')
        if self.fail_load:
            self.fail_load = False
            raise RuntimeError('injected reload failure')
        self.config = desired


class WithdrawalTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        root = Path(self.temporary.name)
        self.root, self.base = root / 'publisher', root / 'caddy'
        self.root.mkdir(mode=0o700)
        (self.base / 'sites').mkdir(parents=True)
        (self.base / 'certs/other').mkdir(parents=True)
        (self.base / 'Caddyfile').write_bytes(b'import /etc/caddy/sites/prichal.Caddyfile\n')
        (self.base / 'certs/other/site.crt').write_bytes(b'unrelated trust')
        parent, target = self.base / 'sites/prichal.Caddyfile', self.base / 'sites/prichal-managed.Caddyfile'
        installed = module.parent_after(ORIGINAL_PARENT)
        parent.write_bytes(installed)
        target.write_bytes(ROUTE)
        self.patches = [patch.object(module, 'ROOT', self.root), patch.object(module, 'BASE', self.base),
                        patch.object(module, 'PARENT', parent), patch.object(module, 'TARGET', target),
                        patch.object(module, 'directory'), patch.object(module, 'verify_upstreams'),
                        # Only rollback's retained-JSON validation reaches the raw command runner.
                        patch.object(module, 'run', return_value=(b'', b''))]
        for item in self.patches:
            item.start()
        original = module.read
        self.patches.append(patch.object(module, 'read', side_effect=lambda p, *args, **kwargs: original(p, *args, private=kwargs.get('private', False))))
        self.patches[-1].start()
        self.history(INITIAL, '', 7, ORIGINAL_PARENT, None, installed)
        self.history(APPLIED, INITIAL, 9, installed, ROUTE, installed)
        self.fake = FakeCaddy(self.base)
        for name in ('caddy', 'active', 'load'):
            item = patch.object(module, name, side_effect=getattr(self.fake, name))
            item.start()
            self.patches.append(item)

    def tearDown(self):
        for item in reversed(self.patches):
            item.stop()
        self.temporary.cleanup()

    def history(self, revision, previous, generation, parent_before, route_before, parent_after, endpoints=None):
        work = self.root / revision
        work.mkdir(mode=0o700)
        inventory = {'version': 1, 'generation': generation, 'expiresAt': dt.datetime.now(dt.timezone.utc).isoformat(),
                     'endpoints': endpoints or [{'deploymentId': module.INSTANCE, 'hostname': module.HOST, 'forwardPort': 35000, 'certificateSha256': 'a' * 64}],
                     'handles': []}
        data = json.dumps(inventory).encode()
        receipt = {'version': 2, 'revision': revision, 'previousRevision': previous, 'generation': generation, 'inventorySHA256': module.digest(data),
                   'parentBefore': module.digest(parent_before), 'parentAfter': module.digest(parent_after),
                   'routeBefore': module.digest(route_before) if route_before is not None else None, 'routeAfter': module.digest(ROUTE)}
        files = [('prepared.json', json.dumps(receipt).encode()), ('inventory.json', data), ('published.json', b'{"published":true}\n'),
                 ('parent-before', parent_before)]
        for name, value in files:
            (work / name).write_bytes(value)
            (work / name).chmod(0o600)

    def prepare(self):
        work = self.root / WITHDRAWAL
        work.mkdir(mode=0o700)
        result = module.prepare_withdrawal(work, WITHDRAWAL, APPLIED)
        self.assertEqual(result, {'prepared': True, 'revision': WITHDRAWAL, 'withdrawal': True, 'liveChanged': False})
        self.assertEqual(self.fake.loads, 0)
        return work, json.loads((work / 'prepared.json').read_bytes())

    def test_withdrawal_restores_initial_parent_and_tombstones_identity(self):
        work, receipt = self.prepare()
        result = module.withdraw(work, receipt)
        self.assertTrue(result['withdrawn'])
        self.assertFalse(result['publicAbsenceQualified'])
        self.assertEqual(module.PARENT.read_bytes(), ORIGINAL_PARENT)
        self.assertFalse(module.TARGET.exists())
        self.assertNotIn(module.HOST, json.dumps(self.fake.config))
        evidence = (work / 'withdrawal-receipt.json').read_bytes()
        self.assertEqual(result['receiptSHA256'], module.digest(evidence))
        self.assertEqual(json.loads(evidence)['routeBeforeSHA256'], module.digest(ROUTE))
        self.assertEqual(module.withdrawn_endpoints(), {module.INSTANCE: 35000})
        replay = module.withdraw(work, receipt)
        self.assertEqual((replay['replayed'], replay['receiptSHA256'], self.fake.loads), (True, result['receiptSHA256'], 1))
        # Neither a newer export of the same identity nor its reserved forward can be republished.
        for endpoint in [{'deploymentId': module.INSTANCE, 'forwardPort': 35000}, {'deploymentId': OTHER, 'forwardPort': 35000}]:
            with self.assertRaises(ValueError):
                module.validate_history_generation({'generation': 99, 'endpoints': [endpoint]})
        module.validate_history_generation({'generation': 99, 'endpoints': [{'deploymentId': OTHER, 'forwardPort': 35001}]})
        with self.assertRaises(ValueError):
            module.prepare_withdrawal(self.root / 'unused', OTHER, APPLIED)

    def test_rollback_requires_current_native_upstream(self):
        work, receipt = self.prepare()
        module.withdraw(work, receipt)
        with patch.object(module, 'verify_upstreams', side_effect=ValueError('native removed')):
            with self.assertRaises(ValueError):
                module.rollback(work, receipt)
        self.assertFalse(module.TARGET.exists())
        module.rollback(work, receipt)
        self.assertEqual((module.TARGET.read_bytes(), module.PARENT.read_bytes()), (ROUTE, module.parent_after(ORIGINAL_PARENT)))
        self.assertEqual(module.withdrawn_endpoints(), {})
        with self.assertRaises(ValueError):
            module.withdraw(work, receipt)

    def test_failed_reload_restores_route(self):
        work, receipt = self.prepare()
        self.fake.fail_load = True
        with self.assertRaises(RuntimeError):
            module.withdraw(work, receipt)
        self.assertEqual((module.TARGET.read_bytes(), module.PARENT.read_bytes()), (ROUTE, module.parent_after(ORIGINAL_PARENT)))
        self.assertIn(module.HOST, json.dumps(self.fake.config))
        self.assertTrue((work / 'rolled-back.json').exists())
        self.assertFalse((work / 'withdrawal-receipt.json').exists())

    def test_failure_rollback_restores_route_with_degraded_upstream(self):
        work, receipt = self.prepare()
        self.fake.fail_load = True
        with patch.object(module, 'verify_upstreams', side_effect=ValueError('upstream degraded')):
            with self.assertRaises(RuntimeError):
                module.withdraw(work, receipt)
        self.assertEqual((module.TARGET.read_bytes(), module.PARENT.read_bytes()), (ROUTE, module.parent_after(ORIGINAL_PARENT)))
        self.assertEqual(module.withdrawn_endpoints(), {})

    def test_interrupted_withdrawal_is_tombstoned_and_resumable(self):
        work, receipt = self.prepare()
        original = module.replace
        def interrupt(path, data):
            original(path, data)
            raise KeyboardInterrupt
        with patch.object(module, 'replace', side_effect=interrupt):
            with self.assertRaises(KeyboardInterrupt):
                module.withdraw(work, receipt)
        self.assertEqual(module.withdrawn_endpoints(), {module.INSTANCE: 35000})
        with self.assertRaises(ValueError):
            module.validate_history_generation({'generation': 99, 'endpoints': [{'deploymentId': module.INSTANCE, 'forwardPort': 35000}]})
        result = module.withdraw(work, receipt)
        self.assertNotIn('replayed', result)
        self.assertFalse(module.TARGET.exists())

    def test_second_prepared_withdrawal_cannot_apply(self):
        first, first_receipt = self.prepare()
        second = self.root / OTHER
        second.mkdir(mode=0o700)
        module.prepare_withdrawal(second, OTHER, APPLIED)
        module.withdraw(first, first_receipt)
        with self.assertRaises(ValueError):
            module.withdraw(second, json.loads((second / 'prepared.json').read_bytes()))
        self.assertFalse((second / 'withdrawal-receipt.json').exists())

    def test_unrelated_or_owned_drift_refuses_before_mutation(self):
        work, receipt = self.prepare()
        (self.base / 'certs/other/site.crt').write_bytes(b'changed trust')
        with self.assertRaises(ValueError):
            module.withdraw(work, receipt)
        self.assertEqual(module.TARGET.read_bytes(), ROUTE)
        (self.base / 'certs/other/site.crt').write_bytes(b'unrelated trust')
        module.TARGET.write_bytes(ROUTE + b'# operator edit\n')
        with self.assertRaises(ValueError):
            module.withdraw(work, receipt)
        self.assertEqual(self.fake.loads, 0)

    def test_preparation_refuses_foreign_scope_and_changed_install(self):
        module.TARGET.write_bytes(b'unknown route')
        with self.assertRaises(ValueError):
            module.prepare_withdrawal(self.root, WITHDRAWAL, APPLIED)
        module.TARGET.write_bytes(ROUTE)
        multiple = 'dddddddd-dddd-4ddd-addd-dddddddddddd'
        self.history(multiple, INITIAL, 10, module.parent_after(ORIGINAL_PARENT), ROUTE, module.parent_after(ORIGINAL_PARENT), endpoints=[
            {'deploymentId': module.INSTANCE, 'hostname': module.HOST, 'forwardPort': 35000, 'certificateSha256': 'a' * 64},
            {'deploymentId': OTHER, 'hostname': 'pds-' + OTHER.replace('-', '') + '.subcult.tv', 'forwardPort': 35001, 'certificateSha256': 'b' * 64}])
        with self.assertRaises(ValueError):
            module.prepare_withdrawal(self.root, WITHDRAWAL, multiple)
        (self.root / INITIAL / 'parent-before').write_bytes(b'tampered')
        with self.assertRaises(ValueError):
            module.prepare_withdrawal(self.root, WITHDRAWAL, APPLIED)
        self.assertEqual(self.fake.loads, 0)


if __name__ == '__main__':
    unittest.main()
