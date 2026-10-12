#!/usr/bin/env python3
"""Recover only the retained Kvant public pilot through its authenticated agent."""
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import ssl
import tempfile
import urllib.request

ROOT = Path.home() / '.local/state/prichal-kvant-public-pds'
INSTANCE = '9f148c55-da61-486b-97f4-50df26b65893'
ENDPOINT = 'https://prichal-staging-pds.subcult.tv'
AGENT = 'https://127.0.0.1:22261/v1/instances/' + INSTANCE


def private(path, directory=False):
    info = path.lstat()
    if (path.is_symlink() or info.st_uid != os.getuid() or info.st_mode & 0o077
            or (not path.is_dir() if directory else not path.is_file())):
        raise ValueError('Owned private material required')


def validate(result):
    if (not isinstance(result, dict) or result.get('id') != INSTANCE
            or result.get('endpoint') != ENDPOINT
            or result.get('state') not in ['starting', 'running']
            or type(result.get('healthy')) is not bool):
        raise ValueError('Unexpected retained pilot identity/status')
    return result


def request(context, method):
    req = urllib.request.Request(AGENT, method=method,
        data=b'{}' if method == 'PUT' else None,
        headers={'Content-Type': 'application/json'})
    # No proxy, redirect, hostname override or TLS bypass is allowed here.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
        urllib.request.HTTPSHandler(context=context), NoRedirect())
    with opener.open(req, timeout=15) as response:
        return validate(json.loads(response.read(8193)))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError('Agent redirect refused')


def reconcile():
    private(ROOT, True)
    paused = ROOT / 'reconciliation-paused'
    if paused.exists() or paused.is_symlink():
        private(paused)
        return {'success': True, 'paused': True, 'reconciled': False}
    private(ROOT / 'pki', True)
    for name in ['ca.crt', 'worker.crt', 'worker.key']:
        private(ROOT / 'pki' / name)
    context = ssl.create_default_context(cafile=str(ROOT / 'pki/ca.crt'))
    context.load_cert_chain(str(ROOT / 'pki/worker.crt'), str(ROOT / 'pki/worker.key'))
    # GET must first prove retained runtime custody. A missing namespace must never
    # become a fresh deployment through this unattended recovery path.
    observed = request(context, 'GET')
    changed = observed['state'] != 'running' or not observed['healthy']
    if changed:
        observed = request(context, 'PUT')
    return {'success': observed['healthy'], 'paused': False, 'reconciled': changed,
            'instanceId': INSTANCE, 'state': observed['state'], 'healthy': observed['healthy']}


def record(result):
    private(ROOT, True)
    target = ROOT / 'last-reconciliation.json'
    if target.exists() or target.is_symlink():
        private(target)
    result['checkedAt'] = dt.datetime.now(dt.timezone.utc).isoformat()
    fd, name = tempfile.mkstemp(dir=ROOT, prefix='.reconciliation-')
    try:
        with os.fdopen(fd, 'w') as file:
            json.dump(result, file)
            file.write('\n')
            file.flush()
            os.fsync(file.fileno())
        os.replace(name, target)
        directory = os.open(ROOT, os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        Path(name).unlink(missing_ok=True)
    print(json.dumps(result))


def main():
    os.umask(0o077)
    private(ROOT, True)
    lock_path = ROOT / 'reconciliation.lock'
    if lock_path.exists() or lock_path.is_symlink():
        private(lock_path)
    with lock_path.open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        try:
            result = reconcile()
        except Exception as error:
            result = {'success': False, 'errorType': type(error).__name__}
        record(result)
        return 0 if result['success'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
