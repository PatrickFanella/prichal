#!/usr/bin/env python3
"""Retained-key certificate coordination for the fixed Kvant public pilot."""
import datetime as dt
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shlex
import ssl
import subprocess
import sys
import time

ROOT = Path.home() / '.local/state/prichal-kvant-public-pds'
VM = Path('/mnt/fast1/apps/prichal-public-pds')
GUEST = '/usr/local/libexec/prichal-public-pds/certificate-guest.py'
TRUST = '/usr/local/libexec/prichal-kvant-public-pds-trust.py'
SSH = ['ssh', '-p', '22260', '-i', str(VM / 'admin.key'), '-o', 'IdentitiesOnly=yes',
       '-o', 'StrictHostKeyChecking=yes', '-o', f'UserKnownHostsFile={VM}/known_hosts',
       '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no',
       '-o', 'ControlPath=none', 'prichal@127.0.0.1']
RECOVERY_TIMER = 'prichal-public-pds-reconcile.timer'


def run(args, body=None):
    result = subprocess.run(list(map(str, args)), input=body, capture_output=True, timeout=90)
    if result.returncode:
        raise RuntimeError('Fixed certificate coordination command failed')
    return result.stdout


def guest(action, **value):
    return json.loads(run(SSH + [shlex.join(['sudo', '-n', '/usr/bin/python3', GUEST])],
                          json.dumps({'action': action, **value}).encode()))


def trust(value):
    command = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
               '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
               'almaz', shlex.join(['sudo', '-n', '/usr/bin/python3', TRUST])]
    return json.loads(run(command, json.dumps(value).encode()))


def expiry(path):
    text = run(['openssl', 'x509', '-in', path, '-enddate', '-noout']).decode().strip().split('=', 1)[1]
    return dt.datetime.strptime(text, '%b %d %H:%M:%S %Y %Z').replace(tzinfo=dt.timezone.utc).timestamp()


def due(dates, now, force=False):
    return force or dates['ca.crt'] - now <= 30 * 86400 or min(
        dates[name] - now for name in ['agent.crt', 'worker.crt', 'ingress.pem']) <= 3 * 86400


def public_check():
    # Existing fixed-origin harness checks metadata, signup closure and WebSocket.
    run([str(ROOT / 'bin/node'), str(ROOT / 'bin/qualify-kvant-public-pds.mjs')])


def health(recovery):
    for name in ['ca.crt', 'worker.crt', 'worker.key']:
        recovery.private(ROOT / 'pki' / name)
    context = ssl.create_default_context(cafile=str(ROOT / 'pki/ca.crt'))
    context.load_cert_chain(str(ROOT / 'pki/worker.crt'), str(ROOT / 'pki/worker.key'))
    return recovery.request(context, 'GET')['healthy']


def stage(path, candidate, recovery):
    pki = ROOT / 'pki'
    before = path / 'before'
    before.mkdir(mode=0o700)
    for name in ['ca.crt', 'agent.crt', 'worker.crt', 'ca.key', 'worker.key']:
        recovery.private(pki / name)
    if candidate['ca'].encode() != (pki / 'ca.crt').read_bytes() or candidate['agent'].encode() != (pki / 'agent.crt').read_bytes():
        raise RuntimeError('Transport certificates differ between host and guest')
    for name in ['ca.crt', 'agent.crt', 'worker.crt']:
        (before / name).write_bytes((pki / name).read_bytes())
    (path / 'agent.csr').write_text(candidate['csr'])
    run(['openssl', 'req', '-in', path / 'agent.csr', '-verify', '-noout'])
    csr_key = run(['openssl', 'req', '-in', path / 'agent.csr', '-pubkey', '-noout'])
    if csr_key != run(['openssl', 'x509', '-in', pki / 'agent.crt', '-pubkey', '-noout']):
        raise RuntimeError('Guest CSR changed retained agent key')
    for role in ['ca', 'worker']:
        if run(['openssl', 'x509', '-in', pki / (role + '.crt'), '-pubkey', '-noout']) != run(['openssl', 'pkey', '-in', pki / (role + '.key'), '-pubout']):
            raise RuntimeError('Retained signer/operator key mismatch')
    run(['openssl', 'req', '-x509', '-new', '-key', pki / 'ca.key', '-days', '90',
        '-subj', '/CN=prichal-kvant-public-pds-ca', '-addext', 'basicConstraints=critical,CA:TRUE,pathlen:0',
        '-addext', 'keyUsage=critical,keyCertSign,cRLSign', '-addext', 'subjectKeyIdentifier=hash',
        '-addext', 'authorityKeyIdentifier=keyid:always', '-out', path / 'ca.crt'])
    run(['openssl', 'req', '-new', '-key', pki / 'worker.key', '-subj', '/CN=pds-ass-worker', '-out', path / 'worker.csr'])
    for role, usage in [('agent', 'serverAuth'), ('worker', 'clientAuth')]:
        extensions = ('basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\n'
            'extendedKeyUsage=' + usage + '\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid:always\n')
        if role == 'agent':
            extensions += 'subjectAltName=IP:127.0.0.1,DNS:localhost\n'
        (path / (role + '.cnf')).write_text(extensions)
        run(['openssl', 'x509', '-req', '-in', path / (role + '.csr'), '-CA', path / 'ca.crt',
             '-CAkey', pki / 'ca.key', '-set_serial', '0x' + secrets.token_hex(16), '-days', '7',
             '-extfile', path / (role + '.cnf'), '-out', path / (role + '.crt')])
        run(['openssl', 'verify', '-x509_strict', '-CAfile', path / 'ca.crt', '-purpose',
             'sslserver' if role == 'agent' else 'sslclient', path / (role + '.crt')])
    return {name: hashlib.sha256((pki / name).read_bytes()).hexdigest()
            for name in ['ca.crt', 'agent.crt', 'worker.crt', 'ca.key', 'worker.key']}


def publish(path, value):
    # Same UID/private parent, fixed paths; retain previous bytes in run receipt.
    temporary = path.with_name('.' + path.name + '.renewing')
    with temporary.open('xb') as file:
        file.write(value)
        file.flush()
        os.fsync(file.fileno())
    os.replace(temporary, path)
    directory = os.open(path.parent, os.O_DIRECTORY)
    try:
        os.fsync(directory)
    finally:
        os.close(directory)


def main():
    os.umask(0o077)
    if sys.argv[1:] not in [[], ['--qualify-renewal'], ['--qualify-rollback']]:
        raise RuntimeError('Unsupported renewal option')
    spec = importlib.util.spec_from_file_location('public_recovery', ROOT / 'bin/reconcile-public-pds.py')
    recovery = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(recovery)
    recovery.private(ROOT, True)
    recovery.private(ROOT / 'pki', True)
    paused = ROOT / 'reconciliation-paused'
    if paused.exists() or paused.is_symlink():
        raise RuntimeError('Pilot maintenance pause active')
    lock_path = ROOT / 'certificates.lock'
    if lock_path.exists() or lock_path.is_symlink():
        recovery.private(lock_path)
    with lock_path.open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        run_id = dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(4)
        parent = ROOT / 'renewals'
        parent.mkdir(mode=0o700, exist_ok=True)
        recovery.private(parent, True)
        path = parent / run_id
        path.mkdir(mode=0o700)
        report = {'run': run_id, 'success': False, 'renewed': False, 'rollbackVerified': False}
        prepared = activated = stopped_timer = False
        try:
            if not health(recovery):
                raise RuntimeError('Pilot unhealthy before inspection')
            initial = guest('check')
            recovery.private(ROOT / 'pki/worker.crt')
            dates = dict(initial['expiry'], **{'worker.crt': expiry(ROOT / 'pki/worker.crt')})
            report['expiry'] = dates
            if due(dates, time.time(), bool(sys.argv[1:])):
                if subprocess.run(['systemctl', '--user', 'is-active', '--quiet', RECOVERY_TIMER]).returncode == 0:
                    run(['systemctl', '--user', 'stop', RECOVERY_TIMER])
                    stopped_timer = True
                run(['systemctl', '--user', 'stop', 'prichal-public-pds-reconcile.service'])
                candidate = guest('prepare', run=run_id)
                hashes = stage(path, candidate, recovery)
                trust({'action': 'prepare', 'run': run_id, 'old': candidate['old'], 'new': candidate['new']})
                prepared = True
                guest('qualify-rollback' if sys.argv[1:] == ['--qualify-rollback'] else 'activate', run=run_id,
                      ca=(path / 'ca.crt').read_text(), agent=(path / 'agent.crt').read_text())
                activated = True
                if any(hashlib.sha256((ROOT / 'pki' / name).read_bytes()).hexdigest() != value for name, value in hashes.items()):
                    raise RuntimeError('Host PKI changed during activation; inspect receipts')
                for name in ['ca.crt', 'agent.crt', 'worker.crt']:
                    publish(ROOT / 'pki' / name, (path / name).read_bytes())
                final = guest('check')
                if final['runtime'] != initial['runtime']:
                    raise RuntimeError('Retained runtime changed after renewal')
                report['renewed'] = True
                report['expiry'] = dict(final['expiry'], **{'worker.crt': expiry(ROOT / 'pki/worker.crt')})
            # Retry only read-only health while the restarted ingress warms up.
            for attempt in range(10):
                try:
                    if not health(recovery):
                        raise RuntimeError('Pilot health incomplete')
                    public_check()
                    break
                except Exception:
                    if attempt == 9:
                        raise
                    time.sleep(1)
            report['success'] = True
        except BaseException as error:
            report['errorType'] = type(error).__name__
            if prepared and not activated:
                try:
                    if guest('rollback-state', run=run_id)['rollbackVerified']:
                        if not health(recovery):
                            raise RuntimeError('Rollback agent unhealthy')
                        trust({'action': 'rollback', 'run': run_id})
                        public_check()
                        report['rollbackVerified'] = True
                    else:
                        report['manualInspectionRequired'] = True
                except Exception:
                    report['manualInspectionRequired'] = True
            if not (sys.argv[1:] == ['--qualify-rollback'] and report['rollbackVerified']):
                raise
        finally:
            if stopped_timer:
                run(['systemctl', '--user', 'start', RECOVERY_TIMER])
            report['checkedAt'] = time.time()
            (path / 'result.json').write_text(json.dumps(report, indent=2))
            last = ROOT / 'last-certificates.json'
            if last.exists() or last.is_symlink():
                recovery.private(last)
                publish(last, json.dumps(report).encode())
            else:
                with last.open('xb') as file:
                    file.write(json.dumps(report).encode())
        print(json.dumps(report))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Public pilot renewal failed; inspect private receipts.', file=sys.stderr)
        sys.exit(1)
