import datetime as dt
import importlib.util
import math
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


publisher = load('staging02_publisher', 'staging-02-metrics-publish.py')
receiver = load('staging02_receiver', 'staging-02-metrics-receiver.py')
NOW = 1793500000


def iso(seconds):
    return dt.datetime.fromtimestamp(seconds, dt.timezone.utc).isoformat()


class Staging02Metrics(unittest.TestCase):
    def receipt(self, now=NOW):
        # The shape renew-staging-02-node.py writes after a successful run.
        return {'success': True, 'checkedAt': iso(now - 60), 'nodeId': publisher.NODE,
                'instanceIds': ['e4067ea1-f000-4321-852f-0c67a179c0ed'], 'renewed': {'node': False, 'worker': False},
                'nodeExpiry': iso(now + 20 * 3600), 'workerExpiry': iso(now + 4 * 86400),
                'caExpiry': iso(now + 90 * 86400), 'retainedKey': True, 'maximumSlots': 1, 'cpus': 3}

    def test_snapshot_carries_only_dates_and_operator_state(self):
        value = publisher.snapshot(self.receipt(), {}, NOW)
        receiver.validate(value, NOW)
        self.assertNotIn('instanceIds', value)
        self.assertEqual(value['lastSuccess'], NOW - 60)
        self.assertEqual(value['expiry'], {'node': NOW + 20 * 3600, 'worker': NOW + 4 * 86400, 'ca': NOW + 90 * 86400})
        self.assertTrue(value['inspectionOK'])

    def test_failed_renewal_preserves_success_time_and_known_expiry(self):
        previous = publisher.snapshot(self.receipt(), {}, NOW)
        failed = publisher.snapshot({'success': False, 'checkedAt': iso(NOW + 3600), 'nodeId': publisher.NODE,
                                     'errorType': 'ValueError'}, previous, NOW + 3610)
        self.assertEqual(failed['lastSuccess'], previous['lastSuccess'])
        self.assertEqual(failed['lastAttempt'], NOW + 3600)
        self.assertEqual(failed['expiry'], previous['expiry'])
        self.assertFalse(failed['success'])
        self.assertTrue(failed['inspectionOK'])
        receiver.validate(failed, NOW + 3610)

    def test_failure_without_known_dates_is_an_inspection_failure(self):
        failed = publisher.snapshot({'success': False, 'checkedAt': iso(NOW - 10), 'nodeId': publisher.NODE}, {}, NOW)
        self.assertFalse(failed['inspectionOK'])
        receiver.validate(failed, NOW)

    def test_replayed_or_foreign_receipts_are_refused(self):
        previous = publisher.snapshot(self.receipt(), {}, NOW)
        for receipt in [self.receipt(now=NOW - 3600), dict(self.receipt(), nodeId='foreign'),
                        dict(self.receipt(), checkedAt='2026-10-31T16:08:33'), dict(self.receipt(), success='true'),
                        dict(self.receipt(), checkedAt=iso(NOW + 3600)), dict(self.receipt(), caExpiry=iso(NOW + 400 * 86400))]:
            with self.subTest(receipt=receipt), self.assertRaises(ValueError):
                publisher.snapshot(receipt, previous, NOW)

    def test_receiver_rejects_unknown_identity_roles_fields_and_nonfinite_data(self):
        valid = publisher.snapshot(self.receipt(), {}, NOW)
        for update in [{'nodeId': 'foreign'}, {'expiry': {'agent': NOW}}, {'lastSuccess': math.inf},
                       {'success': 'false'}, {'path': '/tmp/override'}, {'observedAt': 1}, {'observedAt': NOW + 20000}]:
            with self.subTest(update=update), self.assertRaises(ValueError):
                receiver.validate(dict(valid, **update), NOW)

    def test_scalar_metric_injection_is_refused(self):
        value = publisher.snapshot(self.receipt(), {}, NOW)
        value['expiry']['node'] = '1\nforged_metric 1'
        with self.assertRaises(ValueError):
            receiver.render(value, NOW)

    def test_atomic_publication_preserves_unrelated_metrics(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            other = root / 'other.prom'
            other.write_text('unrelated 1\n')
            target = root / 'certificates.prom'
            value = publisher.snapshot(self.receipt(), {}, NOW)
            with patch.object(receiver, 'ROOT', root), patch.object(receiver, 'TARGET', target):
                receiver.publish(receiver.render(value, NOW))
            self.assertEqual(other.read_text(), 'unrelated 1\n')
            self.assertEqual(target.stat().st_mode & 0o777, 0o644)
            lines = target.read_text().splitlines()
            self.assertEqual(len(lines), 9)
            self.assertIn(f'prichal_staging_02_certificate_not_after_seconds{{role="node"}} {float(NOW + 20 * 3600)}', lines)

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
