#!/usr/bin/env python3
"""Fixed Dozor pilot operator commands. No HTTP control or management activation."""
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import signal
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = Path('/srv/apps/prichal/pilot')
PKI = Path('/etc/prichal/pilot/pki')
INSTANCE = 'e8e82a65-7aa8-4e37-9306-0de701bf1b19'
STATE = ROOT / 'state'
INGRESS = STATE / (INSTANCE + '-tls.pem')
RUNS = ROOT / 'certificate-runs'
RESULT = ROOT / 'certificate-automation-result.json'
METRICS = Path('/var/lib/prometheus/node-exporter/prichal-pilot/certificates.prom')


def run(*args):
    process = subprocess.run(args, capture_output=True, timeout=60)
    if process.returncode:
        raise RuntimeError('Bounded pilot command failed')
    return process.stdout


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def private(path):
    info = path.lstat()
    if not path.is_file() or path.is_symlink() or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise RuntimeError('Unsafe private pilot file')
    return path.read_bytes()


def expiry(path):
    date = run('openssl', 'x509', '-in', str(path), '-noout', '-enddate').decode().strip().split('=', 1)[1]
    return datetime.datetime.strptime(date, '%b %d %H:%M:%S %Y %Z').replace(tzinfo=datetime.timezone.utc).timestamp()


def status():
    context = ssl.create_default_context(cafile=str(PKI / 'ca.crt'))
    context.load_cert_chain(str(PKI / 'worker.crt'), str(PKI / 'worker.key'))
    with urllib.request.urlopen('https://127.0.0.1:9091/v1/instances/' + INSTANCE, context=context, timeout=15) as response:
        result = json.loads(response.read())
    if result.get('healthy') is not True:
        raise RuntimeError('Pilot agent unhealthy')


def atomic(path, data, mode=0o600):
    fd, temporary = tempfile.mkstemp(prefix='.certificate-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def check():
    dates = {name: expiry(PKI / (name + '.crt')) for name in ['ca', 'agent', 'worker']}
    dates['ingress'] = expiry(INGRESS)
    status()
    return {'expiry': dates, 'healthy': True}


def stage_path(run_id):
    if not re.fullmatch(r'[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}', run_id):
        raise RuntimeError('Invalid renewal run')
    if RUNS.is_symlink():
        raise RuntimeError('Unsafe renewal directory')
    RUNS.mkdir(mode=0o700, exist_ok=True)
    if RUNS.stat().st_uid != os.geteuid() or RUNS.stat().st_mode & 0o077:
        raise RuntimeError('Unsafe renewal directory')
    return RUNS / run_id


def prepare(run_id):
    target = stage_path(run_id)
    spec = importlib.util.spec_from_file_location('pilot_stage', ROOT / 'certificate-tools/stage-certificates.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    argv = sys.argv
    try:
        sys.argv = ['stage-certificates.py', str(target)]
        # Stage's small public report is not a transport message.
        import contextlib
        import io
        with contextlib.redirect_stdout(io.StringIO()):
            module.main()
    finally:
        sys.argv = argv
    return {'run': run_id, 'old': run('openssl', 'x509', '-in', str(target / 'before/ingress.pem'), '-outform', 'PEM').decode(),
            'new': (target / 'ingress.crt').read_text()}


def inspect(name):
    return json.loads(run('docker', 'inspect', name))[0]


def native_receipt():
    result = {}
    for role, name in [('native', 'pds-ass-tenant-'), ('edge', 'pds-ass-edge-'), ('guard', 'pds-ass-enrollment-')]:
        resource = inspect(name + INSTANCE)
        if resource['Config']['Labels'].get('tv.subcult.pds-ass.instance') != INSTANCE or not resource['State']['Running']:
            raise RuntimeError('Pilot runtime identity unavailable')
        result[role] = {'id': resource['Id'], 'mounts': sorted(resource['Mounts'], key=lambda mount: mount['Destination'])}
    result['custody'] = {name: digest(STATE / name) for name in [INSTANCE + '.json', 'pilot-policy.sha256']}
    return result


def replace_contents(path, data):
    private(path)
    # Caddy's existing single-file bind mount requires retaining its inode.
    with path.open('r+b') as file:
        file.write(data)
        file.truncate()
        file.flush()
        os.fsync(file.fileno())


def restart():
    run('docker', 'restart', 'pds-ass-edge-' + INSTANCE)
    run('systemctl', '--user', 'restart', 'prichal-pilot-agent.service')
    for attempt in range(30):
        try:
            status()
            return
        except Exception:
            if attempt == 29:
                raise
            time.sleep(0.3)


def activate(run_id, qualification_failure=False):
    target = stage_path(run_id)
    report = json.loads(private(target / 'staging-report.json'))
    paths = {name: PKI / name for name in ['ca.crt', 'ca.key', 'agent.crt', 'agent.key', 'worker.crt', 'worker.key']}
    paths['ingress.pem'] = INGRESS
    if report['instanceId'] != INSTANCE or any(digest(path) != report['sourceHashes'][name] for name, path in paths.items()):
        raise RuntimeError('Live source changed after staging')
    original = native_receipt()
    changed = ['ca.crt', 'agent.crt', 'worker.crt', 'ingress.pem']
    before = {name: private(target / 'before' / name) for name in changed}
    candidates = {name: private(target / name) for name in changed}
    # Recheck the actual candidates immediately before installation.
    for role, purpose in [('agent', 'sslserver'), ('worker', 'sslclient')]:
        run('openssl', 'verify', '-CAfile', str(target / 'ca.crt'), '-purpose', purpose, str(target / (role + '.crt')))
        pub = run('openssl', 'x509', '-in', str(target / (role + '.crt')), '-pubkey', '-noout')
        if pub != run('openssl', 'pkey', '-in', str(PKI / (role + '.key')), '-pubout'):
            raise RuntimeError('Candidate key differs')
        if run('openssl', 'x509', '-in', str(target / (role + '.crt')), '-noout', '-subject').decode().strip() != 'subject=CN=pds-ass-' + role:
            raise RuntimeError('Candidate role differs')
    for certificate, key in [('ca.crt', PKI / 'ca.key'), ('ingress.pem', INGRESS)]:
        if run('openssl', 'x509', '-in', str(target / certificate), '-pubkey', '-noout') != run('openssl', 'pkey', '-in', str(key), '-pubout'):
            raise RuntimeError('Candidate key differs')
    run('openssl', 'verify', '-CAfile', str(target / 'ingress.crt'), '-purpose', 'sslserver', '-verify_hostname', INSTANCE + '.localhost', str(target / 'ingress.crt'))
    run('openssl', 'verify', '-CAfile', str(target / 'ca.crt'), '-verify_ip', '127.0.0.1', str(target / 'agent.crt'))
    if any(not 6 * 86400 < expiry(target / name) - time.time() <= 7 * 86400 for name in ['agent.crt', 'worker.crt', 'ingress.crt']):
        raise RuntimeError('Candidate validity differs')
    atomic(target / 'activation-started.json', json.dumps({'runtime': original, 'candidateHashes': {name: hashlib.sha256(value).hexdigest() for name, value in candidates.items()}}).encode())
    try:
        for name in changed:
            replace_contents(paths[name], candidates[name])
        restart()
        if qualification_failure:
            raise RuntimeError('Qualification failure after activation')
        if native_receipt() != original or any(digest(paths[name]) != report['sourceHashes'][name] for name in ['ca.key', 'agent.key', 'worker.key']):
            raise RuntimeError('Retained native identity differs')
    except BaseException:
        if any(private(paths[name]) not in [before[name], candidates[name]] for name in changed):
            raise RuntimeError('Certificate changed during activation; retain manual recovery evidence')
        for name in changed:
            replace_contents(paths[name], before[name])
        restart()
        if native_receipt() != original:
            raise RuntimeError('Rollback native identity differs')
        atomic(target / 'rollback-verified.json', b'{"healthy":true,"nativeIdentityPreserved":true}')
        raise
    atomic(target / 'activation-verified.json', b'{"healthy":true,"nativeIdentityPreserved":true}')
    return {'renewed': True, 'nativeIdentityPreserved': True}


def record(success):
    preceding = json.loads(private(RESULT)) if RESULT.exists() else {}
    value = {'last_attempt': time.time(), 'success': success, 'last_success': time.time() if success else preceding.get('last_success', 0)}
    atomic(RESULT, json.dumps(value).encode())


def metrics():
    lines = []
    valid = True
    for role, path in [('ca', PKI / 'ca.crt'), ('agent', PKI / 'agent.crt'), ('worker', PKI / 'worker.crt'), ('ingress', INGRESS)]:
        try:
            date = expiry(path)
        except Exception:
            date = 0
            valid = False
        lines.append(f'prichal_pilot_certificate_not_after_seconds{{role="{role}"}} {date}')
    result = json.loads(private(RESULT)) if RESULT.exists() else {'last_success': 0, 'success': False}
    lines.extend([f'prichal_pilot_certificate_inspection_ok {int(valid)}',
                  f'prichal_pilot_certificate_inspected_seconds {time.time()}',
                  f'prichal_pilot_certificate_operator_success {int(result["success"])}',
                  f'prichal_pilot_certificate_operator_last_success_seconds {result["last_success"]}'])
    atomic(METRICS, ('\n'.join(lines) + '\n').encode(), 0o644)
    return {'metricsWritten': True}


def main():
    os.umask(0o077)
    signal.signal(signal.SIGTERM, lambda *_: (_ for _ in ()).throw(RuntimeError('Operator terminated')))
    action = sys.argv[1] if len(sys.argv) > 1 else ''
    if action == 'check' and len(sys.argv) == 2:
        value = check()
    elif action == 'prepare' and len(sys.argv) == 3:
        value = prepare(sys.argv[2])
    elif action in ['activate', 'qualify-rollback'] and len(sys.argv) == 3:
        value = activate(sys.argv[2], action == 'qualify-rollback')
    elif action == 'record' and len(sys.argv) == 3 and sys.argv[2] in ['success', 'failure']:
        record(sys.argv[2] == 'success')
        value = metrics()
    elif action == 'rollback-state' and len(sys.argv) == 3:
        target = stage_path(sys.argv[2])
        preceding = {name: PKI / name for name in ['ca.crt', 'agent.crt', 'worker.crt']}
        preceding['ingress.pem'] = INGRESS
        value = {'rollbackVerified': (target / 'rollback-verified.json').exists() and
                 all(private(path) == private(target / 'before' / name) for name, path in preceding.items())}
    elif action == 'metrics' and len(sys.argv) == 2:
        value = metrics()
    else:
        raise RuntimeError('Unsupported pilot operator command')
    print(json.dumps(value))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Pilot certificate operation failed; inspect private receipts.', file=sys.stderr)
        sys.exit(1)
