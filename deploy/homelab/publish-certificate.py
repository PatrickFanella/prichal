#!/usr/bin/env python3
"""Guest-root publisher for the fixed staging public certificate pair."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path('/var/lib/prichal-agent')
NODE = '1ef77d45-c6d5-4bdb-b77b-ba1103b45a66'


def output(*args):
    return subprocess.run(args, check=True, capture_output=True, timeout=10).stdout


def replace(path, data, uid, gid):
    fd, name = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        os.chown(name, uid, gid)
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        Path(name).unlink(missing_ok=True)


def main():
    if os.geteuid() != 0:
        raise ValueError('Guest root required')
    data = sys.stdin.buffer.read(16385)
    if len(data) > 16384:
        raise ValueError('Certificate payload too large')
    payload = json.loads(data)
    if set(payload) != {'ca', 'certificate'} or not all(isinstance(v, str) for v in payload.values()):
        raise ValueError('Fixed public certificate fields required')
    ca, cert, key = ROOT / 'ca.crt', ROOT / 'enrolled/node.crt', ROOT / 'enrolled/node.key'
    for path in [ROOT, ROOT / 'enrolled', ca, cert, key]:
        info = path.lstat()
        if path.is_symlink() or info.st_mode & 0o077:
            raise ValueError('Private regular custody paths required')
    old_ca, old_cert = ca.read_bytes(), cert.read_bytes()
    info = cert.stat()
    with tempfile.TemporaryDirectory(dir=ROOT) as directory:
        candidate_ca, candidate_cert = Path(directory) / 'ca.crt', Path(directory) / 'node.crt'
        candidate_ca.write_text(payload['ca'])
        candidate_cert.write_text(payload['certificate'])
        for path in [candidate_ca, candidate_cert]:
            path.chmod(0o600)
        if output('openssl', 'x509', '-in', str(candidate_ca), '-pubkey', '-noout') != output('openssl', 'x509', '-in', str(ca), '-pubkey', '-noout'):
            raise ValueError('Trust key replacement refused')
        if output('openssl', 'x509', '-in', str(candidate_cert), '-pubkey', '-noout') != output('openssl', 'pkey', '-in', str(key), '-pubout'):
            raise ValueError('Node key replacement refused')
        subject = output('openssl', 'x509', '-in', str(candidate_cert), '-noout', '-subject', '-nameopt', 'RFC2253').decode().strip()
        if subject != 'subject=CN=pds-ass-node-' + NODE:
            raise ValueError('Foreign node identity')
        output('openssl', 'verify', '-CAfile', str(candidate_ca), '-purpose', 'sslserver', '-verify_ip', '127.0.0.1', str(candidate_cert))
        output('openssl', 'x509', '-in', str(candidate_cert), '-checkend', '3600', '-noout')
        if ca.read_bytes() != old_ca or cert.read_bytes() != old_cert:
            raise ValueError('Concurrent certificate mutation')
        try:
            replace(ca, candidate_ca.read_bytes(), info.st_uid, info.st_gid)
            replace(cert, candidate_cert.read_bytes(), info.st_uid, info.st_gid)
            output('systemctl', 'restart', 'prichal-staging-agent.service')
            output('systemctl', 'is-active', 'prichal-staging-agent.service')
        except Exception:
            replace(ca, old_ca, info.st_uid, info.st_gid)
            replace(cert, old_cert, info.st_uid, info.st_gid)
            output('systemctl', 'restart', 'prichal-staging-agent.service')
            raise
    print('Retained-key certificate installed; agent restarted.')


if __name__ == '__main__':
    main()
