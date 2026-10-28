#!/usr/bin/env python3
"""Root-only Almaz publisher for the single authorized staging-02 endpoint, never a worker route."""
import argparse
import datetime as dt
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import ssl
import stat
import subprocess
import tempfile
import traceback

INSTANCE = '2395105c-9958-47b5-ae8b-5b0424cbfc4f'
HOST = 'pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv'
BASE = Path('/opt/server/management/config/caddy')
ROOT = Path('/var/lib/prichal-managed-publisher')
RENDERER = Path('/usr/local/libexec/prichal/managed-edge')
PARENT = BASE / 'sites/prichal.Caddyfile'
TARGET = BASE / 'sites/prichal-managed.Caddyfile'
IMPORT = b'import /etc/caddy/sites/prichal-managed.Caddyfile\n'
PARENT_IMPORT = b'import /etc/caddy/sites/prichal.Caddyfile'
UUID = re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$')
SHA = re.compile(r'^[a-f0-9]{64}$')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read(path, limit=1 << 20, private=False, root_owned=False):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_size > limit or info.st_mode & (0o077 if private else 0o022) or root_owned and info.st_uid != 0:
        raise ValueError('Bounded regular file with safe custody required')
    with path.open('rb') as source:
        if not os.path.samestat(info, os.fstat(source.fileno())):
            raise ValueError('File changed during custody inspection')
        data = source.read(limit + 1)
    if len(data) > limit:
        raise ValueError('File exceeded its custody bound')
    return data


def directory(path, private=False):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & (0o077 if private else 0o022):
        raise ValueError('Root-owned regular publication directory required')


def write_once(path, data, mode=0o600):
    with path.open('xb') as output:
        output.write(data)
        output.flush()
        os.fchmod(output.fileno(), mode)
        os.fsync(output.fileno())
    sync_directory(path.parent)


def sync_directory(path):
    fd = os.open(path, os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def replace(path, data):
    if data is None:
        path.unlink()
        sync_directory(path.parent)
        return
    fd, temporary = tempfile.mkstemp(prefix='.prichal-publish-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as output:
            output.write(data)
            output.flush()
            os.fchmod(output.fileno(), 0o644)
            os.fsync(output.fileno())
        os.replace(temporary, path)
        sync_directory(path.parent)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def run(arguments, work, label):
    result = subprocess.run(arguments, capture_output=True, timeout=60)
    # Logs can include existing private configuration; never return them to HTTP or stdout.
    with (work / (label + '.log')).open('ab') as output:
        output.write(result.stderr)
    if result.returncode:
        raise RuntimeError('Publication command failed; inspect root-only receipts')
    return result.stdout, result.stderr


def caddy(work, command, config):
    return run(['docker', 'exec', 'caddy', 'caddy', command, '--adapter', 'caddyfile', '--config', config], work, command)[0]


def active(work):
    body, headers = run(['docker', 'exec', 'caddy', 'wget', '-S', '-qO-', 'http://127.0.0.1:2019/config/'], work, 'active')
    tags = re.findall(rb'(?im)^\s*etag:\s*("[^\r\n]+")\s*$', headers)
    if len(tags) != 1:
        raise ValueError('Caddy root configuration ETag unavailable')
    return json.loads(body), tags[0].decode()


def load(work, config_path, expected, desired):
    current, tag = active(work)
    if current != expected:
        raise ValueError('Concurrent active configuration change; refusing reload')
    path = BASE / Path(config_path).relative_to('/etc/caddy')
    if json.loads(read(path, private=True, root_owned=True)) != desired:
        raise ValueError('Retained reload payload differs from the validated configuration')
    run(['docker', 'exec', 'caddy', 'wget', '-qO-', '--header=Content-Type: application/json',
         '--header=If-Match: ' + tag, '--post-file=' + config_path,
         'http://127.0.0.1:2019/config/'], work, 'reload')


def authority(data, expected):
    if not SHA.fullmatch(expected) or digest(data) != expected:
        raise ValueError('Inventory differs from operator-delivered export digest')
    value = json.loads(data)
    if set(value) != {'version', 'generation', 'expiresAt', 'endpoints', 'handles'} or value['version'] != 1 or type(value['generation']) is not int or value['generation'] <= 0 or value['handles'] != [] or len(value['endpoints']) != 1:
        raise ValueError('Only a complete authoritative single-fixture export is supported')
    endpoint = value['endpoints'][0]
    if set(endpoint) != {'deploymentId', 'hostname', 'forwardPort', 'certificateSha256'} or endpoint['deploymentId'] != INSTANCE or endpoint['hostname'] != HOST or type(endpoint['forwardPort']) is not int or endpoint['forwardPort'] != 35000 or not SHA.fullmatch(endpoint['certificateSha256']):
        raise ValueError('Publication is restricted to the retained staging-02 endpoint')
    expires = dt.datetime.fromisoformat(value['expiresAt'].replace('Z', '+00:00'))
    now = dt.datetime.now(dt.timezone.utc)
    if expires.tzinfo is None or not now < expires <= now + dt.timedelta(minutes=16):
        raise ValueError('Fresh bounded authenticated export required')
    return value


def parent_after(before):
    lines = before.splitlines(keepends=True)
    imports = [line for line in lines if line.strip() == IMPORT.strip()]
    if imports:
        if len(imports) != 1 or imports[0] != IMPORT:
            raise ValueError('Conflicting managed import')
        return before
    if b'prichal-managed' in before:
        raise ValueError('Unknown managed parent configuration')
    return before + (b'' if before.endswith(b'\n') else b'\n') + b'\n# Owned Prichal managed staging import.\n' + IMPORT


def sources():
    # Snapshot every mounted configuration/trust input, excluding only this helper's owned outputs.
    result = {}
    for path in [BASE / 'Caddyfile', *sorted((BASE / 'sites').rglob('*')), *sorted((BASE / 'certs').rglob('*'))]:
        relative = path.relative_to(BASE)
        if any(part.startswith('.prichal-managed-') for part in relative.parts) or relative.parts[:2] == ('certs', 'prichal-managed') or path in (PARENT, TARGET):
            continue
        info = path.lstat()
        if stat.S_ISDIR(info.st_mode):
            continue
        # Existing certificates may be operator-owned; configuration bytes and file type are pinned.
        if not stat.S_ISREG(info.st_mode) or info.st_size > 1 << 20:
            raise ValueError('Unbounded or linked Caddy input requires operator inspection')
        result[str(relative)] = digest(path.read_bytes())
    return result


def check_sources(receipt):
    if sources() != receipt['sources']:
        raise ValueError('Unrelated configuration changed; preserve evidence for reviewed recovery')


def production_hide_paths(value, staged_main):
    # Caddy automatically hides the input Caddyfile from file_server handlers.
    # The staged path must become the actual main path; all other JSON remains exact.
    value = json.loads(json.dumps(value))
    def visit(item):
        if isinstance(item, dict):
            if item.get('handler') == 'file_server' and isinstance(item.get('hide'), list):
                item['hide'] = ['/etc/caddy/Caddyfile' if path == staged_main else path for path in item['hide']]
            for child in item.values():
                visit(child)
        elif isinstance(item, list):
            for child in item:
                visit(child)
    visit(value)
    return value


def upstream(certificate):
    context = ssl.create_default_context(cadata=certificate.decode())
    context.minimum_version = ssl.TLSVersion.TLSv1_3
    paths = [('/xrpc/com.atproto.server.describeServer', 'did', 'did:web:' + HOST),
             ('/.well-known/oauth-authorization-server', 'issuer', 'https://' + HOST)]
    for path, key, expected in paths:
        with socket.create_connection(('172.20.0.1', 35000), timeout=5) as connection:
            with context.wrap_socket(connection, server_hostname=INSTANCE + '.localhost') as secure:
                secure.sendall(('GET ' + path + ' HTTP/1.1\r\nHost: ' + HOST + '\r\nConnection: close\r\n\r\n').encode())
                response = http.client.HTTPResponse(secure)
                response.begin()
                body = response.read((1 << 20) + 1)
                if response.status != 200 or len(body) > 1 << 20 or json.loads(body).get(key) != expected:
                    raise ValueError('Pinned upstream does not advertise the retained public identity')


def prepare(work, revision, bundle, expected):
    inventory = read(bundle / 'inventory.json', private=True)
    value = authority(inventory, expected)
    certificate = read(bundle / 'certificates' / (INSTANCE + '.crt'), 16384, private=True)
    if digest(certificate) != value['endpoints'][0]['certificateSha256']:
        raise ValueError('Certificate custody differs from authenticated export')
    parent = read(PARENT, root_owned=True)
    target = read(TARGET, root_owned=True) if os.path.lexists(TARGET) else None
    if target is not None or b'prichal-managed' in parent:
        raise ValueError('Initial staging publication requires an unused owned route/import; updates are unsupported')
    after_parent = parent_after(parent)
    original = json.loads(caddy(work, 'adapt', '/etc/caddy/Caddyfile'))
    if active(work)[0] != original:
        raise ValueError('Active Caddy configuration differs from disk')
    inputs = sources()
    for relative in inputs:
        if HOST.encode() in (BASE / relative).read_bytes():
            raise ValueError('Managed name conflicts with an existing unrelated input')
    cert_root = BASE / 'certs/prichal-managed'
    cert_root.mkdir(mode=0o755, exist_ok=True)
    directory(cert_root)
    certs = cert_root / revision
    certs.mkdir(mode=0o755)
    os.chmod(cert_root, 0o755)
    os.chmod(certs, 0o755)
    write_once(certs / (INSTANCE + '.crt'), certificate, 0o644)
    write_once(work / 'inventory.json', inventory)
    stage = BASE / 'sites' / ('.prichal-managed-' + revision)
    stage.mkdir(mode=0o700)
    stage_container = '/etc/caddy/sites/' + stage.name
    run([str(RENDERER), '--inventory', str(work / 'inventory.json'), '--certificates', str(certs),
         '--revision', revision, '--out', str(stage / 'route.Caddyfile')], work, 'render')
    route = read(stage / 'route.Caddyfile', private=True, root_owned=True)
    stage_parent = after_parent.replace(IMPORT, ('import ' + stage_container + '/route.Caddyfile\n').encode())
    main = read(BASE / 'Caddyfile')
    if main.count(PARENT_IMPORT) != 1:
        raise ValueError('Expected one existing Prichal parent import')
    stage_main = main.replace(PARENT_IMPORT, ('import ' + stage_container + '/parent.Caddyfile').encode())
    write_once(stage / 'parent.Caddyfile', stage_parent)
    write_once(stage / 'Caddyfile', stage_main)
    candidate = production_hide_paths(json.loads(caddy(work, 'adapt', stage_container + '/Caddyfile')), stage_container + '/Caddyfile')
    caddy(work, 'validate', stage_container + '/Caddyfile')
    upstream(certificate)
    for name, data in [('parent-before', parent), ('parent-after', after_parent), ('route-after', route)]:
        write_once(work / name, data)
    if target is not None:
        write_once(work / 'route-before', target)
    write_once(stage / 'before.json', json.dumps(original).encode())
    write_once(stage / 'after.json', json.dumps(candidate).encode())
    receipt = {'version': 1, 'revision': revision, 'inventorySHA256': expected, 'generation': value['generation'],
               'expiresAt': value['expiresAt'], 'sources': inputs, 'parentBefore': digest(parent),
               'parentAfter': digest(after_parent), 'routeBefore': digest(target) if target is not None else None,
               'routeAfter': digest(route), 'certificateSHA256': digest(certificate),
               'before': original, 'after': candidate, 'stage': stage_container}
    check_sources(receipt)
    if read(PARENT) != parent or (read(TARGET) if os.path.lexists(TARGET) else None) != target or active(work)[0] != original:
        raise ValueError('Publication inputs changed during preparation')
    write_once(work / 'prepared.json', json.dumps(receipt).encode())
    return {'prepared': True, 'revision': revision, 'generation': value['generation'], 'liveChanged': False}


def owned_state(receipt):
    parent = read(PARENT, root_owned=True)
    target = read(TARGET, root_owned=True) if os.path.lexists(TARGET) else None
    if digest(parent) not in (receipt['parentBefore'], receipt['parentAfter']) or (digest(target) if target is not None else None) not in (receipt['routeBefore'], receipt['routeAfter']):
        raise ValueError('Owned publication files changed; refusing overwrite')
    return parent, target


def rollback(work, receipt):
    check_sources(receipt)
    owned_state(receipt)
    current = active(work)[0]
    if current not in (receipt['before'], receipt['after']):
        raise ValueError('Concurrent active configuration changed; refusing rollback')
    before_parent = read(work / 'parent-before', private=True, root_owned=True)
    before_route = read(work / 'route-before', private=True, root_owned=True) if receipt['routeBefore'] is not None else None
    if digest(before_parent) != receipt['parentBefore'] or (digest(before_route) if before_route is not None else None) != receipt['routeBefore']:
        raise ValueError('Rollback custody differs')
    replace(PARENT, before_parent)
    if before_route is not None or os.path.lexists(TARGET):
        replace(TARGET, before_route)
    check_sources(receipt)
    caddy(work, 'validate', '/etc/caddy/Caddyfile')
    if json.loads(caddy(work, 'adapt', '/etc/caddy/Caddyfile')) != receipt['before']:
        raise ValueError('Restored full configuration differs; no reload attempted')
    load(work, receipt['stage'] + '/before.json', current, receipt['before'])
    if active(work)[0] != receipt['before']:
        raise ValueError('Rollback readback differs')
    check_sources(receipt)
    if not (work / 'rolled-back.json').exists():
        write_once(work / 'rolled-back.json', b'{"rolledBack":true}\n')
    return {'rolledBack': True, 'revision': receipt['revision']}


def publish(work, receipt):
    if (work / 'rolled-back.json').exists():
        raise ValueError('A rolled-back operation cannot be republished; obtain a fresh export/revision')
    check_sources(receipt)
    parent, target = owned_state(receipt)
    installed = digest(parent) == receipt['parentAfter'] and target is not None and digest(target) == receipt['routeAfter']
    if (work / 'published.json').exists():
        if not installed or active(work)[0] != receipt['after']:
            raise ValueError('Published readback changed')
        return {'published': True, 'revision': receipt['revision'], 'replayed': True}
    authority(read(work / 'inventory.json', private=True, root_owned=True), receipt['inventorySHA256'])
    certs = BASE / 'certs/prichal-managed' / receipt['revision']
    certificate = read(certs / (INSTANCE + '.crt'), 16384, root_owned=True)
    if digest(certificate) != receipt['certificateSHA256']:
        raise ValueError('Staged public trust changed')
    upstream(certificate)
    current = active(work)[0]
    if current not in (receipt['before'], receipt['after']):
        raise ValueError('Concurrent active configuration changed')
    route = read(work / 'route-after', private=True, root_owned=True)
    after_parent = read(work / 'parent-after', private=True, root_owned=True)
    if digest(route) != receipt['routeAfter'] or digest(after_parent) != receipt['parentAfter']:
        raise ValueError('Prepared publication custody differs')
    with tempfile.TemporaryDirectory(prefix='revalidate-', dir=work) as temporary:
        output = Path(temporary) / 'route.Caddyfile'
        run([str(RENDERER), '--inventory', str(work / 'inventory.json'), '--certificates', str(certs),
             '--revision', receipt['revision'], '--out', str(output)], work, 'render-recheck')
        if read(output, private=True, root_owned=True) != route:
            raise ValueError('Current renderer/certificate differs from prepared candidate')
    try:
        replace(TARGET, route)
        replace(PARENT, after_parent)
        check_sources(receipt)
        caddy(work, 'validate', '/etc/caddy/Caddyfile')
        live_candidate = json.loads(caddy(work, 'adapt', '/etc/caddy/Caddyfile'))
        if live_candidate != receipt['after']:
            write_once(work / 'refused-adapted.json', json.dumps(live_candidate).encode())
            raise ValueError('Live candidate differs from validated full configuration')
        load(work, receipt['stage'] + '/after.json', current, receipt['after'])
        if active(work)[0] != receipt['after']:
            raise ValueError('Published configuration readback differs')
        check_sources(receipt)
    except Exception:
        rollback(work, receipt)
        raise
    write_once(work / 'published.json', b'{"published":true}\n')
    return {'published': True, 'revision': receipt['revision'], 'generation': receipt['generation'], 'publicReadinessQualified': False}


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'publish', 'rollback'])
    parser.add_argument('--revision', required=True)
    parser.add_argument('--bundle', type=Path)
    parser.add_argument('--inventory-sha256')
    parser.add_argument('--confirm-staging-02', action='store_true')
    args = parser.parse_args()
    if not args.confirm_staging_02 or not UUID.fullmatch(args.revision) or os.geteuid() != 0 or socket.gethostname() != 'almaz':
        raise ValueError('Explicit root authorization on Almaz for the designated staging fixture required')
    ROOT.mkdir(mode=0o700, exist_ok=True)
    directory(ROOT, private=True)
    for path in (BASE / 'sites',):
        directory(path)
    read(RENDERER, 100 << 20, root_owned=True)
    lock = os.open(ROOT / 'publisher.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        work = ROOT / args.revision
        if args.action == 'prepare':
            if not args.bundle or not args.bundle.is_absolute() or not args.inventory_sha256:
                raise ValueError('Private export bundle and its independently delivered digest required')
            work.mkdir(mode=0o700)
            result = prepare(work, args.revision, args.bundle, args.inventory_sha256)
        else:
            directory(work, private=True)
            receipt = json.loads(read(work / 'prepared.json', private=True, root_owned=True))
            if receipt['revision'] != args.revision:
                raise ValueError('Retained publication revision differs')
            result = {'publish': publish, 'rollback': rollback}[args.action](work, receipt)
        print(json.dumps(result))
    finally:
        os.close(lock)


if __name__ == '__main__':
    try:
        main()
    except Exception:
        if os.geteuid() == 0 and ROOT.exists():
            directory(ROOT, private=True)
            fd, path = tempfile.mkstemp(prefix='refused-', suffix='.log', dir=ROOT)
            with os.fdopen(fd, 'w') as output:
                output.write(traceback.format_exc())
        raise SystemExit('Managed staging publication refused; inspect root-only receipts before retry')
