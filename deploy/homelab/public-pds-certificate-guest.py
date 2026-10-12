#!/usr/bin/env python3
"""Root-owned fixed pilot certificate helper; guest private keys never leave it."""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import subprocess
import sys

ROOT = Path('/var/lib/prichal-public-pds')
RUNS = Path('/var/lib/prichal-public-pds-certificates')
INSTANCE = '9f148c55-da61-486b-97f4-50df26b65893'
INGRESS = ROOT / 'state' / (INSTANCE + '-tls.pem')
UID = pwd.getpwnam('prichal-agent').pw_uid


def run(*args, body=None):
    result = subprocess.run(list(map(str, args)), input=body, capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError('Fixed guest command failed')
    return result.stdout


def private(path, owner=UID):
    info = path.lstat()
    if path.is_symlink() or not path.is_file() or info.st_uid != owner or info.st_mode & 0o077:
        raise RuntimeError('Unsafe private guest material')
    return path.read_bytes()


def digest(path):
    return hashlib.sha256(private(path)).hexdigest()


def expiry(path):
    text = run('openssl', 'x509', '-in', path, '-enddate', '-noout').decode().strip().split('=', 1)[1]
    return dt.datetime.strptime(text, '%b %d %H:%M:%S %Y %Z').replace(tzinfo=dt.timezone.utc).timestamp()


def material():
    return {'ca.crt': ROOT / 'ca.crt', 'agent.crt': ROOT / 'agent.crt', 'ingress.pem': INGRESS}


def runtime():
    result = {}
    for role, prefix in [('native', 'pds-ass-tenant-'), ('edge', 'pds-ass-edge-')]:
        item = json.loads(run('docker', 'inspect', prefix + INSTANCE))[0]
        if item['Config']['Labels'].get('tv.subcult.pds-ass.instance') != INSTANCE:
            raise RuntimeError('Foreign runtime identity')
        result[role] = {'id': item['Id'], 'mountsHash': hashlib.sha256(
            json.dumps(sorted(item['Mounts'], key=lambda m: m['Destination']), sort_keys=True).encode()).hexdigest()}
    result['custody'] = {name: digest(ROOT / 'state' / name)
                         for name in [INSTANCE + '.json', 'pilot-policy.sha256']}
    result['agentKey'] = digest(ROOT / 'agent.key')
    return result


def target(run_id, create=False):
    if not re.fullmatch(r'[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}', run_id):
        raise RuntimeError('Invalid renewal run')
    RUNS.mkdir(mode=0o700, exist_ok=True)
    info = RUNS.lstat()
    if RUNS.is_symlink() or info.st_uid != 0 or info.st_mode & 0o077:
        raise RuntimeError('Unsafe guest receipts')
    path = RUNS / run_id
    if create:
        path.mkdir(mode=0o700)
    info = path.lstat()
    if path.is_symlink() or not path.is_dir() or info.st_uid != 0 or info.st_mode & 0o077:
        raise RuntimeError('Unsafe renewal directory')
    return path


def write(path, value):
    with path.open('xb') as file:
        file.write(value)
        file.flush()
        os.fsync(file.fileno())


def pubkey(path, certificate=False):
    return run('openssl', 'x509' if certificate else 'pkey', '-in', path,
               *(['-pubkey', '-noout'] if certificate else ['-pubout']))


def prepare(run_id):
    original = runtime()
    path = target(run_id, True)
    before = path / 'before'
    before.mkdir(mode=0o700)
    for name, source in material().items():
        write(before / name, private(source))
    if pubkey(ROOT / 'agent.crt', True) != pubkey(ROOT / 'agent.key') or pubkey(INGRESS, True) != pubkey(INGRESS):
        raise RuntimeError('Retained guest key mismatch')
    run('openssl', 'req', '-new', '-key', ROOT / 'agent.key', '-subj', '/CN=pds-ass-agent', '-out', path / 'agent.csr')
    # The extracted ingress key stays in the private root-only guest receipt.
    run('openssl', 'pkey', '-in', INGRESS, '-out', path / 'ingress.key')
    run('openssl', 'req', '-x509', '-new', '-key', path / 'ingress.key', '-days', '7',
        '-subj', '/CN=' + INSTANCE + '.localhost',
        '-addext', 'basicConstraints=critical,CA:FALSE', '-addext', 'keyUsage=critical,digitalSignature',
        '-addext', 'extendedKeyUsage=serverAuth', '-addext', 'subjectAltName=DNS:' + INSTANCE + '.localhost',
        '-addext', 'subjectKeyIdentifier=hash', '-addext', 'authorityKeyIdentifier=keyid:always',
        '-out', path / 'ingress.crt')
    write(path / 'ingress.pem', (path / 'ingress.crt').read_bytes() + (path / 'ingress.key').read_bytes())
    if pubkey(path / 'ingress.crt', True) != pubkey(INGRESS):
        raise RuntimeError('Ingress key changed')
    report = {'runtime': original, 'sourceHashes': {name: digest(source) for name, source in material().items()}}
    write(path / 'staging.json', json.dumps(report).encode())
    return {'run': run_id, 'old': run('openssl', 'x509', '-in', INGRESS, '-outform', 'PEM').decode(),
            'new': (path / 'ingress.crt').read_text(), 'csr': (path / 'agent.csr').read_text(),
            'ca': (ROOT / 'ca.crt').read_text(), 'agent': (ROOT / 'agent.crt').read_text()}


def replace(path, value):
    private(path)
    # Retain the inode used by the ingress container's single-file bind mount.
    with path.open('r+b') as file:
        file.write(value)
        file.truncate()
        file.flush()
        os.fsync(file.fileno())


def restart():
    run('docker', 'restart', 'pds-ass-edge-' + INSTANCE)
    run('systemctl', 'restart', 'prichal-public-pds-agent.service')


def validate_candidates(path):
    run('openssl', 'verify', '-x509_strict', '-CAfile', path / 'ca.crt', '-purpose', 'sslserver',
        '-verify_ip', '127.0.0.1', path / 'agent.crt')
    run('openssl', 'verify', '-x509_strict', '-CAfile', path / 'ingress.crt', '-purpose', 'sslserver',
        '-verify_hostname', INSTANCE + '.localhost', path / 'ingress.crt')
    for role, live in [('ca', ROOT / 'ca.crt'), ('agent', ROOT / 'agent.crt'), ('ingress', INGRESS)]:
        if pubkey(path / (role + '.crt'), True) != pubkey(live, True):
            raise RuntimeError('Candidate changed retained key')
    for role, expected in [('ca', 'prichal-kvant-public-pds-ca'), ('agent', 'pds-ass-agent')]:
        if run('openssl', 'x509', '-in', path / (role + '.crt'), '-subject', '-nameopt', 'RFC2253', '-noout').decode().strip() != 'subject=CN=' + expected:
            raise RuntimeError('Wrong certificate identity')
    now = dt.datetime.now(dt.timezone.utc).timestamp()
    for role in ['agent', 'ingress']:
        if not 6 * 86400 < expiry(path / (role + '.crt')) - now <= 7 * 86400:
            raise RuntimeError('Unexpected leaf lifetime')


def activate(value, fail=False):
    path = target(value['run'])
    report = json.loads(private(path / 'staging.json', 0))
    if runtime() != report['runtime'] or any(digest(source) != report['sourceHashes'][name] for name, source in material().items()):
        raise RuntimeError('Guest state changed after staging')
    for name in ['ca', 'agent']:
        text = value[name]
        if not isinstance(text, str) or len(text) > 16384 or 'PRIVATE KEY' in text:
            raise RuntimeError('Only public transport certificates accepted')
        write(path / (name + '.crt'), text.encode())
    validate_candidates(path)
    before = {name: private(path / 'before' / name, 0) for name in material()}
    candidates = {name: private(path / name, 0) for name in material()}
    try:
        for name, source in material().items():
            replace(source, candidates[name])
        restart()
        if fail or runtime() != report['runtime']:
            raise RuntimeError('Activation qualification failed')
    except BaseException:
        if any(private(source) not in [before[name], candidates[name]] for name, source in material().items()):
            raise RuntimeError('Concurrent guest change; manual inspection required')
        for name, source in material().items():
            replace(source, before[name])
        restart()
        if runtime() != report['runtime']:
            raise RuntimeError('Retained runtime differs after rollback')
        write(path / 'rollback-verified.json', b'{"retained":true}')
        raise
    write(path / 'activation-verified.json', b'{"retained":true}')
    return {'activated': True, 'runtimeRetained': True}


def rollback_verified(path):
    report = json.loads(private(path / 'staging.json', 0))
    return (runtime() == report['runtime']
            and all(private(source) == private(path / 'before' / name, 0)
                    for name, source in material().items()))


def main():
    os.umask(0o077)
    value = json.loads(sys.stdin.buffer.read(65537))
    action = value.get('action')
    fields = {'check': {'action'}, 'prepare': {'action', 'run'}, 'activate': {'action', 'run', 'ca', 'agent'},
              'qualify-rollback': {'action', 'run', 'ca', 'agent'}, 'rollback-state': {'action', 'run'}}
    if action not in fields or set(value) != fields[action] or os.geteuid() != 0:
        raise RuntimeError('Unsupported guest operator request')
    if action == 'check':
        result = {'expiry': {name: expiry(path) for name, path in material().items()}, 'runtime': runtime()}
    elif action == 'prepare':
        result = prepare(value['run'])
    elif action in ['activate', 'qualify-rollback']:
        result = activate(value, action == 'qualify-rollback')
    else:
        path = target(value['run'])
        result = {'rollbackVerified': rollback_verified(path)}
    print(json.dumps(result))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Fixed guest certificate operation failed; inspect private receipts.', file=sys.stderr)
        sys.exit(1)
