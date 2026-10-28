#!/usr/bin/env python3
"""Local root operator for the one authorized empty staging-02 fixture; never an agent HTTP route."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import socket
import shutil
import sqlite3
import stat
import subprocess
import tarfile
import datetime as dt

DEPLOYMENT = '2395105c-9958-47b5-ae8b-5b0424cbfc4f'
NODE = '60684f10-c6b2-4833-a0b8-fb9f96a7869b'
HOST = 'pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv'
STATE = Path('/var/lib/prichal-agent/state')
ROOT = Path('/var/lib/prichal-agent/managed-conversions')
BINARY = Path('/usr/local/libexec/prichal/node-agent')
DROPIN = Path('/etc/systemd/system/prichal-agent.service.d/managed-runtime.conf')
DROPIN_BYTES = b'[Service]\nEnvironment=PDS_STAGING_MANAGED_RUNTIME=true\n'
PDS_IMAGE = 'ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336'
EDGE_IMAGE = 'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d'
NATIVE = 'pds-ass-tenant-' + DEPLOYMENT
EDGE = 'pds-ass-edge-' + DEPLOYMENT
OLD_NATIVE = 'pds-ass-private-predecessor-' + DEPLOYMENT
OLD_EDGE = 'pds-ass-private-edge-predecessor-' + DEPLOYMENT
LABEL = 'tv.subcult.pds-ass.instance'
UUID = re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$')
SHA = re.compile(r'^[a-f0-9]{64}$')


def run(arguments, **kwargs):
    try:
        return subprocess.run(arguments, check=True, capture_output=True, timeout=45, **kwargs)
    except subprocess.CalledProcessError:
        raise RuntimeError('Scoped native command failed; private state is retained') from None


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def private_directory(path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_mode & 0o077:
        raise ValueError('Private regular conversion directory required')


def inspect(name):
    result = subprocess.run(['docker', 'inspect', name], capture_output=True, text=True, timeout=15)
    if result.returncode:
        # Distinguish a missing object from an unavailable daemon before treating a name as fresh.
        run(['docker', 'info', '--format', '{{.DockerRootDir}}'], text=True)
        if any(message in result.stderr.lower() for message in ('no such object', 'no such container', 'no such network')):
            return None
        raise RuntimeError('Native inventory unavailable')
    return json.loads(result.stdout)[0]


def validate_container(value, role, running=None):
    expected_id, image = ('9af8dcea60d6', PDS_IMAGE) if role == 'native' else ('38cefcb10934', EDGE_IMAGE)
    if (not value or not value['Id'].startswith(expected_id) or value['Config']['Image'] != image
            or value['Config'].get('Labels', {}).get(LABEL) != DEPLOYMENT
            or value['HostConfig'].get('Privileged')
            or value['HostConfig'].get('RestartPolicy', {}).get('Name') != 'no'
            or running is not None and value['State']['Running'] != running):
        raise ValueError('Retained staging-02 container identity or state differs')
    if role == 'native':
        env = dict(item.split('=', 1) for item in value['Config']['Env'] if '=' in item)
        if env.get('PDS_DEV_MODE') != 'true' or env.get('PDS_HOSTNAME') != DEPLOYMENT + '.localhost:45109':
            raise ValueError('Only the original private fixture may be converted')
        mounts = {m['Destination']: m for m in value.get('Mounts', [])}
        if set(mounts) != {'/pds', '/blobs'} or any(mounts[d].get('Type') != 'volume' or mounts[d].get('Name') != NATIVE + '-' + suffix or mounts[d].get('RW') is not True for d, suffix in [('/pds', 'data'), ('/blobs', 'blobs')]):
            raise ValueError('Native storage differs from the retained fixture')
    return value


def inventory(path):
    result = {}
    for entry in sorted(path.rglob('*')):
        info = entry.lstat()
        if stat.S_ISDIR(info.st_mode):
            continue
        if not stat.S_ISREG(info.st_mode):
            raise ValueError('Cold fixture contains a link or nonregular file')
        if entry.name == '.native-owner.lock':
            continue
        result[str(entry.relative_to(path))] = [info.st_size, digest(entry)]
        if len(result) > 100000:
            raise ValueError('Cold fixture inventory exceeded its bound')
    return result


def volume(suffix):
    value = json.loads(run(['docker', 'volume', 'inspect', NATIVE + '-' + suffix], text=True).stdout)[0]
    expected = Path('/var/lib/docker/volumes') / (NATIVE + '-' + suffix) / '_data'
    if value.get('Labels', {}).get(LABEL) != DEPLOYMENT or value.get('Driver') != 'local' or value.get('Mountpoint') != str(expected) or expected.is_symlink():
        raise ValueError('Native volume is outside the fixed local fixture')
    return expected


def write_once(path, value):
    data = json.dumps(value, indent=2, sort_keys=True).encode() + b'\n'
    with path.open('xb') as output:
        output.write(data)
        output.flush()
        os.fsync(output.fileno())


def certificate_digest():
    data = run(['openssl', 'x509', '-in', str(STATE / (DEPLOYMENT + '-tls.pem')), '-outform', 'PEM']).stdout
    return hashlib.sha256(data).hexdigest()


def verify_sqlite(path):
    files = list(path.rglob('*.sqlite'))
    if not files:
        raise ValueError('Cold native SQLite inventory is missing')
    for file in files:
        db = sqlite3.connect(file.as_uri() + '?mode=ro', uri=True)
        try:
            if db.execute('PRAGMA integrity_check').fetchall() != [('ok',)]:
                raise ValueError('Restored native SQLite integrity differs')
        finally:
            db.close()
    db = sqlite3.connect((path / 'account.sqlite').as_uri() + '?mode=ro', uri=True)
    try:
        if db.execute('SELECT count(*) FROM actor').fetchone() != (0,):
            raise ValueError('Only a zero-account fixture may be converted')
    finally:
        db.close()


def validate_members(members):
    for member in members:
        parts = Path(member.name).parts
        if not parts or parts[0] not in ('state', 'data', 'blobs') or member.name.startswith('/') or '..' in parts or not (member.isdir() or member.isfile()):
            raise ValueError('Cold archive escaped its fixed regular-file scope')


def prepare(work, args):
    if work.exists():
        private_directory(work)
        if any(work.iterdir()):
            raise ValueError('Retained conversion exists; use its explicit activation or abort step')
    else:
        work.mkdir(mode=0o700)
    native = validate_container(inspect(NATIVE), 'native', True)
    edge = validate_container(inspect(EDGE), 'edge', True)
    if native['State'].get('Health', {}).get('Status') != 'healthy' or inspect(OLD_NATIVE) or inspect(OLD_EDGE):
        raise ValueError('Healthy private fixture and unused predecessor names required')
    accounts = run(['docker', 'exec', native['Id'], 'node', '--no-warnings', '-e', "const{DatabaseSync}=require('node:sqlite');const d=new DatabaseSync('/pds/account.sqlite',{readOnly:true});console.log(d.prepare('SELECT count(*) n FROM actor').get().n);d.close()"], text=True).stdout.strip()
    if accounts != '0':
        raise ValueError('Private fixture has native accounts; conversion refused')
    credentials = digest(STATE / (DEPLOYMENT + '.json'))
    retained_credentials = json.loads((STATE / (DEPLOYMENT + '.json')).read_text())
    environment = dict(item.split('=', 1) for item in native['Config']['Env'] if '=' in item)
    if any(environment.get(setting) != retained_credentials.get(field) for setting, field in [('PDS_JWT_SECRET', 'jwt'), ('PDS_ADMIN_PASSWORD', 'admin'), ('PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX', 'rotation')]):
        raise ValueError('Native runtime and retained credentials differ')
    certificate = certificate_digest()
    run(['docker', 'stop', '--time', '15', edge['Id']])
    run(['docker', 'stop', '--time', '15', native['Id']])
    validate_container(inspect(native['Id']), 'native', False)
    validate_container(inspect(edge['Id']), 'edge', False)
    paths = {'state': STATE, 'data': volume('data'), 'blobs': volume('blobs')}
    before = {name: inventory(path) for name, path in paths.items()}
    total = sum(value[0] for entries in before.values() for value in entries.values())
    if total > 1 << 30 or shutil.disk_usage(work).free < max(2 << 30, total * 4):
        raise ValueError('Disposable cold backup exceeds its retained disk budget')
    archive = work / 'native-cold.tar.gz'
    with archive.open('xb') as destination:
        with tarfile.open(fileobj=destination, mode='w:gz') as output:
            for name, path in paths.items():
                output.add(path, arcname=name, filter=lambda m: None if Path(m.name).name == '.native-owner.lock' else m)
        destination.flush()
        os.fsync(destination.fileno())
    restored = work / 'restored'
    restored.mkdir(mode=0o700)
    with tarfile.open(archive) as source:
        validate_members(source.getmembers())
        source.extractall(restored, filter='data')
    if {name: inventory(restored / name) for name in paths} != before or {name: inventory(path) for name, path in paths.items()} != before:
        raise ValueError('Independent cold restore or stopped source differs')
    verify_sqlite(restored / 'data')
    write_once(work / 'native-inspect.json', native)
    write_once(work / 'edge-inspect.json', edge)
    write_once(work / 'cold-inventory.json', before)
    receipt = {'version': 1, 'deploymentId': DEPLOYMENT, 'nodeId': NODE, 'requestKey': args.key,
               'createdAt': dt.datetime.now(dt.timezone.utc).isoformat(), 'sourceAccounts': 0, 'backupVerified': True,
               'nativeId': native['Id'], 'edgeId': edge['Id'], 'nativeBackupSha256': digest(archive),
               'managementBackupSha256': args.management_backup_sha256, 'credentialsSha256': credentials,
               'certificateSha256': certificate, 'agentSha256': args.agent_sha256}
    write_once(work / 'prepared.json', receipt)
    return receipt


def retained(work, args):
    private_directory(work)
    receipt = json.loads((work / 'prepared.json').read_text())
    if receipt.get('deploymentId') != DEPLOYMENT or receipt.get('nodeId') != NODE or receipt.get('requestKey') != args.key or receipt.get('agentSha256') != args.agent_sha256 or receipt.get('managementBackupSha256') != args.management_backup_sha256 or digest(work / 'native-cold.tar.gz') != receipt['nativeBackupSha256']:
        raise ValueError('Retained conversion custody differs')
    if digest(STATE / (DEPLOYMENT + '.json')) != receipt['credentialsSha256'] or certificate_digest() != receipt['certificateSha256']:
        raise ValueError('Native credentials or ingress identity changed')
    for role, original, predecessor in [('native', NATIVE, OLD_NATIVE), ('edge', EDGE, OLD_EDGE)]:
        current = inspect(original)
        previous = inspect(predecessor)
        if current and previous:
            raise ValueError('New managed resources exist; automatic private abort is forbidden')
        selected = validate_container(current or previous, role, False)
        if selected['Id'] != receipt[role + 'Id']:
            raise ValueError('Stopped predecessor identity changed')
    expected = json.loads((work / 'cold-inventory.json').read_text())
    current_state = inventory(STATE)
    current_state.pop(DEPLOYMENT + '-managed-identity.json', None)
    if current_state != expected['state']:
        raise ValueError('Retained private configuration changed')
    for name, path in [('data', volume('data')), ('blobs', volume('blobs'))]:
        if inventory(path) != expected[name]:
            raise ValueError('Native data changed; automatic conversion/abort is forbidden')
    return receipt


def activate(work, args):
    receipt = retained(work, args)
    for role, original, predecessor in [('native', NATIVE, OLD_NATIVE), ('edge', EDGE, OLD_EDGE)]:
        value = inspect(original)
        if value:
            validate_container(value, role, False)
            run(['docker', 'rename', value['Id'], predecessor])
    identity = STATE / (DEPLOYMENT + '-managed-identity.json')
    data = json.dumps({'version': 1, 'instanceId': DEPLOYMENT, 'hostname': HOST}, separators=(',', ':')).encode() + b'\n'
    if identity.exists():
        if identity.is_symlink() or identity.read_bytes() != data:
            raise ValueError('Managed identity receipt conflict')
    else:
        with identity.open('xb') as target:
            target.write(data)
            target.flush()
            os.fsync(target.fileno())
        agent = pwd.getpwnam('prichal-agent')
        os.chown(identity, agent.pw_uid, agent.pw_gid)
        directory = os.open(STATE, os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    DROPIN.parent.mkdir(mode=0o755, exist_ok=True)
    if DROPIN.exists():
        if DROPIN.is_symlink() or DROPIN.read_bytes() != DROPIN_BYTES:
            raise ValueError('Managed runtime drop-in conflict')
    else:
        with DROPIN.open('xb') as output:
            output.write(DROPIN_BYTES)
            output.flush()
            os.fsync(output.fileno())
            os.fchmod(output.fileno(), 0o644)
    run(['systemctl', 'daemon-reload'])
    result = dict(receipt, nativeReceiptSha256=digest(identity), phase='activated-before-create')
    if not (work / 'activated.json').exists():
        write_once(work / 'activated.json', result)
    return result


def abort_before_create(work, args):
    receipt = retained(work, args)
    activated = json.loads((work / 'activated.json').read_text())
    identity = STATE / (DEPLOYMENT + '-managed-identity.json')
    if identity.is_symlink() or digest(identity) != activated['nativeReceiptSha256'] or DROPIN.is_symlink() or DROPIN.read_bytes() != DROPIN_BYTES:
        raise ValueError('Abort custody changed')
    if (STATE / (DEPLOYMENT + '-managed-edge')).exists() or inspect('pds-ass-managed-tenant-' + DEPLOYMENT + '-network'):
        raise ValueError('Managed resources were created; use reviewed cold recovery instead')
    identity.unlink()
    DROPIN.unlink()
    for role, original, predecessor in [('native', NATIVE, OLD_NATIVE), ('edge', EDGE, OLD_EDGE)]:
        value = validate_container(inspect(predecessor), role, False)
        if inspect(original):
            raise ValueError('Original resource name is occupied')
        run(['docker', 'rename', value['Id'], original])
    run(['systemctl', 'daemon-reload'])
    return dict(receipt, phase='aborted-before-create')


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare', 'activate', 'abort-before-create'])
    parser.add_argument('--key', required=True)
    parser.add_argument('--management-backup-sha256', required=True)
    parser.add_argument('--agent-sha256', required=True)
    parser.add_argument('--confirm-empty-staging-02', action='store_true')
    args = parser.parse_args()
    if not args.confirm_empty_staging_02 or not UUID.fullmatch(args.key) or not SHA.fullmatch(args.agent_sha256) or not SHA.fullmatch(args.management_backup_sha256):
        raise ValueError('Explicit fixed-scope conversion and custody digests required')
    if os.geteuid() != 0 or socket.gethostname() != 'prichal-staging-02' or digest(BINARY) != args.agent_sha256:
        raise ValueError('Only the verified second staging guest is authorized')
    private_directory(STATE)
    if (run(['systemctl', 'show', 'prichal-agent.service', '--property=MainPID', '--value'], text=True).stdout.strip() != '0'
            or run(['systemctl', 'show', 'prichal-agent.service', '--property=ActiveState', '--value'], text=True).stdout.strip() != 'inactive'):
        raise ValueError('Stop the dedicated guest agent before local conversion')
    lock = os.open(STATE / '.native-owner.lock', os.O_RDWR | os.O_NOFOLLOW)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        ROOT.mkdir(mode=0o700, exist_ok=True)
        private_directory(ROOT)
        work = ROOT / args.key
        result = {'prepare': prepare, 'activate': activate, 'abort-before-create': abort_before_create}[args.action](work, args)
        print(json.dumps({k: v for k, v in result.items() if k not in ('credentialsSha256',)}))
    finally:
        os.close(lock)


if __name__ == '__main__':
    main()
