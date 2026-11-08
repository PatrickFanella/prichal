import copy
import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('dns_check', Path(__file__).with_name('check-managed-dns.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

NOW = dt.datetime(2026, 11, 7, tzinfo=dt.timezone.utc)
HOST = 'pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv'
HANDLE = 'at-49aac16a26634eb.subcult.tv'


def state():
    zone = {'name': 'subcult.tv', 'status': 'active', 'type': 'full'}
    records = [{'type': 'CNAME', 'name': '*.subcult.tv', 'proxied': True, 'content': '0123-tunnel.cfargotunnel.com'},
               {'type': 'CNAME', 'name': 'subcult.tv', 'proxied': True, 'content': '0123-tunnel.cfargotunnel.com'},
               {'type': 'A', 'name': 'prichal.subcult.tv', 'proxied': True, 'content': '192.0.2.1'}]
    universal = {'enabled': True}
    packs = [{'type': 'universal', 'status': 'active', 'hosts': ['*.subcult.tv', 'subcult.tv'],
              'certificates': [{'expires_on': '2026-12-15T23:16:24.000000Z'}]},
             {'type': 'universal', 'status': 'backup_issued', 'hosts': ['*.subcult.tv', 'subcult.tv'],
              'certificates': [{'expires_on': '2027-01-07T11:58:41.000000Z'}]}]
    return zone, records, universal, packs


class ManagedDNSCheck(unittest.TestCase):
    def check(self, mutate=None, names=(HOST, HANDLE), now=NOW):
        values = copy.deepcopy(state())
        if mutate:
            mutate(*values)
        return module.evaluate(*values, list(names), now)

    def test_current_zone_serves_generated_names(self):
        problems, evidence = self.check()
        self.assertEqual(problems, [])
        self.assertEqual(evidence['certificateExpiresAt'], '2026-12-15T23:16:24+00:00')
        self.assertEqual(len(evidence['wildcardTargetSha256']), 64)
        self.assertNotIn('cfargotunnel', json.dumps(evidence))

    def test_shadowing_records_are_refused(self):
        for name in [HOST, '_atproto.' + HANDLE, 'www.' + HOST]:
            with self.subTest(name=name):
                problems, _ = self.check(lambda z, r, u, p: r.append({'type': 'TXT', 'name': name, 'content': 'x'}))
                self.assertEqual(len(problems), 1)
                self.assertIn('shadowed', problems[0])

    def test_wildcard_route_changes_are_refused(self):
        def wildcard(r):
            return next(x for x in r if x['name'] == '*.subcult.tv')
        for name, mutate in {
            'unproxied': lambda z, r, u, p: wildcard(r).update(proxied=False),
            'not a tunnel': lambda z, r, u, p: wildcard(r).update(content='elsewhere.example.net'),
            'A record': lambda z, r, u, p: wildcard(r).update(type='A', content='192.0.2.9'),
            'missing': lambda z, r, u, p: r.remove(wildcard(r)),
            'duplicate': lambda z, r, u, p: r.append(dict(wildcard(r))),
        }.items():
            with self.subTest(name=name):
                problems, _ = self.check(mutate)
                self.assertTrue(any('wildcard' in p for p in problems), problems)

    def test_certificate_and_zone_failures_are_refused(self):
        for name, mutate, now in [
            ('universal off', lambda z, r, u, p: u.update(enabled=False), NOW),
            ('no wildcard host', lambda z, r, u, p: p[0].update(hosts=['subcult.tv']), NOW),
            ('pack inactive', lambda z, r, u, p: p[0].update(status='pending_validation'), NOW),
            ('near expiry', None, dt.datetime(2026, 12, 5, tzinfo=dt.timezone.utc)),
            ('zone paused', lambda z, r, u, p: z.update(status='pending'), NOW),
            ('partial setup', lambda z, r, u, p: z.update(type='partial'), NOW),
        ]:
            with self.subTest(name=name):
                problems, _ = self.check(mutate, now=now)
                self.assertTrue(problems)

    def test_only_generated_names_are_accepted(self):
        for name in ['prichal.subcult.tv', 'at-aaaaaaaaaaaa4aaa8aaaaaaaaaaaaaaa.subcult.tv', 'at-49aac16a26634e.subcult.tv', 'pds-e4067ea1.subcult.tv', 'pds-e4067ea1f0004321852f0c67a179c0ed.example.net', 'PDS-E4067EA1F0004321852F0C67A179C0ED.subcult.tv']:
            with self.subTest(name=name):
                problems, _ = self.check(names=[name])
                self.assertTrue(any('not a generated' in p for p in problems))
        problems, _ = self.check(names=[])
        self.assertIn('no hostnames to check', problems)

    def test_inventory_endpoints_and_handles_are_checked(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'inventory.json'
            path.write_text(json.dumps({'version': 1, 'generation': 12, 'endpoints': [{'hostname': HOST}], 'handles': [{'hostname': HANDLE}]}))
            self.assertEqual(module.hostnames_from_inventory(path), [HOST, HANDLE])
            path.write_text(json.dumps({'version': 2, 'endpoints': []}))
            with self.assertRaises(ValueError):
                module.hostnames_from_inventory(path)


if __name__ == '__main__':
    unittest.main()
