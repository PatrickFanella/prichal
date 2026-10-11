#!/usr/bin/env python3
"""Prepare one dated independent checkpoint; refuse existing restore state."""
import hashlib
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
from urllib.parse import urlsplit, urlunsplit

ROOT = Path('/mnt/fast1/apps/prichal-connection-restore-20261011')
CIPHER_SHA = '7c6195a3f3f0f38b0716584537b64b2ab8a12882569718f65e4d0883fdfaab78'
PLAIN_SHA = '4637cecc01948adc9c633a91c0b97493f3eb9753dc34525d5a30cf576286f7e2'
PREFIXES = ('mnt/fast1/apps/prichal-staging',
            'home/onnwee/.local/state/prichal-staging-control',
            'home/onnwee/.local/state/prichal-staging-management',
            'home/onnwee/.local/state/prichal-staging-recovery/20261011-connection')


def validate_members(members):
    for member in members:
        if member.issym() or member.islnk() or not (member.isfile() or member.isdir()):
            raise ValueError('Restore archive must contain only directories and regular files')
        if '..' in Path(member.name).parts or member.name.startswith('/') or not any(
                member.name == prefix or member.name.startswith(prefix+'/') for prefix in PREFIXES):
            raise ValueError('Archive path is outside the fixed checkpoint scope')


def env(path):
    return dict(line.split('=', 1) for line in path.read_text().splitlines() if '=' in line)


def main():
    os.umask(0o077)
    vm_spec = importlib.util.spec_from_file_location('staging_vm', Path(__file__).with_name('staging-vm.py'))
    vm_operator = importlib.util.module_from_spec(vm_spec)
    vm_spec.loader.exec_module(vm_operator)
    vm_operator.check_mount()
    ROOT.mkdir(mode=0o700)  # Exclusive: a failed or previous fixture is retained for inspection.
    for remote, name in [('/srv/recovery/kvant/prichal-staging/staging-connection-20261011.pvm', 'archive.pvm'),
                         ('/home/onnwee/.local/state/prichal-staging-recovery/keys/node-1ef77d45.key', 'recovery.key')]:
        subprocess.run(['scp', '-q', 'dozor:'+remote, str(ROOT / name)], check=True, timeout=180)
    with (ROOT / 'archive.pvm').open('rb') as incoming:
        if hashlib.file_digest(incoming, 'sha256').hexdigest() != CIPHER_SHA:
            raise ValueError('Independent checkpoint ciphertext differs')
    spec = importlib.util.spec_from_file_location('recovery_crypto', Path(__file__).with_name('recovery-crypto.py'))
    crypto = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(crypto)
    crypto.transform('decrypt', ROOT / 'archive.pvm', ROOT / 'archive.tar.gz', ROOT / 'recovery.key', PLAIN_SHA)
    with tarfile.open(ROOT / 'archive.tar.gz') as archive:
        validate_members(archive.getmembers())
        archive.extractall(ROOT / 'extracted', filter='data')
    source = Path(__file__).resolve().parent
    for name in ['restore-staging-vm.py', 'staging-vm.py', 'restore-management.compose.yaml']:
        shutil.copy2(source / name, ROOT / name)
    vm = ROOT / 'extracted/mnt/fast1/apps/prichal-staging'
    (ROOT / 'known_hosts').write_text('[127.0.0.1]:22253 '+(vm / 'host.key.pub').read_text())
    management = ROOT / 'extracted/home/onnwee/.local/state/prichal-staging-management'
    control = ROOT / 'extracted/home/onnwee/.local/state/prichal-staging-control'
    api = env(management / 'api.env')
    database = urlsplit(api['PDS_DATABASE_URL'])
    api['PDS_DATABASE_URL'] = urlunsplit(database._replace(netloc=database.netloc.rsplit('@', 1)[0]+'@postgres:5432'))
    (ROOT / 'api.env').write_text('\n'.join(k+'='+v for k, v in api.items())+'\n')
    node = subprocess.check_output(['docker', 'image', 'inspect', 'node:24-bookworm-slim', '--format', '{{.Id}}'], text=True, timeout=30).strip()
    (ROOT / 'compose.env').write_text('\n'.join([
        'PDS_DB_PASSWORD='+env(control / 'registry.env')['PDS_DB_PASSWORD'],
        'RESTORE_ROOT='+str(ROOT), 'RESTORE_MANAGEMENT_ROOT='+str(management),
        'RESTORE_CHECK_ROOT='+str(source.parents[1] / 'scripts'), 'RESTORE_NODE_IMAGE='+node])+'\n')
    print('Independent archive/key authenticated; fixture prepared without starting services')


if __name__ == '__main__':
    main()
