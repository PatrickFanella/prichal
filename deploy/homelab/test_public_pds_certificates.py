import hashlib
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    module = importlib.util.module_from_spec(spec)
    with patch('pwd.getpwnam', return_value=SimpleNamespace(pw_uid=os.getuid())):
        spec.loader.exec_module(module)
    return module


renewal = load('renewal', 'renew-public-pds-certificates.py')
guest = load('guest', 'public-pds-certificate-guest.py')
trust = load('trust', 'public-pds-certificate-trust.py')
initialize = load('initialize', 'initialize-public-pds-pki.py')
recovery = load('recovery', 'reconcile-public-pds.py')


class PublicCertificateLifecycle(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.old_umask = os.umask(0o077)
        self.addCleanup(os.umask, self.old_umask)

    def command(self, *args):
        return subprocess.check_output(['openssl', *map(str, args)], stderr=subprocess.DEVNULL)

    def fixture(self):
        pki = self.root / 'pki'
        pki.mkdir(mode=0o700)
        key = self.root / 'guest-only.key'
        self.command('genpkey', '-algorithm', 'ED25519', '-out', key)
        csr = self.command('req', '-new', '-key', key, '-subj', '/CN=pds-ass-agent')
        with patch.object(initialize, 'ROOT', pki), patch.object(initialize.subprocess, 'check_output', return_value=csr):
            initialize.main()
        path = self.root / 'run'
        path.mkdir(mode=0o700)
        candidate = {'ca': (pki / 'ca.crt').read_text(), 'agent': (pki / 'agent.crt').read_text(), 'csr': csr.decode()}
        return pki, path, candidate

    def test_real_csr_renewal_preserves_keys_and_strict_profiles(self):
        pki, path, candidate = self.fixture()
        before = {name: (pki / name).read_bytes() for name in ['ca.key', 'worker.key', 'ca.crt', 'agent.crt', 'worker.crt']}
        with patch.object(renewal, 'ROOT', self.root):
            hashes = renewal.stage(path, candidate, recovery)
        self.assertEqual(before, {name: (pki / name).read_bytes() for name in before})
        self.assertFalse((pki / 'agent.key').exists())
        self.assertEqual(hashes['ca.key'], hashlib.sha256(before['ca.key']).hexdigest())
        for role, purpose in [('agent', 'sslserver'), ('worker', 'sslclient')]:
            self.command('verify', '-x509_strict', '-CAfile', path / 'ca.crt', '-purpose', purpose, path / (role + '.crt'))

    def test_foreign_csr_is_refused_before_signing(self):
        _, path, candidate = self.fixture()
        foreign = self.root / 'foreign.key'
        self.command('genpkey', '-algorithm', 'ED25519', '-out', foreign)
        candidate['csr'] = self.command('req', '-new', '-key', foreign, '-subj', '/CN=pds-ass-agent').decode()
        with patch.object(renewal, 'ROOT', self.root), self.assertRaisesRegex(RuntimeError, 'changed retained agent key'):
            renewal.stage(path, candidate, recovery)
        self.assertFalse((path / 'ca.crt').exists())

    def test_guest_public_trust_must_match_host_before_signing(self):
        _, path, candidate = self.fixture()
        candidate['ca'] = 'foreign'
        with patch.object(renewal, 'ROOT', self.root), self.assertRaisesRegex(RuntimeError, 'differ between host and guest'):
            renewal.stage(path, candidate, recovery)
        self.assertFalse((path / 'ca.crt').exists())

    def test_due_threshold_covers_ca_and_each_leaf(self):
        now = time.time()
        fresh = {'ca.crt': now + 90 * 86400, 'agent.crt': now + 7 * 86400,
                 'worker.crt': now + 7 * 86400, 'ingress.pem': now + 7 * 86400}
        self.assertFalse(renewal.due(fresh, now))
        self.assertTrue(renewal.due(fresh, now, True))
        for role in fresh:
            with self.subTest(role=role):
                dates = dict(fresh)
                dates[role] = now + (30 if role == 'ca.crt' else 3) * 86400
                self.assertTrue(renewal.due(dates, now))

    def test_public_trust_rejects_private_material_and_wrong_key(self):
        with self.assertRaises(RuntimeError):
            trust.certificates('-----BEGIN PRIVATE KEY-----')
        key = self.root / 'ingress.key'
        cert = self.root / 'ingress.crt'
        self.command('genpkey', '-algorithm', 'ED25519', '-out', key)
        self.command('req', '-x509', '-new', '-key', key, '-days', '7', '-subj',
                     '/CN=' + trust.INSTANCE + '.localhost', '-out', cert)
        with self.assertRaisesRegex(RuntimeError, 'public key differs'):
            trust.validate(cert.read_text())

    def test_guest_activation_failure_restores_all_retained_files(self):
        path = self.root / 'run'
        path.mkdir(mode=0o700)
        before = path / 'before'
        before.mkdir(mode=0o700)
        sources = {}
        for name in ['ca.crt', 'agent.crt', 'ingress.pem']:
            sources[name] = self.root / name
            sources[name].write_bytes(b'old-' + name.encode())
            (before / name).write_bytes(sources[name].read_bytes())
        (path / 'ingress.pem').write_bytes(b'new-ingress')
        report = {'runtime': {'retained': True}, 'sourceHashes': {name: hashlib.sha256(source.read_bytes()).hexdigest() for name, source in sources.items()}}
        (path / 'staging.json').write_text(__import__('json').dumps(report))
        with patch.object(guest, 'target', return_value=path), patch.object(guest, 'material', return_value=sources), patch.object(guest, 'runtime', return_value=report['runtime']), patch.object(guest, 'private', side_effect=lambda p, owner=None: p.read_bytes()), patch.object(guest, 'validate_candidates'), patch.object(guest, 'restart') as restart:
            with self.assertRaises(RuntimeError):
                guest.activate({'run': 'fixed', 'ca': 'new-ca', 'agent': 'new-agent'}, fail=True)
            self.assertEqual(restart.call_count, 2)
        self.assertTrue((path / 'rollback-verified.json').exists())
        for name, source in sources.items():
            self.assertEqual(source.read_bytes(), (before / name).read_bytes())


if __name__ == '__main__':
    unittest.main()
