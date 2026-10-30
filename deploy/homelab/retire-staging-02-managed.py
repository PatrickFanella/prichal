#!/usr/bin/env python3
"""Hold reconciliation and remove only the preserved empty staging-02 fixture's containers."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import stat
import subprocess

INSTANCE = '2395105c-9958-47b5-ae8b-5b0424cbfc4f'
HOST = 'pds-' + INSTANCE.replace('-', '') + '.subcult.tv'
AGENT = Path('/var/lib/prichal-agent')
STATE = AGENT / 'state'
ROOT = AGENT / 'managed-retirements'
PRESERVATION = ROOT / 'c3b6c1be-8493-43f4-9a79-ad6f3cbab521'
ARCHIVE_SHA256 = '1b6617c6068fc8dfaa45e0c8cda34f9a09293684e63c89e6c91c41301fe4a56f'
LABEL = 'tv.subcult.pds-ass.instance'
UUID = re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$')
SHA = re.compile(r'^[a-f0-9]{64}$')
NAMES = ['pds-ass-tenant-', 'pds-ass-edge-', 'pds-ass-private-predecessor-', 'pds-ass-private-edge-predecessor-']
IDS = ['a7f519a5cc683fbe64602123ee14b336e864a7c121390a1bd8e29b53622ae9c1',
       'e86b64f782a4a2f58c91cd65fce343f9b142624f945e7284c76922b5f3eee359',
       '9af8dcea60d61f945e1b30b4f96ec00ccdaaf8243b563835474a102998e51ab8',
       '38cefcb1093424173ef074557d500660719107381cbcf12b97a3debe4206058f']
VOLUMES = ['pds-ass-tenant-' + INSTANCE + '-data', 'pds-ass-tenant-' + INSTANCE + '-blobs']
# The agent rejects any later service generation below the maximum, so the hold cannot be superseded.
HOLD_GENERATION = 1 << 53
RETAINED = [STATE / (INSTANCE + '.json'), STATE / (INSTANCE + '-tls.pem'), STATE / (INSTANCE + '-managed-identity.json'),
            AGENT / 'enrolled/node.key', AGENT / 'enrolled/node.crt']


def run(arguments):
    result = subprocess.run(arguments, check=False, capture_output=True, timeout=45)
    if result.returncode:
        raise RuntimeError('Scoped retirement command failed; inspect retained private state')
    return result.stdout


def digest_bytes(data):
    return hashlib.sha256(data).hexdigest()


def digest(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_size > 64 << 30:
        raise ValueError('Regular retained file required')
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def private_json(path, limit=1 << 20):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077 or info.st_size > limit:
        raise ValueError('Private root-owned retirement custody required')
    return json.loads(path.read_bytes())


def write_atomic(path, data):
    # A crash leaves either no file or the complete file; a stale temporary from this request is replaced.
    temporary = path.with_name('.' + path.name + '.partial')
    if os.path.lexists(temporary):
        temporary.unlink()
    with temporary.open('xb') as output:
        output.write(data)
        output.flush()
        os.fchmod(output.fileno(), 0o600)
        os.fsync(output.fileno())
    os.link(temporary, path)
    temporary.unlink()
    sync(path.parent)


def write_once(path, data):
    with path.open('xb') as output:
        output.write(data)
        output.flush()
        os.fchmod(output.fileno(), 0o600)
        os.fsync(output.fileno())
    sync(path.parent)


def sync(path):
    fd = os.open(path, os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def namespace():
    # The designated node hosts only this fixture; any other labeled resource refuses retirement.
    listed = run(['docker', 'ps', '-aq', '--no-trunc', '--filter', 'label=' + LABEL]).split()
    return sorted(item.decode() for item in listed)


def inspect():
    if namespace() != sorted(IDS):
        raise ValueError('Labeled native namespace differs from the preserved fixture')
    values = json.loads(run(['docker', 'inspect'] + [prefix + INSTANCE for prefix in NAMES]))
    if len(values) != 4:
        raise ValueError('Expected four retained fixture containers')
    for index, value in enumerate(values):
        if value['Id'] != IDS[index] or value['Config'].get('Labels', {}).get(LABEL) != INSTANCE or value['HostConfig']['RestartPolicy']['Name'] != 'no':
            raise ValueError('Retained container identity or restart policy differs')
        if index > 1 and value['State']['Running']:
            raise ValueError('Private predecessor is running')
    environment = dict(item.split('=', 1) for item in values[0]['Config']['Env'] if '=' in item)
    if environment.get('PDS_HOSTNAME') != HOST or environment.get('PDS_INVITE_REQUIRED') != 'true':
        raise ValueError('Generated identity and closed native registration required')
    for destination, name in zip(('/pds', '/blobs'), VOLUMES):
        mounts = [mount for mount in values[0]['Mounts'] if mount['Destination'] == destination]
        if len(mounts) != 1 or mounts[0]['Type'] != 'volume' or mounts[0]['Name'] != name:
            raise ValueError('Retained native storage differs')
    return values


def volumes():
    values = json.loads(run(['docker', 'volume', 'inspect'] + VOLUMES))
    if [value['Name'] for value in values] != VOLUMES:
        raise ValueError('Retained native volumes differ')
    return {value['Name']: value['Mountpoint'] for value in values}


def preservation():
    report = private_json(PRESERVATION / 'report.json')
    if report.get('deploymentId') != INSTANCE or report.get('archiveSHA256') != ARCHIVE_SHA256 or report.get('accounts') != 0 or report.get('retired') is not False:
        raise ValueError('Verified preservation custody differs')
    if digest(PRESERVATION / 'native-cold.tar.gz') != ARCHIVE_SHA256:
        raise ValueError('Preserved native archive differs')
    return ARCHIVE_SHA256


def lease_bytes(request_key):
    # Field names follow the agent's flattened service lease; it is read as held before any reconciliation.
    return json.dumps({'instanceId': INSTANCE, 'ticket': request_key, 'generation': HOLD_GENERATION,
                       'state': 'suspended', 'phase': 'applied'}, separators=(',', ':')).encode()


def install_hold(request_key):
    path = STATE / (INSTANCE + '-service.json')
    expected = lease_bytes(request_key)
    owner = STATE.lstat()
    if os.path.lexists(path):
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or path.read_bytes() != expected:
            raise ValueError('An existing service lease differs from this retirement hold')
        return digest_bytes(expected)
    temporary = STATE / ('.retirement-hold-' + request_key)
    if os.path.lexists(temporary):
        temporary.unlink()
    with temporary.open('xb') as output:
        output.write(expected)
        output.flush()
        os.fchmod(output.fileno(), 0o600)
        os.fchown(output.fileno(), owner.st_uid, owner.st_gid)
        os.fsync(output.fileno())
    os.link(temporary, path)
    temporary.unlink()
    sync(STATE)
    return digest_bytes(expected)


def accounts_empty():
    accounts = run(['docker', 'exec', IDS[0], 'node', '--no-warnings', '-e', "const{DatabaseSync}=require('node:sqlite');const d=new DatabaseSync('/pds/account.sqlite',{readOnly:true});console.log(d.prepare('SELECT count(*) n FROM actor').get().n);d.close()"])
    if accounts.strip() != b'0':
        raise ValueError('Fixture contains accounts; retirement refused')


def verify_retired(report):
    if namespace():
        raise ValueError('Retired native namespace still has containers')
    if volumes() != report['retainedVolumes']:
        raise ValueError('Retained native volumes changed')
    if {str(path): digest(path) for path in RETAINED} != report['retainedFiles']:
        raise ValueError('Retained native credentials or node identity changed')
    if digest(STATE / (INSTANCE + '-service.json')) != report['holdSHA256']:
        raise ValueError('Reconciliation hold changed')


def retire(work, request_key, withdrawal):
    if (work / 'native-retirement.json').exists():
        data = (work / 'native-retirement.json').read_bytes()
        report = private_json(work / 'native-retirement.json')
        if report['requestKey'] != request_key or report['edgeWithdrawalSHA256'] != withdrawal:
            raise ValueError('Retirement request differs from its retained receipt')
        verify_retired(report)
        return {'retired': True, 'replayed': True, 'receiptSHA256': digest_bytes(data)}
    archive = preservation()
    if not os.path.lexists(work) and not namespace():
        raise ValueError('Fixture containers are already absent without this request receipt')
    work.mkdir(mode=0o700, exist_ok=True)
    if not (work / 'before.json').exists():
        containers = inspect()
        running = [value['State']['Running'] for value in containers[:2]]
        if running not in ([True, True], [False, False]):
            raise ValueError('Mixed managed native state requires inspection')
        if all(running):
            accounts_empty()
        write_atomic(work / 'before.json', json.dumps({'containers': containers, 'volumes': volumes(),
                                                      'files': {str(path): digest(path) for path in RETAINED}}, indent=2).encode())
    retained = private_json(work / 'before.json', 1 << 22)
    hold = install_hold(request_key)
    # Stop the edge first, then the writer; repeated stops and removals of absent exact IDs are skipped.
    present = set(namespace())
    if not present <= set(IDS):
        raise ValueError('Labeled native namespace differs from the preserved fixture')
    for container in (IDS[1], IDS[0]):
        if container in present:
            run(['docker', 'stop', '--time', '15', container])
    for container in IDS:
        if container in present:
            # No -v: named and anonymous volumes are retained for recovery.
            run(['docker', 'rm', container])
    report = {'version': 1, 'kind': 'managed-native-retirement', 'deploymentId': INSTANCE, 'hostname': HOST, 'requestKey': request_key,
              'edgeWithdrawalSHA256': withdrawal, 'preservationArchiveSHA256': archive, 'removedContainerIds': IDS,
              'retainedVolumes': retained['volumes'], 'retainedFiles': retained['files'], 'holdSHA256': hold,
              'holdGeneration': HOLD_GENERATION, 'retiredAt': dt.datetime.now(dt.timezone.utc).isoformat().replace('+00:00', 'Z')}
    verify_retired(report)
    data = (json.dumps(report, sort_keys=True) + '\n').encode()
    write_once(work / 'native-retirement.json', data)
    return {'retired': True, 'receiptSHA256': digest_bytes(data), 'volumesRetained': True, 'capacityReleased': False}


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--request-key', required=True)
    parser.add_argument('--edge-withdrawal-sha256', required=True)
    parser.add_argument('--confirm-empty-staging-02', action='store_true')
    parser.add_argument('--confirm-remove-native-containers', action='store_true')
    args = parser.parse_args()
    if not (args.confirm_empty_staging_02 and args.confirm_remove_native_containers) or not UUID.fullmatch(args.request_key) or not SHA.fullmatch(args.edge_withdrawal_sha256) or os.geteuid() != 0 or socket.gethostname() != 'prichal-staging-02':
        raise ValueError('Explicit fixed staging-02 root retirement scope required')
    if args.request_key == PRESERVATION.name:
        raise ValueError('Retirement requires its own request key')
    if subprocess.run(['systemctl', 'is-active', '--quiet', 'prichal-agent.service']).returncode != 3:
        raise ValueError('Stop the agent before taking its exclusive state lock')
    info = ROOT.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise ValueError('Private root-owned retirement directory required')
    lock = os.open(STATE / '.native-owner.lock', os.O_RDWR | os.O_NOFOLLOW)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        print(json.dumps(retire(ROOT / args.request_key, args.request_key, args.edge_withdrawal_sha256)))
    finally:
        os.close(lock)


if __name__ == '__main__':
    main()
