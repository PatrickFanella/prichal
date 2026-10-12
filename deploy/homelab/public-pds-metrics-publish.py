#!/usr/bin/env python3
"""Read bounded renewal receipts and publish status using a restricted SSH identity."""
import fcntl
import json
import math
import os
from pathlib import Path
import subprocess
import tempfile
import time
import sys

ROOT = Path.home() / '.local/state/prichal-kvant-public-pds-metrics'
RECEIPT = Path.home() / '.local/state/prichal-kvant-public-pds/last-certificates.json'
INSTANCE = '9f148c55-da61-486b-97f4-50df26b65893'
ROLE_MAP = {'ca.crt': 'ca', 'agent.crt': 'agent', 'worker.crt': 'worker', 'ingress.pem': 'ingress'}


def private(path):
    info = path.lstat()
    if path.is_symlink() or not path.is_file() or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Owned private publisher material required')


def load(path):
    private(path)
    if path.stat().st_size > 8192:
        raise ValueError('Receipt too large')
    value = json.loads(path.read_text())
    if not isinstance(value, dict):
        raise ValueError('Receipt object required')
    return value


def timestamp(value, now):
    if type(value) not in [int, float] or not math.isfinite(value) or not 0 <= value <= now + 120:
        raise ValueError('Invalid receipt timestamp')
    return value


def snapshot(current, previous, now):
    state = {'lastSuccess': timestamp(previous.get('lastSuccess', 0), now),
             'lastAttempt': 0, 'success': False, 'inspectionOK': False,
             'expiry': previous.get('expiry', {role: 0 for role in ROLE_MAP.values()})}
    state['lastAttempt'] = timestamp(current['checkedAt'], now)
    if state['lastAttempt'] < previous.get('lastAttempt', 0):
        raise ValueError('Operator receipt went backwards')
    if type(current.get('success')) is not bool:
        raise ValueError('Invalid operator result')
    state['success'] = current['success']
    if state['success']:
        state['lastSuccess'] = max(state['lastSuccess'], state['lastAttempt'])
    dates = current.get('expiry')
    if isinstance(dates, dict) and set(dates) == set(ROLE_MAP):
        if any(type(v) not in [int, float] or not math.isfinite(v) or not 0 < v <= now + 366 * 86400 for v in dates.values()):
            raise ValueError('Invalid certificate expiry')
        state['expiry'] = {ROLE_MAP[name]: value for name, value in dates.items()}
        state['inspectionOK'] = True
    return dict(state, instanceId=INSTANCE, observedAt=now)


def main():
    os.umask(0o077)
    if sys.argv[1:]:
        raise ValueError('Publisher has no runtime/path overrides')
    info = ROOT.lstat()
    if ROOT.is_symlink() or not ROOT.is_dir() or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Private publisher directory required')
    lock_path = ROOT / 'run.lock'
    if lock_path.exists() or lock_path.is_symlink():
        private(lock_path)
    with lock_path.open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        state = ROOT / 'state.json'
        previous = load(state) if state.exists() else {}
        now = time.time()
        try:
            value = snapshot(load(RECEIPT), previous, now)
        except Exception:
            # A corrupt/unavailable receipt must publish failure while retaining
            # last known expiry/success; otherwise delivery freshness hides it.
            value = {'instanceId': INSTANCE, 'observedAt': now, 'lastAttempt': previous.get('lastAttempt', 0),
                     'lastSuccess': previous.get('lastSuccess', 0), 'success': False, 'inspectionOK': False,
                     'expiry': previous.get('expiry', {role: 0 for role in ROLE_MAP.values()})}
        for name in ['publisher.key', 'known_hosts']:
            private(ROOT / name)
        result = subprocess.run(['ssh', '-T', '-i', str(ROOT / 'publisher.key'), '-o', 'IdentitiesOnly=yes',
            '-o', 'UserKnownHostsFile=' + str(ROOT / 'known_hosts'), '-o', 'StrictHostKeyChecking=yes',
            '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no', '-o', 'ControlPath=none',
            'onnwee@10.0.0.57'], input=json.dumps(value).encode(), capture_output=True, timeout=30)
        if result.returncode or json.loads(result.stdout) != {'published': True}:
            raise RuntimeError('Restricted status publication failed')
        fd, candidate = tempfile.mkstemp(dir=ROOT, prefix='.state-')
        try:
            with os.fdopen(fd, 'w') as file:
                json.dump(value, file)
                file.flush()
                os.fsync(file.fileno())
            os.replace(candidate, state)
            fd = os.open(ROOT, os.O_DIRECTORY)
            try:
                os.fsync(fd)
            finally:
                os.close(fd)
        finally:
            Path(candidate).unlink(missing_ok=True)
        print(json.dumps({'published': True, 'inspectionOK': value['inspectionOK'], 'operatorSuccess': value['success']}))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Fixed public pilot status publication failed.', file=sys.stderr)
        sys.exit(1)
