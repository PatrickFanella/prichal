import importlib.util
import math
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


publisher = load('publisher', 'public-pds-metrics-publish.py')
receiver = load('receiver', 'public-pds-metrics-receiver.py')


class PilotMetrics(unittest.TestCase):
    def receipt(self, now=10000):
        return {'checkedAt': now - 60, 'success': True,
                'expiry': {name: now + 7 * 86400 for name in publisher.ROLE_MAP}}

    def test_snapshot_excludes_all_extra_private_receipt_fields(self):
        current = self.receipt()
        current['secret'] = 'must-never-cross-transport'
        value = publisher.snapshot(current, {}, 10000)
        receiver.validate(value, 10000)
        self.assertNotIn('secret', value)
        self.assertEqual(value['lastSuccess'], 9940)
        self.assertTrue(value['inspectionOK'])

    def test_failed_renewal_preserves_success_time_and_known_expiry(self):
        previous = publisher.snapshot(self.receipt(), {}, 10000)
        failed = publisher.snapshot({'checkedAt': 10010, 'success': False}, previous, 10020)
        self.assertEqual(failed['lastSuccess'], previous['lastSuccess'])
        self.assertEqual(failed['expiry'], previous['expiry'])
        self.assertFalse(failed['inspectionOK'])
        self.assertFalse(failed['success'])
        receiver.validate(failed, 10020)

    def test_replayed_operator_receipt_cannot_refresh_success(self):
        previous = publisher.snapshot(self.receipt(), {}, 10000)
        with self.assertRaises(ValueError):
            publisher.snapshot(self.receipt(now=9900), previous, 10020)

    def test_receiver_rejects_unknown_identity_roles_fields_and_nonfinite_data(self):
        valid = publisher.snapshot(self.receipt(), {}, 10000)
        for update in [{'instanceId': 'foreign'}, {'expiry': {'foreign': 10000}},
                       {'lastSuccess': math.inf}, {'success': 'false'}, {'path': '/tmp/override'},
                       {'observedAt': 1}, {'observedAt': 20000}]:
            with self.subTest(update=update), self.assertRaises(ValueError):
                receiver.validate(dict(valid, **update), 10000)

    def test_scalar_metric_injection_is_refused(self):
        value = publisher.snapshot(self.receipt(), {}, 10000)
        value['expiry']['agent'] = '1\nforged_metric 1'
        with self.assertRaises(ValueError):
            receiver.render(value, 10000)

    def test_atomic_publication_preserves_unrelated_metrics(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            other = root / 'other.prom'
            other.write_text('unrelated 1\n')
            target = root / 'certificates.prom'
            value = publisher.snapshot(self.receipt(), {}, 10000)
            with patch.object(receiver, 'ROOT', root), patch.object(receiver, 'TARGET', target):
                receiver.publish(receiver.render(value, 10000))
            self.assertEqual(other.read_text(), 'unrelated 1\n')
            self.assertEqual(target.stat().st_mode & 0o777, 0o644)
            self.assertEqual(len(target.read_text().splitlines()), 10)

    def test_symlink_target_is_never_followed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            unrelated = root / 'other.prom'
            unrelated.write_text('unchanged')
            target = root / 'certificates.prom'
            target.symlink_to(unrelated)
            with patch.object(receiver, 'ROOT', root), patch.object(receiver, 'TARGET', target):
                with self.assertRaises(ValueError):
                    receiver.publish('fake')
            self.assertEqual(unrelated.read_text(), 'unchanged')


if __name__ == '__main__':
    unittest.main()
