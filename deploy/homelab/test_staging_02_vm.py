import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('staging_02_vm', Path(__file__).with_name('staging-02-vm.py'))
vm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(vm)


class StagingTests(unittest.TestCase):
    def test_wrong_mount_refused_before_launch(self):
        with patch.object(vm, 'output', return_value='another-filesystem'):
            with self.assertRaisesRegex(RuntimeError, 'filesystem'):
                vm.check_mount()

    def test_foreign_state_never_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt = root / 'bootstrap.json'
            receipt.write_text('{"name":"someone-elses-vm"}')
            with patch.object(vm, 'ROOT', root), patch.object(vm, 'plan'), patch.object(vm, 'run') as run:
                with self.assertRaisesRegex(RuntimeError, 'differs'):
                    vm.bootstrap(root / 'unused-image')
                run.assert_not_called()
            self.assertEqual(json.loads(receipt.read_text())['name'], 'someone-elses-vm')

    def test_retry_preserves_disk_and_keys(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            root = home / 'vm'
            root.mkdir(mode=0o700)
            source = Path(vm.__file__).read_bytes()
            receipt = {'name': 'prichal-staging-02', 'imageSHA256': vm.IMAGE_SHA,
                       'operatorSHA256': hashlib.sha256(source).hexdigest()}
            (root / 'bootstrap.json').write_text(json.dumps(receipt))
            before = {}
            for name in ['disk.qcow2', 'seed.iso', 'admin.key', 'host.key', 'vm-control.py']:
                data = source if name == 'vm-control.py' else os.urandom(32)
                (root / name).write_bytes(data)
                (root / name).chmod(0o600)
                before[name] = data
            with patch.object(vm, 'ROOT', root), patch.object(vm, 'plan'), patch.object(vm.Path, 'home', return_value=home), patch.object(vm, 'run') as run:
                vm.bootstrap(root / 'unused-image')
                self.assertFalse(any('qemu-img' in str(call) or 'ssh-keygen' in str(call) for call in run.call_args_list))
            for name, data in before.items():
                self.assertEqual((root / name).read_bytes(), data)


if __name__ == '__main__':
    unittest.main()
