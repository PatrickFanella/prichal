import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from cryptography.exceptions import InvalidTag

spec = importlib.util.spec_from_file_location('recovery_crypto', Path(__file__).with_name('recovery-crypto.py'))
crypto = importlib.util.module_from_spec(spec)
spec.loader.exec_module(crypto)


class RecoveryTests(unittest.TestCase):
    def test_roundtrip_and_exclusive_publication(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            key = root / 'key'
            key.write_bytes(os.urandom(32))
            key.chmod(0o600)
            source = root / 'source'
            source.write_bytes(os.urandom(2048))
            encrypted, restored = root / 'encrypted', root / 'restored'
            digest = crypto.transform('encrypt', source, encrypted, key)
            crypto.transform('decrypt', encrypted, restored, key, digest)
            self.assertEqual(source.read_bytes(), restored.read_bytes())
            with self.assertRaises(ValueError):
                crypto.transform('decrypt', encrypted, restored, key, digest)

    def test_corruption_never_publishes_plaintext(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            key = root / 'key'
            key.write_bytes(os.urandom(32))
            key.chmod(0o600)
            source = root / 'source'
            source.write_bytes(b'private fixture' * 100)
            encrypted, restored = root / 'encrypted', root / 'restored'
            digest = crypto.transform('encrypt', source, encrypted, key)
            data = bytearray(encrypted.read_bytes())
            data[-20] ^= 1
            encrypted.write_bytes(data)
            with self.assertRaises(InvalidTag):
                crypto.transform('decrypt', encrypted, restored, key, digest)
            self.assertFalse(restored.exists())
            self.assertEqual(list(root.glob('.pending-*')), [])

    def test_public_key_file_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            key = Path(directory) / 'key'
            key.write_bytes(os.urandom(32))
            key.chmod(0o644)
            with self.assertRaises(ValueError):
                crypto.private_key(key)

    def test_wrong_scope_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            key = root / 'key'
            key.write_bytes(os.urandom(32))
            key.chmod(0o600)
            source = root / 'source'
            source.write_bytes(b'other archive type' * 100)
            with self.assertRaises(ValueError):
                crypto.transform('decrypt', source, root / 'restored', key, '0' * 64)
            self.assertFalse((root / 'restored').exists())


if __name__ == '__main__':
    unittest.main()
