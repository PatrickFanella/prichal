#!/usr/bin/env python3
"""Kvant host operator; SSH transports only fixed commands and public certificates."""
import datetime
import fcntl
import json
import os
from pathlib import Path
import secrets
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = Path('/home/onnwee/.local/state/prichal-pilot-certificates')
HOST = '/srv/apps/prichal/pilot/certificate-tools/certificate-host.py'
TRUST = '/usr/local/libexec/prichal-pilot-trust.py'
RENEW_BEFORE = 3 * 86400


def ssh(host, args, body=None):
    command = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no', '-o', 'ControlPath=none', host, shlex.join(args)]
    result = subprocess.run(command, input=None if body is None else json.dumps(body).encode(), capture_output=True, timeout=90)
    if result.returncode:
        raise RuntimeError('Fixed host operation failed')
    return json.loads(result.stdout)


def dozor(*args):
    return ssh('dozor', ['/usr/bin/python', HOST, *args])


def trust(body):
    return ssh('almaz', ['sudo', '-n', '/usr/bin/python3', TRUST], body)


def public_check():
    headers = {'User-Agent': 'prichal-pilot-qualification/1.0', 'Content-Type': 'application/json'}
    for host, version in [('prichal-pds.subcult.tv', '0.4.5036')]:
        request = urllib.request.Request('https://' + host + '/xrpc/_health', headers=headers)
        with urllib.request.urlopen(request, timeout=20) as response:
            if response.status != 200 or json.loads(response.read()).get('version') != version:
                raise RuntimeError('Public PDS health differs')
    for path in ['/xrpc/com.atproto.server.createAccount', '/@atproto/oauth-provider/~api/sign-up']:
        request = urllib.request.Request('https://prichal-pds.subcult.tv' + path, data=b'{}', headers=headers, method='POST')
        try:
            urllib.request.urlopen(request, timeout=20).close()
            raise RuntimeError('Pilot signup unexpectedly accepted')
        except urllib.error.HTTPError as error:
            if error.code != 403 or error.headers.get('Cf-Mitigated') or error.read(1024) != b'':
                raise RuntimeError('Pilot signup closure differs') from None


def main():
    os.umask(0o077)
    if sys.argv[1:] not in [[], ['--qualify-renewal'], ['--qualify-rollback']]:
        raise RuntimeError('Unsupported operator option')
    ROOT.mkdir(mode=0o700, exist_ok=True)
    if ROOT.is_symlink() or ROOT.stat().st_uid != os.geteuid() or ROOT.stat().st_mode & 0o077:
        raise RuntimeError('Unsafe operator root')
    with (ROOT / 'run.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        run_id = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(4)
        report = {'run': run_id, 'renewed': False, 'success': False, 'startedAt': time.time()}
        prepared = False
        activated = False
        try:
            current = dozor('check')
            if min(current['expiry'].values()) - time.time() <= RENEW_BEFORE or sys.argv[1:]:
                candidate = dozor('prepare', run_id)
                trust({'action': 'prepare', **candidate})
                prepared = True
                dozor('qualify-rollback' if sys.argv[1:] == ['--qualify-rollback'] else 'activate', run_id)
                activated = True
                report['renewed'] = True
            # Public failure after successful local activation must not roll back
            # trust independently: the renewed writer needs the new certificate.
            public_check()
            report['expiry'] = dozor('check')['expiry']
            dozor('record', 'success')
            report['success'] = True
        except BaseException:
            if prepared and not activated:
                # Host activation verifies rollback before returning failure.
                # If transport was lost, inspect host state before trust rollback.
                try:
                    dozor('check')
                    with_receipt = dozor('rollback-state', run_id)
                    if with_receipt.get('rollbackVerified'):
                        trust({'action': 'rollback', 'run': run_id})
                        report['rollbackVerified'] = True
                    else:
                        report['manualInspectionRequired'] = True
                except Exception:
                    report['manualInspectionRequired'] = True
            try:
                dozor('record', 'failure')
            except Exception:
                pass  # The previous heartbeat becomes stale and raises an alert.
            raise
        finally:
            report['completedAt'] = time.time()
            path = ROOT / (run_id + '.json')
            path.write_text(json.dumps(report, indent=2))
            path.chmod(0o600)
        print(json.dumps(report))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('Pilot certificate coordination failed; inspect private host receipts.', file=sys.stderr)
        sys.exit(1)
