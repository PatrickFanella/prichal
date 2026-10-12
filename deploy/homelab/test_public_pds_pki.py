import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('public_pds_pki', Path(__file__).with_name('initialize-public-pds-pki.py'))
pki = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pki)


class PublicPDSTrust(unittest.TestCase):
    def test_retained_key_refused_before_transport(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'pki'
            root.mkdir(mode=0o700)
            key = root / 'ca.key'
            key.write_bytes(b'retained-key')
            with patch.object(pki, 'ROOT', root), patch.object(pki.subprocess, 'check_output') as transport:
                with self.assertRaisesRegex(RuntimeError, 'Retain existing PKI'):
                    pki.main()
                transport.assert_not_called()
            self.assertEqual(key.read_bytes(), b'retained-key')

    def test_fresh_public_csr_profiles_verify(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            root = parent / 'pki'
            root.mkdir(mode=0o700)
            node_key = parent / 'guest-only.key'
            subprocess.run(['openssl', 'genpkey', '-algorithm', 'ED25519', '-out', str(node_key)], check=True, capture_output=True)
            csr = subprocess.check_output(['openssl', 'req', '-new', '-key', str(node_key), '-subj', '/CN=pds-ass-agent'])
            with patch.object(pki, 'ROOT', root), patch.object(pki.subprocess, 'check_output', return_value=csr):
                pki.main()
            self.assertFalse((root / 'agent.key').exists())
            for role, purpose in [('agent', 'sslserver'), ('worker', 'sslclient')]:
                subprocess.run(['openssl', 'verify', '-x509_strict', '-CAfile', str(root / 'ca.crt'),
                                '-purpose', purpose, str(root / (role+'.crt'))], check=True, capture_output=True)
            self.assertTrue(all(path.stat().st_mode & 0o077 == 0 for path in root.iterdir()))


if __name__ == '__main__':
    unittest.main()
