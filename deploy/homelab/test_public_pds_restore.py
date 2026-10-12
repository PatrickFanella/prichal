import importlib.util
from pathlib import Path
import tarfile
import unittest

spec = importlib.util.spec_from_file_location('public_restore', Path(__file__).with_name('prepare-public-pds-restore.py'))
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)


class PublicRestoreScope(unittest.TestCase):
    def test_current_vm_and_operator_files_are_accepted(self):
        for prefix in restore.PREFIXES:
            restore.validate_members([tarfile.TarInfo(prefix if prefix.endswith('.json') else prefix + '/regular')])

    def test_previous_or_foreign_namespaces_cannot_be_extracted(self):
        for name in ['etc/passwd', '/mnt/fast1/apps/prichal-public-pds/disk.qcow2',
                     'mnt/fast1/apps/prichal-public-pds/../../foreign',
                     'mnt/fast1/apps/prichal-staging/disk.qcow2',
                     'home/onnwee/.local/state/prichal-kvant-public-pds-other/key']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                restore.validate_members([tarfile.TarInfo(name)])

    def test_links_special_files_and_duplicate_paths_are_refused(self):
        name = restore.PREFIXES[0] + '/disk.qcow2'
        for kind in [tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.CHRTYPE, tarfile.FIFOTYPE]:
            member = tarfile.TarInfo(name)
            member.type = kind
            with self.subTest(kind=kind), self.assertRaises(ValueError):
                restore.validate_members([member])
        with self.assertRaises(ValueError):
            restore.validate_members([tarfile.TarInfo(name), tarfile.TarInfo(name)])
        with self.assertRaises(ValueError):
            restore.validate_members([tarfile.TarInfo(name), tarfile.TarInfo(restore.PREFIXES[0] + '/./disk.qcow2')])


if __name__ == '__main__':
    unittest.main()
