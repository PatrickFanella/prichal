#!/usr/bin/env python3
"""Renew the fixed staging-02 node leaf and worker identity with retained keys; no namespace or authority-key changes."""
import argparse
import datetime as dt
import fcntl
import json
import os
import re
from pathlib import Path
import secrets
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
NODE_WINDOW = dt.timedelta(hours=8)
WORKER_WINDOW = dt.timedelta(days=3)
# CA rotation replaces trust on the guest and in the registry; it stays a reviewed manual operation.
CA_WINDOW = dt.timedelta(days=30)
WORKER_DAYS = 7


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


def expiry(pem):
    text = run(['openssl', 'x509', '-noout', '-enddate'], input=pem).decode().strip()
    if not text.startswith('notAfter='):
        raise ValueError('Certificate expiry unavailable')
    return dt.datetime.strptime(text[9:], '%b %d %H:%M:%S %Y GMT').replace(tzinfo=dt.timezone.utc)


def public_key(pem, kind='x509'):
    command = ['openssl', 'x509', '-noout', '-pubkey'] if kind == 'x509' else ['openssl', 'pkey', '-pubout']
    return run(command, input=pem)


def plan(now, node, worker, ca, force_node=False):
    """Return the renewals due at `now`; CA expiry refuses because rotation is manual."""
    if expiry(ca) - now <= CA_WINDOW:
        raise ValueError('Staging-02 CA is within its rotation window; rotate it manually')
    return {'worker': expiry(worker) - now <= WORKER_WINDOW,
            'node': force_node or expiry(node) - now <= NODE_WINDOW}


def renew_worker(receipt):
    """Re-sign the retained worker CSR; the private key and subject stay unchanged."""
    pki = ROOT / 'pki'
    previous = (pki / 'worker.crt').read_bytes()
    key = public_key((pki / 'worker.key').read_bytes(), 'key')
    csr = (pki / 'worker.csr').read_bytes()
    if run(['openssl', 'req', '-noout', '-pubkey'], input=csr) != key or public_key(previous) != key:
        raise ValueError('Retained worker request or certificate differs from its key')
    (receipt / 'worker-before.crt').write_bytes(previous)
    candidate = receipt / 'worker.crt'
    run(['openssl', 'x509', '-req', '-in', str(pki / 'worker.csr'), '-CA', str(pki / 'ca.crt'),
         '-CAkey', str(pki / 'ca.key'), '-set_serial', '0x' + secrets.token_hex(16),
         '-days', str(WORKER_DAYS), '-extfile', str(pki / 'worker.cnf'), '-out', str(candidate)])
    data = candidate.read_bytes()
    subject = run(['openssl', 'x509', '-noout', '-subject', '-nameopt', 'RFC2253'], input=data).decode().strip()
    if public_key(data) != key or subject != 'subject=CN=pds-ass-worker':
        raise ValueError('Worker candidate changed identity')
    run(['openssl', 'verify', '-CAfile', str(pki / 'ca.crt'), '-purpose', 'sslclient', str(candidate)])
    if expiry(data) <= expiry(previous):
        raise ValueError('Worker candidate does not extend validity')
    replace(pki / 'worker.crt', data, previous)
    return previous


def rotate_worker(receipt, verify):
    previous = renew_worker(receipt)
    try:
        verify()
    except Exception:
        # The previous leaf is still valid; restore it rather than keep an unproven client identity.
        replace(ROOT / 'pki/worker.crt', previous, (receipt / 'worker.crt').read_bytes())
        raise


def replace(path, data, expected):
    if path.read_bytes() != expected:
        raise ValueError('Concurrent certificate change; preserve receipts')
    temporary = path.with_name('.' + path.name + '.candidate')
    temporary.unlink(missing_ok=True)
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as output:
        output.write(data)
        output.flush()
        os.fsync(output.fileno())
    os.replace(temporary, path)
    fd = os.open(path.parent, os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def capacity():
    return json.loads(run(['curl', '-fsS', '--max-time', '10', '--cacert', str(ROOT / 'pki/ca.crt'),
        '--cert', str(ROOT / 'pki/worker.crt'), '--key', str(ROOT / 'pki/worker.key'),
        'https://127.0.0.1:22271/v1/capacity']))


def checked(command, env, before):
    after = json.loads(run([command, 'custody', '--node', NODE], env=env))
    if not valid_custody(capacity(), after, before):
        raise ValueError('Authenticated capacity/custody differs')


def renew_node(command, env, receipt, before):
    (receipt / 'node-before.crt').write_bytes(before)
    candidate = receipt / 'node.crt'
    run([command, 'renew', '--node', NODE, '--out', str(candidate)], env=env)
    if public_key(candidate.read_bytes()) != public_key(before):
        raise ValueError('Renewal changed retained node key')
    payload = json.dumps({'ca': (ROOT / 'pki/ca.crt').read_text(), 'certificate': candidate.read_text()}).encode()
    run(SSH + ['sudo /usr/bin/python3 /usr/local/libexec/prichal/publish-certificate.py --staging-02'], input=payload)
    installed = run(SSH + ['sudo cat /var/lib/prichal-agent/enrolled/node.crt'])
    if installed != candidate.read_bytes():
        raise ValueError('Installed certificate readback differs; preserve receipts')
    return installed


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--confirm-staging-02', action='store_true')
    parser.add_argument('--force-node', action='store_true', help='renew the node leaf even when it is not yet due')
    args = parser.parse_args()
    if not args.confirm_staging_02:
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
        try:
            renew(args, control)
        except Exception as error:
            # Record the failed attempt so monitoring sees it before the success heartbeat ages out.
            replace_status(control / 'status.json', {
                'success': False, 'checkedAt': dt.datetime.now(dt.timezone.utc).isoformat(),
                'nodeId': NODE, 'errorType': type(error).__name__})
            raise


def renew(args, control):
    env = environment()
    for name in ('ca.crt', 'ca.key', 'worker.crt', 'worker.key', 'worker.csr', 'worker.cnf'):
        private(ROOT / 'pki' / name)
    ca = (ROOT / 'pki/ca.crt').read_bytes()
    subject = run(['openssl', 'x509', '-noout', '-subject', '-nameopt', 'RFC2253'], input=ca).decode().strip()
    if subject != 'subject=CN=prichal-staging-02-ca':
        raise ValueError('Wrong staging-02 trust domain')
    command = str(control / 'node-admin')
    retained = json.loads(run([command, 'custody', '--node', NODE], env=env))
    if not valid_placements(retained):
        raise ValueError('Unexpected staging-02 placement custody')
    node = run(SSH + ['sudo cat /var/lib/prichal-agent/enrolled/node.crt'])
    now = dt.datetime.now(dt.timezone.utc)
    due = plan(now, node, (ROOT / 'pki/worker.crt').read_bytes(), ca, args.force_node)
    receipt = None
    if due['worker'] or due['node']:
        receipt = control / now.strftime('%Y%m%dT%H%M%S%fZ')
        receipt.mkdir(mode=0o700)
    # The node leaf first: its renewal needs no worker identity, while proving a new
    # worker leaf over mTLS needs a valid node leaf.
    if due['node']:
        node = renew_node(command, env, receipt, node)
    if due['worker']:
        rotate_worker(receipt, lambda: checked(command, env, retained))
    # Every run, including a no-op, proves current mTLS and unchanged custody.
    checked(command, env, retained)
    result = {'success': True, 'checkedAt': now.isoformat(), 'nodeId': NODE, 'instanceIds': retained,
              'renewed': due, 'nodeExpiry': expiry(node).isoformat(),
              'workerExpiry': expiry((ROOT / 'pki/worker.crt').read_bytes()).isoformat(),
              'caExpiry': expiry(ca).isoformat(), 'retainedKey': True, 'maximumSlots': 1, 'cpus': 3}
    if receipt is not None:
        (receipt / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
    replace_status(control / 'status.json', result)
    print(json.dumps(result))


def replace_status(path, result):
    data = (json.dumps(result, indent=2) + '\n').encode()
    temporary = path.with_name('.status.json.candidate')
    temporary.unlink(missing_ok=True)
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as output:
        output.write(data)
        output.flush()
        os.fsync(output.fileno())
    os.replace(temporary, path)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Avoid printing database credentials or captured transport response bodies.
        print(json.dumps({'success': False, 'errorType': type(error).__name__}))
        raise SystemExit(1)
