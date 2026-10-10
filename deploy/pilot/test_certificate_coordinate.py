"""Exercise lost-response and public-failure boundaries without host credentials."""
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import time
import unittest
import urllib.error
from unittest.mock import patch, MagicMock

spec = importlib.util.spec_from_file_location('coordinate', Path(__file__).with_name('certificate-coordinate.py'))
coordinate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(coordinate)


class FailureBoundaries(unittest.TestCase):
    def scenario(self, *, activation_error=False, rollback_verified=False, public_error=False, force=True):
        calls = []
        expiry = {role: time.time() + 7 * 86400 for role in ['ca', 'agent', 'worker', 'ingress']}

        def dozor(action, *args):
            calls.append(('host', action))
            if action == 'check':
                return {'healthy': True, 'expiry': expiry}
            if action == 'prepare':
                return {'run': args[0], 'old': 'public old', 'new': 'public new'}
            if action == 'activate' and activation_error:
                raise RuntimeError('Transport response lost')
            if action == 'rollback-state':
                return {'rollbackVerified': rollback_verified}
            return {}

        def trust(body):
            calls.append(('trust', body['action']))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(coordinate, 'ROOT', root), patch.object(coordinate, 'dozor', dozor), patch.object(coordinate, 'trust', trust), patch.object(coordinate, 'public_check', side_effect=RuntimeError('Public transport failed') if public_error else None), patch.object(coordinate.sys, 'argv', ['operator', '--qualify-renewal'] if force else ['operator']):
                if activation_error or public_error:
                    with self.assertRaises(RuntimeError):
                        coordinate.main()
                else:
                    coordinate.main()
            receipt = json.loads(next(root.glob('*.json')).read_text())
        return calls, receipt

    def test_lost_activation_response_keeps_overlapping_trust_without_verified_rollback(self):
        calls, receipt = self.scenario(activation_error=True)
        self.assertNotIn(('trust', 'rollback'), calls)
        self.assertTrue(receipt['manualInspectionRequired'])
        self.assertIn(('host', 'record'), calls)

    def test_verified_host_rollback_allows_trust_rollback(self):
        calls, receipt = self.scenario(activation_error=True, rollback_verified=True)
        self.assertIn(('trust', 'rollback'), calls)
        self.assertTrue(receipt['rollbackVerified'])
        self.assertFalse(receipt['success'])

    def test_public_failure_after_activation_keeps_new_trust(self):
        calls, receipt = self.scenario(public_error=True)
        self.assertNotIn(('trust', 'rollback'), calls)
        self.assertTrue(receipt['renewed'])
        self.assertFalse(receipt['success'])

    def test_young_certificates_only_inspect_without_mutation(self):
        calls, receipt = self.scenario(force=False)
        self.assertFalse(receipt['renewed'])
        self.assertTrue(receipt['success'])
        self.assertNotIn(('host', 'prepare'), calls)
        self.assertNotIn(('trust', 'prepare'), calls)

    def test_cloudflare_challenge_is_not_signup_closure_evidence(self):
        health = MagicMock()
        health.__enter__.return_value.status = 200
        health.__enter__.return_value.read.return_value = b'{"version":"0.4.5036"}'
        challenge = urllib.error.HTTPError('https://pilot/signup', 403, 'Forbidden', {'Cf-Mitigated': 'challenge'}, io.BytesIO(b''))
        with patch.object(coordinate.urllib.request, 'urlopen', side_effect=[health, challenge]):
            with self.assertRaises(RuntimeError):
                coordinate.public_check()
        closed = [urllib.error.HTTPError('https://pilot/signup', 403, 'Forbidden', {}, io.BytesIO(b'')) for _ in range(2)]
        with patch.object(coordinate.urllib.request, 'urlopen', side_effect=[health, *closed]):
            coordinate.public_check()


if __name__ == '__main__':
    unittest.main()
