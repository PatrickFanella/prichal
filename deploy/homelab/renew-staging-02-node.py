#!/usr/bin/env python3
"""Renew the fixed staging-02 node leaf with its retained key; no namespace or authority-key changes."""
import argparse
import datetime as dt
import fcntl
import json
import os
import re
from pathlib import Path
import stat
import subprocess
import urllib.parse

ROOT = Path.home() / '.local/state/prichal-staging-02-bootstrap'
NODE = '60684f10-c6b2-4833-a0b8-fb9f96a7869b'
# Retired October 30, 2026; the node then holds no placement until a fresh allocation.
INSTANCE = '2395105c-9958-47b5-ae8b-5b0424cbfc4f'
PLACEMENT = re.compile(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}')
VM = Path('/mnt/fast1/apps/prichal-staging-02')
SSH = ['ssh', '-p', '22270', '-i', str(VM / 'admin.key'), '-o', 'IdentitiesOnly=yes',
       '-o', 'StrictHostKeyChecking=yes', '-o', f'UserKnownHostsFile={VM}/known_hosts',
       '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no',
       '-o', 'ControlPath=none', 'prichal@127.0.0.1']


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=45, **kwargs).stdout


def private(path, directory=False):
    info = path.lstat()
    expected = stat.S_ISDIR if directory else stat.S_ISREG
    if not expected(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Private owned regular custody required')


def environment():
    private(ROOT / 'database.env')
    values = {}
    for line in (ROOT / 'database.env').read_text().splitlines():
        name, value = line.split('=', 1)
        values[name] = value
    url = urllib.parse.urlsplit(values.get('PDS_DATABASE_URL', ''))
    if url.scheme not in ('postgres', 'postgresql') or url.hostname != '127.0.0.1' or url.port != 35437 or url.path != '/pds_local' or url.username != 'pds_staging' or url.query not in ('', 'sslmode=disable') or url.fragment:
        raise ValueError('Dedicated staging-02 registry required')
    env = os.environ.copy()
    env.update(PDS_MODE='local', PDS_DATABASE_URL=values['PDS_DATABASE_URL'],
               PDS_ENROLLMENT_CA_FILE=str(ROOT / 'pki/ca.crt'),
               PDS_ENROLLMENT_CA_KEY_FILE=str(ROOT / 'pki/ca.key'))
    return env


def valid_placements(retained):
    """The one-slot node holds no placement or one well-formed placement."""
    return (isinstance(retained, list) and len(retained) <= 1
            and all(isinstance(item, str) and PLACEMENT.fullmatch(item) for item in retained))


def valid_custody(capacity, retained, before):
    return (valid_placements(before) and retained == before
            and capacity.get('maximumSlots') == 1 and capacity.get('cpus') == 3
            and capacity.get('instanceIds') == before)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--confirm-staging-02', action='store_true')
    if not parser.parse_args().confirm_staging_02:
        raise ValueError('Explicit staging-02 confirmation required')
    os.umask(0o077)
    private(ROOT, True)
    private(ROOT / 'pki', True)
    control = ROOT / 'transport-renewal'
    private(control, True)
    private(control / 'node-admin')
    lock_path = control / 'renewal.lock'
    fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'a') as lock:
        private(lock_path)
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        env = environment()
        for name in ('ca.crt', 'ca.key', 'worker.crt', 'worker.key'):
            private(ROOT / 'pki' / name)
        for role, minimum in [('ca', '172800'), ('worker', '86400')]:
            run(['openssl', 'x509', '-in', str(ROOT / f'pki/{role}.crt'), '-checkend', minimum, '-noout'])
        subject = run(['openssl', 'x509', '-in', str(ROOT / 'pki/ca.crt'), '-noout', '-subject', '-nameopt', 'RFC2253']).decode().strip()
        if subject != 'subject=CN=prichal-staging-02-ca':
            raise ValueError('Wrong staging-02 trust domain')
        command = str(control / 'node-admin')
        retained = json.loads(run([command, 'custody', '--node', NODE], env=env))
        if not valid_placements(retained):
            raise ValueError('Unexpected staging-02 placement custody')
        receipt = control / dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
        receipt.mkdir(mode=0o700)
        before = run(SSH + ['sudo cat /var/lib/prichal-agent/enrolled/node.crt'])
        (receipt / 'node-before.crt').write_bytes(before)
        candidate = receipt / 'node.crt'
        run([command, 'renew', '--node', NODE, '--out', str(candidate)], env=env)
        old_key = run(['openssl', 'x509', '-in', str(receipt / 'node-before.crt'), '-pubkey', '-noout'])
        if run(['openssl', 'x509', '-in', str(candidate), '-pubkey', '-noout']) != old_key:
            raise ValueError('Renewal changed retained node key')
        payload = json.dumps({'ca': (ROOT / 'pki/ca.crt').read_text(), 'certificate': candidate.read_text()}).encode()
        run(SSH + ['sudo /usr/bin/python3 /usr/local/libexec/prichal/publish-certificate.py --staging-02'], input=payload)
        installed = run(SSH + ['sudo cat /var/lib/prichal-agent/enrolled/node.crt'])
        if installed != candidate.read_bytes():
            raise ValueError('Installed certificate readback differs; preserve receipts')
        capacity = json.loads(run(['curl', '-fsS', '--max-time', '10', '--cacert', str(ROOT / 'pki/ca.crt'),
            '--cert', str(ROOT / 'pki/worker.crt'), '--key', str(ROOT / 'pki/worker.key'),
            'https://127.0.0.1:22271/v1/capacity']))
        after = json.loads(run([command, 'custody', '--node', NODE], env=env))
        if not valid_custody(capacity, after, retained):
            raise ValueError('Authenticated capacity/custody differs after publication')
        expiry = run(['openssl', 'x509', '-in', str(candidate), '-noout', '-enddate']).decode().strip()
        result = {'success': True, 'nodeId': NODE, 'instanceIds': retained, 'nodeExpiry': expiry,
                  'retainedKey': True, 'maximumSlots': 1, 'cpus': 3}
        (receipt / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps(result))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Avoid printing database credentials or captured transport response bodies.
        print(json.dumps({'success': False, 'errorType': type(error).__name__}))
        raise SystemExit(1)
