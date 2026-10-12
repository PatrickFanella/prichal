import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('recovery', Path(__file__).with_name('reconcile-public-pds.py'))
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class PublicPDSRecovery(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        (self.root / 'pki').mkdir(mode=0o700)
        for name in ['ca.crt', 'worker.crt', 'worker.key']:
            path = self.root / 'pki' / name
            path.write_text('private-fixture')
            path.chmod(0o600)
        self.patch = patch.object(recovery, 'ROOT', self.root)
        self.patch.start()
        self.addCleanup(self.patch.stop)

    def status(self, state='running', healthy=True):
        return {'id': recovery.INSTANCE, 'endpoint': recovery.ENDPOINT,
                'state': state, 'healthy': healthy}

    def test_missing_runtime_never_creates_deployment(self):
        with patch.object(recovery.ssl, 'create_default_context'), patch.object(
                recovery, 'request', side_effect=FileNotFoundError) as request:
            with self.assertRaises(FileNotFoundError):
                recovery.reconcile()
            self.assertEqual([call.args[1] for call in request.call_args_list], ['GET'])

    def test_retained_stopped_runtime_reconciles_then_observes_health(self):
        with patch.object(recovery.ssl, 'create_default_context'), patch.object(
                recovery, 'request', side_effect=[self.status('starting', False), self.status()]) as request:
            result = recovery.reconcile()
            self.assertTrue(result['success'])
            self.assertTrue(result['reconciled'])
            self.assertEqual([call.args[1] for call in request.call_args_list], ['GET', 'PUT'])

    def test_healthy_runtime_is_read_only(self):
        with patch.object(recovery.ssl, 'create_default_context'), patch.object(
                recovery, 'request', return_value=self.status()) as request:
            self.assertTrue(recovery.reconcile()['success'])
            self.assertEqual([call.args[1] for call in request.call_args_list], ['GET'])

    def test_pause_prevents_transport_and_key_reads(self):
        pause = self.root / 'reconciliation-paused'
        pause.touch(mode=0o600)
        with patch.object(recovery.ssl, 'create_default_context') as context:
            self.assertTrue(recovery.reconcile()['paused'])
            context.assert_not_called()

    def test_wrong_identity_and_malformed_health_are_refused(self):
        for update in [{'id': 'foreign'}, {'endpoint': 'https://foreign.example'},
                       {'healthy': 'false'}, {'state': 'unknown'}, {'state': 'suspended'}]:
            with self.subTest(update=update), self.assertRaises(ValueError):
                recovery.validate(dict(self.status(), **update))

    def test_symlink_key_refused_before_transport(self):
        key = self.root / 'pki/worker.key'
        key.unlink()
        key.symlink_to(self.root / 'pki/ca.crt')
        with patch.object(recovery.ssl, 'create_default_context') as context:
            with self.assertRaises(ValueError):
                recovery.reconcile()
            context.assert_not_called()

    def test_redirect_cannot_send_credentials_to_another_origin(self):
        with self.assertRaises(ValueError):
            recovery.NoRedirect().redirect_request(None, None, 302, '', {}, 'https://foreign.example')


if __name__ == '__main__':
    unittest.main()
