import importlib.util
from pathlib import Path
import tarfile
import unittest

spec = importlib.util.spec_from_file_location('prepare_restore', Path(__file__).with_name('prepare-staging-restore.py'))
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)


class RestoreArchiveScope(unittest.TestCase):
    def test_owned_regular_file(self):
        restore.validate_members([tarfile.TarInfo(restore.PREFIXES[0]+'/disk.qcow2')])

    def test_foreign_and_traversal_paths(self):
        for name in ['/mnt/fast1/apps/prichal-staging/disk.qcow2',
                     'mnt/fast1/apps/prichal-staging/../../foreign', 'etc/passwd']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                restore.validate_members([tarfile.TarInfo(name)])

    def test_links_and_special_files(self):
        for kind in [tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.CHRTYPE, tarfile.FIFOTYPE]:
            member = tarfile.TarInfo(restore.PREFIXES[0]+'/foreign')
            member.type = kind
            with self.subTest(kind=kind), self.assertRaises(ValueError):
                restore.validate_members([member])


if __name__ == '__main__':
    unittest.main()
