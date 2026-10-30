#!/usr/bin/env python3
"""Preserve the fixed empty managed fixture before its separately authorized retirement."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import sqlite3
import stat
import subprocess
import tarfile

INSTANCE = '2395105c-9958-47b5-ae8b-5b0424cbfc4f'
HOST = 'pds-' + INSTANCE.replace('-', '') + '.subcult.tv'
STATE = Path('/var/lib/prichal-agent/state')
ROOT = Path('/var/lib/prichal-agent/managed-retirements')
UUID = re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$')
NAMES = ['pds-ass-tenant-', 'pds-ass-edge-', 'pds-ass-private-predecessor-', 'pds-ass-private-edge-predecessor-']
IDS = ['a7f519a5cc683fbe64602123ee14b336e864a7c121390a1bd8e29b53622ae9c1',
       'e86b64f782a4a2f58c91cd65fce343f9b142624f945e7284c76922b5f3eee359',
       '9af8dcea60d61f945e1b30b4f96ec00ccdaaf8243b563835474a102998e51ab8',
       '38cefcb1093424173ef074557d500660719107381cbcf12b97a3debe4206058f']


def run(arguments):
    result = subprocess.run(arguments, check=False, capture_output=True, timeout=45)
    if result.returncode:
        raise RuntimeError('Scoped preservation command failed; inspect retained private state')
    return result.stdout


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def inventory(root):
    result = {}
    for path in sorted(root.rglob('*')):
        if path == STATE / '.native-owner.lock':
            continue
        mode = path.lstat().st_mode
        if stat.S_ISREG(mode):
            result[str(path.relative_to(root))] = [path.stat().st_size, digest(path)]
        elif not stat.S_ISDIR(mode):
            raise ValueError('Backup contains a symlink or special file')
    if len(result) > 100000:
        raise ValueError('Backup exceeds its file bound')
    return result


def verify_sqlite(root):
    databases = list(root.rglob('*.sqlite'))
    if not databases or not (root / 'account.sqlite').is_file():
        raise ValueError('Native SQLite inventory is missing')
    for path in databases:
        connection = sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)
        try:
            if connection.execute('PRAGMA integrity_check').fetchall() != [('ok',)]:
                raise ValueError('Restored SQLite integrity failed')
            if path.name == 'account.sqlite' and connection.execute('SELECT count(*) FROM actor').fetchone() != (0,):
                raise ValueError('Fixture contains accounts; retirement refused')
        finally:
            connection.close()


def validate_members(members):
    for member in members:
        parts = Path(member.name).parts
        if not parts or parts[0] not in ('state', 'data', 'blobs', 'node') or member.name.startswith('/') or '..' in parts or not (member.isfile() or member.isdir()):
            raise ValueError('Archive member escaped the regular-file backup scope')


def inspect():
    values = json.loads(run(['docker', 'inspect'] + [prefix + INSTANCE for prefix in NAMES]))
    if len(values) != 4:
        raise ValueError('Expected four retained fixture containers')
    for index, value in enumerate(values):
        if value['Id'] != IDS[index] or value['Config'].get('Labels', {}).get('tv.subcult.pds-ass.instance') != INSTANCE or value['HostConfig']['RestartPolicy']['Name'] != 'no':
            raise ValueError('Retained container identity or restart policy differs')
        if index > 1 and value['State']['Running']:
            raise ValueError('Private predecessor is running')
    environment = dict(item.split('=', 1) for item in values[0]['Config']['Env'] if '=' in item)
    if environment.get('PDS_HOSTNAME') != HOST or environment.get('PDS_INVITE_REQUIRED') != 'true':
        raise ValueError('Generated identity and closed native registration required')
    return values


def cold_copy(work, roots):
    source = {name: inventory(root) for name, root in roots.items()}
    total = sum(value[0] for files in source.values() for value in files.values())
    if total > 1 << 30 or shutil.disk_usage(work).free < max(2 << 30, total * 4):
        raise ValueError('Cold preservation exceeds its disk budget')
    archive = work / 'native-cold.tar.gz'
    with archive.open('xb') as output:
        with tarfile.open(fileobj=output, mode='w:gz') as tar:
            for name, root in roots.items():
                tar.add(root, arcname=name, filter=lambda member: None if member.name == 'state/.native-owner.lock' else member)
        output.flush()
        os.fsync(output.fileno())
    archive.chmod(0o600)
    restored = work / 'restored'
    restored.mkdir(mode=0o700)
    with tarfile.open(archive) as tar:
        validate_members(tar.getmembers())
        tar.extractall(restored, filter='data')
    # SQLite can create/remove WAL/SHM files even during a read-only integrity check.
    # Keep the byte-comparison restore untouched and inspect a separate disposable copy.
    verification = work / 'sqlite-verification'
    shutil.copytree(restored / 'data', verification)
    verify_sqlite(verification)
    for name, root in roots.items():
        if inventory(root) != source[name] or inventory(restored / name) != source[name]:
            raise ValueError('Native source or restored file inventory differs')
    return {'archiveSHA256': digest(archive), 'filesVerified': sum(map(len, source.values())), 'accounts': 0, 'sqliteIntegrity': 'ok', 'sourceUnchanged': True}


def preserve(work, allow_stopped=False):
    before = inspect()
    running = [value['State']['Running'] for value in before[:2]]
    if running != [True, True] and not (allow_stopped and running == [False, False]):
        raise ValueError('Expected running fixture or explicitly confirmed retained cold fixture')
    if all(running):
        accounts = run(['docker', 'exec', IDS[0], 'node', '--no-warnings', '-e', "const{DatabaseSync}=require('node:sqlite');const d=new DatabaseSync('/pds/account.sqlite',{readOnly:true});console.log(d.prepare('SELECT count(*) n FROM actor').get().n);d.close()"])
        if accounts.strip() != b'0':
            raise ValueError('Fixture contains accounts; preservation stop refused')
    roots = {'state': STATE, 'node': Path('/var/lib/prichal-agent/enrolled')}
    for destination, suffix in [('/pds', 'data'), ('/blobs', 'blobs')]:
        mounts = [mount for mount in before[0]['Mounts'] if mount['Destination'] == destination]
        expected = 'pds-ass-tenant-' + INSTANCE + '-' + suffix
        if len(mounts) != 1 or mounts[0]['Type'] != 'volume' or mounts[0]['Name'] != expected or mounts[0]['Source'] != '/var/lib/docker/volumes/' + expected + '/_data':
            raise ValueError('Retained native storage differs')
        roots[suffix] = Path(mounts[0]['Source'])
    if any(root.is_symlink() or not root.is_dir() for root in roots.values()):
        raise ValueError('Regular source directories required')
    work.mkdir(mode=0o700)
    (work / 'containers-before.json').write_text(json.dumps(before, indent=2) + '\n')
    shutil.copyfile('/var/lib/prichal-agent/ca.crt', work / 'agent-ca.crt')
    shutil.copyfile('/usr/local/libexec/prichal/node-agent', work / 'node-agent')
    (work / 'agent-unit.txt').write_bytes(run(['systemctl', 'cat', 'prichal-agent.service']))
    if all(running):
        for container in (IDS[1], IDS[0]):
            run(['docker', 'stop', '--time', '15', container])
    if any(value['State']['Running'] for value in inspect()):
        raise ValueError('Cold backup requires every fixture container stopped')
    report = cold_copy(work, roots)
    if any(value['State']['Running'] for value in inspect()):
        raise ValueError('A native writer appeared during preservation')
    report.update(version=1, deploymentId=INSTANCE, containersStopped=True, retired=False)
    (work / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--request-key', required=True)
    parser.add_argument('--confirm-empty-staging-02', action='store_true')
    parser.add_argument('--confirm-stopped-fixture', action='store_true')
    args = parser.parse_args()
    if not args.confirm_empty_staging_02 or not UUID.fullmatch(args.request_key) or os.geteuid() != 0 or socket.gethostname() != 'prichal-staging-02':
        raise ValueError('Explicit fixed staging-02 root preservation scope required')
    if subprocess.run(['systemctl', 'is-active', '--quiet', 'prichal-agent.service']).returncode != 3:
        raise ValueError('Stop the agent before taking its exclusive state lock')
    ROOT.mkdir(mode=0o700, exist_ok=True)
    info = ROOT.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise ValueError('Private root-owned preservation directory required')
    lock = os.open(STATE / '.native-owner.lock', os.O_RDWR | os.O_NOFOLLOW)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        print(json.dumps(preserve(ROOT / args.request_key, args.confirm_stopped_fixture)))
    finally:
        os.close(lock)


if __name__ == '__main__':
    main()
