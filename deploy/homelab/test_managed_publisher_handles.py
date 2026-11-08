import copy
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('publisher_handles', Path(__file__).with_name('publish-managed-staging.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

DEPLOYMENT = 'e4067ea1-f000-4321-852f-0c67a179c0ed'
INVITATION = '49aac16a-2663-4eb4-a95d-4c5c3029506e'
HANDLE = {'invitationId': INVITATION, 'deploymentId': DEPLOYMENT, 'hostname': 'at-49aac16a26634eb.subcult.tv', 'did': 'did:plc:eqtyxf4i56sfx4plgd2necjd'}


def inventory(handles):
    return {'version': 1, 'generation': 13, 'expiresAt': '2026-11-08T03:00:00+00:00',
            'endpoints': [{'deploymentId': DEPLOYMENT, 'hostname': 'pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv', 'forwardPort': 35001, 'certificateSha256': 'a' * 64}],
            'handles': handles}


class PublisherHandleTests(unittest.TestCase):
    def test_verified_handle_is_accepted(self):
        value = module.inventory_value(json.dumps(inventory([HANDLE])).encode())
        self.assertEqual(value['handles'], [HANDLE])
        self.assertEqual(module.handle_hostname(INVITATION), 'at-49aac16a26634eb.subcult.tv')

    def test_malformed_or_unplaced_handles_are_refused(self):
        other = dict(HANDLE, invitationId='bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb', hostname='at-bbbbbbbbbbbb4bb.subcult.tv')
        for name, handles in {
            'long legacy form': [dict(HANDLE, hostname='at-49aac16a26634eb4a95d4c5c3029506e.subcult.tv')],
            'arbitrary host': [dict(HANDLE, hostname='admin.subcult.tv')],
            'unknown deployment': [dict(HANDLE, deploymentId='bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb')],
            'web DID': [dict(HANDLE, did='did:web:evil.example')],
            'DID injection': [dict(HANDLE, did='did:plc:eqtyxf4i56sfx4plgd2necjd"\n}')],
            'extra field': [dict(HANDLE, upstream='http://evil')],
            'duplicate host': [HANDLE, dict(HANDLE)],
            'duplicate DID': [HANDLE, dict(other, did=HANDLE['did'])],
            'not a list': {'0': HANDLE},
        }.items():
            with self.subTest(name=name):
                with self.assertRaises(ValueError):
                    module.inventory_value(json.dumps(inventory(handles)).encode())

    def test_updates_may_add_but_not_drop_or_remap_handles(self):
        before = inventory([HANDLE])
        before['generation'] = 12
        module.validate_transition(inventory([]) | {'generation': 11}, before)
        for name, handles in {
            'omitted': [],
            'remapped DID': [dict(HANDLE, did='did:plc:aaaaaaaaaaaaaaaaaaaaaaaa')],
        }.items():
            with self.subTest(name=name):
                with self.assertRaises(ValueError):
                    module.validate_transition(before, inventory(handles))
        module.validate_transition(before, inventory([copy.deepcopy(HANDLE)]))


if __name__ == '__main__':
    unittest.main()
