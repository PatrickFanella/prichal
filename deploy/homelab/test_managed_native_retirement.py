import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('retire', Path(__file__).with_name('retire-staging-02-managed.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
REQUEST = 'dddddddd-dddd-4ddd-addd-dddddddddddd'
WITHDRAWAL = 'e' * 64


class FakeDocker:
    def __init__(self, accounts=b'0\n'):
        self.accounts = accounts
        self.containers = {}
        for index, (prefix, identifier) in enumerate(zip(module.NAMES, module.IDS)):
            mounts = [{'Destination': '/pds', 'Type': 'volume', 'Name': module.VOLUMES[0]},
                      {'Destination': '/blobs', 'Type': 'volume', 'Name': module.VOLUMES[1]}] if index == 0 else []
            self.containers[prefix + module.INSTANCE] = {
                'Id': identifier, 'State': {'Running': index < 2}, 'Mounts': mounts,
                'Config': {'Labels': {module.LABEL: module.INSTANCE}, 'Env': ['PDS_HOSTNAME=' + module.HOST, 'PDS_INVITE_REQUIRED=true']},
                'HostConfig': {'RestartPolicy': {'Name': 'no'}}}
        self.volumes = {name: '/var/lib/docker/volumes/' + name + '/_data' for name in module.VOLUMES}
        self.removed = []
        self.fail_rm = None

    def by_id(self, identifier):
        return next(name for name, value in self.containers.items() if value['Id'] == identifier)

    def run(self, arguments):
        command = arguments[1]
        if command == 'ps':
            return '\n'.join(value['Id'] for value in self.containers.values()).encode()
        if command == 'inspect':
            if any(name not in self.containers for name in arguments[2:]):
                raise RuntimeError('missing')
            return json.dumps([self.containers[name] for name in arguments[2:]]).encode()
        if command == 'volume':
            return json.dumps([{'Name': name, 'Mountpoint': self.volumes[name]} for name in arguments[3:]]).encode()
        if command == 'exec':
            return self.accounts
        if command == 'stop':
            self.containers[self.by_id(arguments[-1])]['State']['Running'] = False
            return b''
        if command == 'rm':
            if '-v' in arguments or '-f' in arguments:
                raise AssertionError('volume or forced removal requested')
            if self.fail_rm == arguments[-1]:
                self.fail_rm = None
                raise RuntimeError('injected removal failure')
            name = self.by_id(arguments[-1])
            if self.containers[name]['State']['Running']:
                raise RuntimeError('running container')
            del self.containers[name]
            self.removed.append(arguments[-1])
            return b''
        raise AssertionError(arguments)


class NativeRetirementTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        agent = Path(self.temporary.name)
        (agent / 'state').mkdir(mode=0o700)
        (agent / 'enrolled').mkdir()
        root = agent / 'managed-retirements'
        preservation = root / 'c3b6c1be-8493-43f4-9a79-ad6f3cbab521'
        preservation.mkdir(parents=True, mode=0o700)
        archive = b'preserved archive'
        (preservation / 'native-cold.tar.gz').write_bytes(archive)
        retained = [agent / 'state' / (module.INSTANCE + suffix) for suffix in ('.json', '-tls.pem', '-managed-identity.json')] + [agent / 'enrolled/node.key', agent / 'enrolled/node.crt']
        for path in retained:
            path.write_bytes(path.name.encode())
            path.chmod(0o600)
        report = {'deploymentId': module.INSTANCE, 'archiveSHA256': hashlib.sha256(archive).hexdigest(), 'accounts': 0, 'retired': False}
        (preservation / 'report.json').write_text(json.dumps(report))
        (preservation / 'report.json').chmod(0o600)
        self.docker = FakeDocker()
        self.patches = [patch.object(module, 'AGENT', agent), patch.object(module, 'STATE', agent / 'state'), patch.object(module, 'ROOT', root),
                        patch.object(module, 'PRESERVATION', preservation), patch.object(module, 'ARCHIVE_SHA256', report['archiveSHA256']),
                        patch.object(module, 'RETAINED', retained), patch.object(module, 'run', side_effect=self.docker.run),
                        # Retain type/permission/size checks; temporary files cannot be root-owned.
                        patch.object(module, 'private_json', side_effect=self.private_json)]
        for item in self.patches:
            item.start()
        self.root, self.state, self.retained = root, agent / 'state', retained

    def private_json(self, path, limit=1 << 20):
        if path.lstat().st_mode & 0o077:
            raise ValueError('unsafe custody')
        return json.loads(path.read_bytes())

    def tearDown(self):
        for item in reversed(self.patches):
            item.stop()
        self.temporary.cleanup()

    def retire(self):
        return module.retire(self.root / REQUEST, REQUEST, WITHDRAWAL)

    def test_hold_then_remove_only_fixture_containers(self):
        result = self.retire()
        self.assertEqual((result['retired'], result['capacityReleased']), (True, False))
        self.assertEqual((self.docker.containers, self.docker.removed), ({}, module.IDS))
        self.assertEqual(set(self.docker.volumes), set(module.VOLUMES))
        lease = json.loads((self.state / (module.INSTANCE + '-service.json')).read_bytes())
        self.assertEqual((lease['state'], lease['phase'], lease['generation'], lease['ticket']), ('suspended', 'applied', 1 << 53, REQUEST))
        data = (self.root / REQUEST / 'native-retirement.json').read_bytes()
        self.assertEqual(result['receiptSHA256'], hashlib.sha256(data).hexdigest())
        receipt = json.loads(data)
        self.assertEqual((receipt['edgeWithdrawalSHA256'], receipt['removedContainerIds']), (WITHDRAWAL, module.IDS))
        replay = self.retire()
        self.assertEqual((replay['replayed'], replay['receiptSHA256']), (True, result['receiptSHA256']))
        with self.assertRaises(ValueError):
            module.retire(self.root / REQUEST, REQUEST, 'f' * 64)
        self.retained[0].write_bytes(b'changed credentials')
        with self.assertRaises(ValueError):
            self.retire()

    def test_interrupted_removal_resumes_under_same_request(self):
        self.docker.fail_rm = module.IDS[2]
        with self.assertRaises(RuntimeError):
            self.retire()
        self.assertTrue((self.state / (module.INSTANCE + '-service.json')).exists())
        self.assertFalse((self.root / REQUEST / 'native-retirement.json').exists())
        result = self.retire()
        self.assertEqual(self.docker.containers, {})
        self.assertNotIn('replayed', result)
        with self.assertRaises(ValueError):
            module.retire(self.root / 'aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa', 'aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa', WITHDRAWAL)

    def test_partial_custody_from_a_crash_is_replaced(self):
        work = self.root / REQUEST
        work.mkdir(mode=0o700)
        (work / '.before.json.partial').write_bytes(b'{"trunc')
        (self.state / ('.retirement-hold-' + REQUEST)).write_bytes(b'{"trunc')
        self.assertTrue(self.retire()['retired'])
        self.assertFalse((work / '.before.json.partial').exists())
        self.assertFalse((self.state / ('.retirement-hold-' + REQUEST)).exists())

    def test_refusals_before_any_mutation(self):
        cases = [
            lambda: setattr(self.docker, 'accounts', b'1\n'),
            lambda: self.docker.containers['pds-ass-private-predecessor-' + module.INSTANCE]['State'].update(Running=True),
            lambda: self.docker.containers['pds-ass-edge-' + module.INSTANCE]['State'].update(Running=False),
            lambda: self.docker.containers['pds-ass-tenant-' + module.INSTANCE].update(Id='0' * 64),
            lambda: self.docker.containers.update(foreign={'Id': 'f' * 64}),
            lambda: (module.PRESERVATION / 'native-cold.tar.gz').write_bytes(b'changed archive'),
            lambda: (self.state / (module.INSTANCE + '-service.json')).write_bytes(b'{"foreign":true}'),
        ]
        for index, change in enumerate(cases):
            with self.subTest(index=index):
                self.tearDown()
                self.setUp()
                change()
                before = json.dumps(self.docker.containers, sort_keys=True)
                with self.assertRaises((ValueError, RuntimeError)):
                    self.retire()
                self.assertEqual((json.dumps(self.docker.containers, sort_keys=True), self.docker.removed), (before, []))
                self.assertFalse((self.root / REQUEST / 'native-retirement.json').exists())


if __name__ == '__main__':
    unittest.main()
