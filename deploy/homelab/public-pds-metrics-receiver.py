#!/usr/bin/env python3
"""Restricted SSH receiver: publish one fixed pilot status file, with no host control."""
import json
import math
import os
from pathlib import Path
import sys
import tempfile
import time

INSTANCE = '9f148c55-da61-486b-97f4-50df26b65893'
ROOT = Path('/var/lib/prometheus/node-exporter/prichal-kvant-public-pds')
TARGET = ROOT / 'certificates.prom'
ROLES = {'ca', 'agent', 'worker', 'ingress'}
PREFIX = 'prichal_kvant_public_pds_certificate_'


def number(value):
    if type(value) not in [int, float] or not math.isfinite(value) or value < 0:
        raise ValueError('Finite nonnegative timestamp required')
    return value


def validate(value, now):
    expected = {'instanceId', 'observedAt', 'lastAttempt', 'lastSuccess', 'success', 'inspectionOK', 'expiry'}
    if not isinstance(value, dict) or set(value) != expected or value['instanceId'] != INSTANCE:
        raise ValueError('Fixed pilot status envelope required')
    if type(value['success']) is not bool or type(value['inspectionOK']) is not bool:
        raise ValueError('Boolean status required')
    observed = number(value['observedAt'])
    if not now - 900 <= observed <= now + 120:
        raise ValueError('Source observation is stale or in the future')
    if not isinstance(value['expiry'], dict) or set(value['expiry']) != ROLES:
        raise ValueError('Fixed certificate roles required')
    for expiry in value['expiry'].values():
        if number(expiry) > now + 366 * 86400:
            raise ValueError('Certificate expiry outside policy')
    for name in ['lastAttempt', 'lastSuccess']:
        if number(value[name]) > observed + 120:
            raise ValueError('Operator timestamp in future')
    if value['lastSuccess'] > value['lastAttempt']:
        raise ValueError('Success cannot follow latest attempt')
    return value


def render(value, now):
    validate(value, now)
    lines = [f'{PREFIX}not_after_seconds{{role="{role}"}} {value["expiry"][role]}' for role in sorted(ROLES)]
    for metric, source in [('operator_attempt_seconds', 'lastAttempt'), ('operator_last_success_seconds', 'lastSuccess'),
                           ('source_observed_seconds', 'observedAt')]:
        lines.append(PREFIX + metric + ' ' + str(value[source]))
    lines.extend([PREFIX + 'inspection_ok ' + str(int(value['inspectionOK'])),
                  PREFIX + 'operator_success ' + str(int(value['success'])),
                  PREFIX + 'received_seconds ' + str(now)])
    return '\n'.join(lines) + '\n'


def publish(data):
    info = ROOT.lstat()
    if ROOT.is_symlink() or not ROOT.is_dir() or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise ValueError('Owned receiver directory required')
    if TARGET.exists() or TARGET.is_symlink():
        info = TARGET.lstat()
        if TARGET.is_symlink() or not TARGET.is_file() or info.st_uid != os.getuid() or info.st_mode & 0o022:
            raise ValueError('Unsafe installed metrics')
    fd, name = tempfile.mkstemp(dir=ROOT, prefix='.metrics-')
    try:
        with os.fdopen(fd, 'w') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        os.chmod(name, 0o644)
        os.replace(name, TARGET)
        fd = os.open(ROOT, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        Path(name).unlink(missing_ok=True)


def main():
    os.umask(0o077)
    if sys.argv[1:] or os.environ.get('SSH_ORIGINAL_COMMAND'):
        raise ValueError('Receiver has no commands or path arguments')
    raw = sys.stdin.buffer.read(4097)
    if len(raw) > 4096:
        raise ValueError('Status envelope exceeds limit')
    now = time.time()
    publish(render(json.loads(raw), now))
    print('{"published":true}')


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Fixed public pilot metrics rejected.', file=sys.stderr)
        sys.exit(1)
