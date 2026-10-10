#!/usr/bin/env python3
"""Root-owned Almaz helper: change only the disposable pilot's public trust file."""
import hashlib
import datetime
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time

INSTANCE = 'e8e82a65-7aa8-4e37-9306-0de701bf1b19'
KEY_HASH = '284f770cb482ec7400981d1507daa5bad427e4d8d45cf90a84d63825d7b49b66'
TARGET = Path('/opt/server/management/config/caddy/certs/prichal-pilot-ingress.crt')
RECEIPTS = Path('/var/lib/prichal-pilot-trust')


def run(*args):
    result = subprocess.run(args, capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError('Pilot public trust command failed')
    return result.stdout


def certificates(value):
    if not isinstance(value, str) or len(value) > 16384 or 'PRIVATE KEY' in value:
        raise RuntimeError('Public certificates only')
    blocks = re.findall(r'-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----', value)
    if not 1 <= len(blocks) <= 2 or re.sub(r'-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----', '', value).strip():
        raise RuntimeError('Unexpected public trust envelope')
    return blocks


def validate(value, current=False):
    blocks = certificates(value)
    with tempfile.TemporaryDirectory(prefix='prichal-public-cert-') as directory:
        path = Path(directory) / 'public.crt'
        for block in blocks:
            path.write_text(block + '\n')
            key = run('openssl', 'x509', '-in', str(path), '-pubkey', '-noout')
            result = subprocess.run(['openssl', 'pkey', '-pubin', '-outform', 'DER'], input=key, capture_output=True)
            if result.returncode or hashlib.sha256(result.stdout).hexdigest() != KEY_HASH:
                raise RuntimeError('Pilot public key differs')
            expected = 'subject=CN=' + INSTANCE + '.localhost'
            if run('openssl', 'x509', '-in', str(path), '-noout', '-subject').decode().strip() != expected:
                raise RuntimeError('Pilot subject differs')
            if current:
                run('openssl', 'verify', '-CAfile', str(path), '-purpose', 'sslserver', '-verify_hostname', INSTANCE + '.localhost', str(path))
                run('openssl', 'x509', '-in', str(path), '-noout', '-checkend', str(6 * 86400))
                date = run('openssl', 'x509', '-in', str(path), '-noout', '-enddate').decode().strip().split('=', 1)[1]
                expires = datetime.datetime.strptime(date, '%b %d %H:%M:%S %Y %Z').replace(tzinfo=datetime.timezone.utc).timestamp()
                if expires - time.time() > 8 * 86400:
                    raise RuntimeError('Pilot ingress lifetime exceeds policy')
    return blocks


def replace(value):
    fd, temporary = tempfile.mkstemp(prefix='.prichal-pilot-trust-', dir=TARGET.parent)
    try:
        with os.fdopen(fd, 'wb') as file:
            file.write(value)
            file.flush()
            os.fsync(file.fileno())
        os.chmod(temporary, 0o644)
        os.replace(temporary, TARGET)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def reload(receipt):
    for command in ['validate', 'reload']:
        args = ['docker', 'exec', 'caddy', 'caddy', command]
        if command == 'reload':
            args.append('--force')
        result = subprocess.run(args + ['--config', '/etc/caddy/Caddyfile'], capture_output=True, timeout=60)
        (receipt / (command + '.log')).write_bytes(result.stdout + result.stderr)
        if result.returncode:
            raise RuntimeError('Caddy full configuration validation/reload failed')


def main():
    os.umask(0o077)
    value = json.loads(sys.stdin.buffer.read(32769))
    if len(json.dumps(value)) > 32768 or value.get('action') not in ['prepare', 'rollback'] or not re.fullmatch(r'[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}', value.get('run', '')):
        raise RuntimeError('Invalid public trust request')
    if set(value) != ({'action', 'run', 'old', 'new'} if value['action'] == 'prepare' else {'action', 'run'}):
        raise RuntimeError('Unexpected public trust fields')
    info = TARGET.lstat()
    if TARGET.is_symlink() or not TARGET.is_file() or info.st_uid != 0 or info.st_mode & 0o022:
        raise RuntimeError('Unsafe installed public trust')
    old = TARGET.read_bytes()
    validate(old.decode())
    RECEIPTS.mkdir(mode=0o700, exist_ok=True)
    if RECEIPTS.is_symlink() or RECEIPTS.stat().st_uid != 0 or RECEIPTS.stat().st_mode & 0o077:
        raise RuntimeError('Unsafe public trust receipts')
    receipt = RECEIPTS / value['run']
    if value['action'] == 'prepare':
        old_blocks = validate(value['old'])
        new_blocks = validate(value['new'], True)
        if len(old_blocks) != 1 or len(new_blocks) != 1 or old_blocks[0] not in certificates(old.decode()):
            raise RuntimeError('Active preceding certificate is not retained in installed trust')
        receipt.mkdir(mode=0o700)
        (receipt / 'before.crt').write_bytes(old)
        new = ('\n'.join(old_blocks + new_blocks) + '\n').encode()
        (receipt / 'receipt.json').write_text(json.dumps({'beforeHash': hashlib.sha256(old).hexdigest(), 'installedHash': hashlib.sha256(new).hexdigest(), 'createdAt': time.time()}))
        if TARGET.read_bytes() != old:
            raise RuntimeError('Installed public trust changed during preparation')
        try:
            replace(new)
            reload(receipt)
        except BaseException:
            if TARGET.read_bytes() != new:
                raise RuntimeError('Installed public trust changed; preserve manual recovery evidence')
            replace(old)
            reload(receipt)
            raise
    else:
        if receipt.is_symlink() or receipt.stat().st_uid != 0 or receipt.stat().st_mode & 0o077:
            raise RuntimeError('Unsafe rollback receipt')
        report = json.loads((receipt / 'receipt.json').read_text())
        preceding = (receipt / 'before.crt').read_bytes()
        if hashlib.sha256(old).hexdigest() != report['installedHash'] or hashlib.sha256(preceding).hexdigest() != report['beforeHash']:
            raise RuntimeError('Public trust changed after activation')
        if TARGET.read_bytes() != old:
            raise RuntimeError('Installed public trust changed during rollback')
        replace(preceding)
        reload(receipt)
        (receipt / 'rollback-verified.json').write_text('{"reloaded":true}')
    print(json.dumps({'publicTrustReady': True, 'run': value['run']}))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Bounded pilot trust operation failed; inspect root-only receipts.', file=sys.stderr)
        sys.exit(1)
