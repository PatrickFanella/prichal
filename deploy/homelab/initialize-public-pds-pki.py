#!/usr/bin/env python3
"""Create fresh transport trust for the fixed Kvant pilot; never replace retained keys."""
import os
from pathlib import Path
import subprocess

ROOT = Path('/home/onnwee/.local/state/prichal-kvant-public-pds/pki')


def openssl(*args):
    subprocess.run(['openssl', *map(str, args)], check=True, stdout=subprocess.DEVNULL,
                   stderr=subprocess.DEVNULL, timeout=30)


def main():
    os.umask(0o077)
    for directory in [ROOT.parent, ROOT]:
        info = directory.lstat()
        if directory.is_symlink() or not directory.is_dir() or info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise RuntimeError('Owned private pilot directory required')
    if any(ROOT.iterdir()):
        raise RuntimeError('Retain existing PKI; initialization cannot replace keys')
    # The guest generates its agent key. Only its public CSR leaves that guest.
    csr = subprocess.check_output([
        'ssh', '-p', '22260', '-i', '/mnt/fast1/apps/prichal-public-pds/admin.key',
        '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes',
        '-o', 'UserKnownHostsFile=/mnt/fast1/apps/prichal-public-pds/known_hosts',
        'prichal@127.0.0.1', 'sudo cat /var/lib/prichal-public-pds/agent.csr'], timeout=30)
    (ROOT / 'agent.csr').write_bytes(csr)
    openssl('genpkey', '-algorithm', 'ED25519', '-out', ROOT / 'ca.key')
    openssl('req', '-x509', '-new', '-key', ROOT / 'ca.key', '-days', '90',
            '-subj', '/CN=prichal-kvant-public-pds-ca', '-addext', 'basicConstraints=critical,CA:TRUE',
            '-addext', 'keyUsage=critical,keyCertSign,cRLSign', '-addext', 'subjectKeyIdentifier=hash',
            '-addext', 'authorityKeyIdentifier=keyid:always', '-out', ROOT / 'ca.crt')
    openssl('genpkey', '-algorithm', 'ED25519', '-out', ROOT / 'worker.key')
    openssl('req', '-new', '-key', ROOT / 'worker.key', '-subj', '/CN=pds-ass-worker', '-out', ROOT / 'worker.csr')
    for role, usage in [('agent', 'serverAuth'), ('worker', 'clientAuth')]:
        extensions = ('basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\n'
                      'extendedKeyUsage='+usage+'\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid:always\n')
        if role == 'agent':
            extensions += 'subjectAltName=IP:127.0.0.1,DNS:localhost\n'
        (ROOT / (role+'.cnf')).write_text(extensions)
        openssl('x509', '-req', '-in', ROOT / (role+'.csr'), '-CA', ROOT / 'ca.crt',
                '-CAkey', ROOT / 'ca.key', '-set_serial', '0x'+os.urandom(16).hex(), '-days', '7',
                '-extfile', ROOT / (role+'.cnf'), '-out', ROOT / (role+'.crt'))
        openssl('verify', '-CAfile', ROOT / 'ca.crt', '-purpose',
                'sslserver' if role == 'agent' else 'sslclient', ROOT / (role+'.crt'))
    for path in ROOT.iterdir():
        path.chmod(0o600)
    print('Dedicated transport trust initialized; agent key remains in guest')


if __name__ == '__main__':
    main()
