#!/usr/bin/env python3
"""Read the staging-02 renewal status and publish bounded expiry/operator state to Dozor."""
import datetime as dt
import fcntl
import json
import math
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path.home() / '.local/state/prichal-staging-02-metrics'
RECEIPT = Path.home() / '.local/state/prichal-staging-02-bootstrap/transport-renewal/status.json'
NODE = '60684f10-c6b2-4833-a0b8-fb9f96a7869b'
ROLES = ('node', 'worker', 'ca')


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


def seconds(text):
    if not isinstance(text, str):
        raise ValueError('Timestamp text required')
    value = dt.datetime.fromisoformat(text)
    if value.tzinfo is None:
        raise ValueError('Timezone-qualified timestamp required')
    return value.timestamp()


def timestamp(value, now):
    if type(value) not in [int, float] or not math.isfinite(value) or not 0 <= value <= now + 120:
        raise ValueError('Invalid receipt timestamp')
    return value


def snapshot(current, previous, now):
    if current.get('nodeId') != NODE or type(current.get('success')) is not bool:
        raise ValueError('Fixed staging-02 operator result required')
    attempt = timestamp(seconds(current['checkedAt']), now)
    if attempt < previous.get('lastAttempt', 0):
        raise ValueError('Operator receipt went backwards')
    state = {'nodeId': NODE, 'observedAt': now, 'lastAttempt': attempt, 'success': current['success'],
             'lastSuccess': timestamp(previous.get('lastSuccess', 0), now),
             'expiry': previous.get('expiry', {role: 0 for role in ROLES})}
    if state['success']:
        # Only a successful run reports current dates; a failed one keeps the last known expiries.
        expiry = {role: seconds(current[role + 'Expiry']) for role in ROLES}
        if any(not 0 < value <= now + 366 * 86400 for value in expiry.values()):
            raise ValueError('Invalid certificate expiry')
        state['expiry'] = expiry
        state['lastSuccess'] = max(state['lastSuccess'], attempt)
    state['inspectionOK'] = all(state['expiry'][role] > 0 for role in ROLES)
    return state


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
            value = {'nodeId': NODE, 'observedAt': now, 'lastAttempt': previous.get('lastAttempt', 0),
                     'lastSuccess': previous.get('lastSuccess', 0), 'success': False, 'inspectionOK': False,
                     'expiry': previous.get('expiry', {role: 0 for role in ROLES})}
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
        print('Staging-02 status publication failed.', file=sys.stderr)
        sys.exit(1)
