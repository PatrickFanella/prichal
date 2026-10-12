#!/usr/bin/env python3
"""Retrieve one fixed public-pilot checkpoint; never install its operator units."""
import hashlib
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

ROOT = Path('/mnt/fast1/apps/prichal-public-pds-restore-20261012')
CIPHER_SHA = '27d84355def24423e03044d1b7642b818d5701665f649237de31b95c58fdb19f'
PLAIN_SHA = '60633f3555dd91dda61b1df73171f71c3e9ed571cb0caeef3badea558353eb3c'
PREFIXES = ('mnt/fast1/apps/prichal-public-pds',
            'home/onnwee/.local/state/prichal-kvant-public-pds',
            'home/onnwee/.local/state/prichal-kvant-public-pds-metrics',
            'home/onnwee/.local/state/prichal-public-pds-recovery/20261012-operated/units',
            'home/onnwee/.local/state/prichal-public-pds-recovery/20261012-operated/external',
            'home/onnwee/.local/state/prichal-public-pds-recovery/20261012-operated/source-receipt.json')


def validate_members(members):
    seen = set()
    for member in members:
        canonical = Path(member.name).as_posix()
        if member.issym() or member.islnk() or not (member.isfile() or member.isdir()):
            raise ValueError('Only regular files/directories are permitted')
        allowed = canonical == PREFIXES[-1] or any(
            canonical == prefix or canonical.startswith(prefix + '/') for prefix in PREFIXES[:-1])
        if canonical in seen or '..' in Path(member.name).parts or member.name.startswith('/') or not allowed:
            raise ValueError('Archive path is outside fixed checkpoint scope or duplicated')
        seen.add(canonical)


def main():
    os.umask(0o077)
    source = Path(__file__).resolve().parent
    spec = importlib.util.spec_from_file_location('public_vm', source / 'public-pds-vm.py')
    vm_operator = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vm_operator)
    vm_operator.check_mount()
    ROOT.mkdir(mode=0o700)
    for remote, name in [('/srv/recovery/kvant/prichal-public-pds/kvant-public-pds-20261012-operated.pvm', 'archive.pvm'),
                         ('/home/onnwee/.local/state/prichal-public-pds-recovery/keys/kvant-public-pds-9f148c55.key', 'recovery.key')]:
        subprocess.run(['scp', '-q', 'dozor:' + remote, str(ROOT / name)], check=True, timeout=180)
    with (ROOT / 'archive.pvm').open('rb') as file:
        if hashlib.file_digest(file, 'sha256').hexdigest() != CIPHER_SHA:
            raise ValueError('Independent ciphertext differs from trusted capture')
    spec = importlib.util.spec_from_file_location('crypto', source / 'recovery-crypto.py')
    crypto = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(crypto)
    crypto.transform('decrypt', ROOT / 'archive.pvm', ROOT / 'archive.tar.gz', ROOT / 'recovery.key', PLAIN_SHA)
    with tarfile.open(ROOT / 'archive.tar.gz') as archive:
        validate_members(archive.getmembers())
        archive.extractall(ROOT / 'extracted', filter='data')
    for name in ['restore-public-pds-vm.py', 'public-pds-vm.py']:
        shutil.copy2(source / name, ROOT / name)
    vm = ROOT / 'extracted/mnt/fast1/apps/prichal-public-pds'
    (ROOT / 'known_hosts').write_text('[127.0.0.1]:22263 ' + (vm / 'host.key.pub').read_text())
    print('Independent archive/key authenticated; units retained as inert evidence; no guest started')


if __name__ == '__main__':
    main()
