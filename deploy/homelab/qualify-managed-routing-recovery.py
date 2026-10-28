#!/usr/bin/env python3
"""Qualify retained routing metadata only in the dedicated disposable staging-02 registry."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import stat
from urllib.parse import urlparse, urlunparse
import uuid

PARENT = Path.home() / '.local/state/prichal-staging-02-bootstrap'
ROOT = PARENT / 'managed-routing'
CONTAINER = 'prichal-staging-02-registry-postgres-1'
IMAGE = 'postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73'
RECOVERY = re.compile(r'^pds_recovery_[a-f0-9]{32}$')


def qualified_root(name):
    if name not in ('managed-routing', 'managed-conversion'):
        raise ValueError('Only the fixed staging qualification workspaces are permitted')
    return PARENT / name


def retained_snapshot(name):
    if name not in ('management-before.dump', 'management-after.dump') or ROOT.name != 'managed-conversion':
        raise ValueError('Only retained before/after staging conversion snapshots are permitted')
    path = ROOT / name
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077 or info.st_size > 1 << 30:
        raise ValueError('Private bounded retained management snapshot required')
    expected = os.environ.get('PDS_MANAGED_RECOVERY_SNAPSHOT_SHA256', '')
    if not re.fullmatch(r'[a-f0-9]{64}', expected) or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
        raise ValueError('Retained management snapshot differs from custody digest')
    return path


def validate_scope(information, database_url):
    labels = information.get('Config', {}).get('Labels', {})
    parsed = urlparse(database_url)
    if (labels.get('com.docker.compose.project') != 'prichal-staging-02-registry'
            or labels.get('com.docker.compose.service') != 'postgres'
            or information.get('Config', {}).get('Image') != IMAGE
            or parsed.scheme != 'postgres' or parsed.hostname != '127.0.0.1'
            or parsed.port != 35437 or parsed.path != '/pds_local'
            or parsed.username != 'pds_staging' or parsed.query != 'sslmode=disable' or parsed.fragment):
        raise ValueError('Dedicated staging-02 registry required')


def recovery_identifier(database):
    if not RECOVERY.fullmatch(database):
        raise ValueError('Only allocator-owned disposable recovery identifiers permitted')
    return database


def run(arguments, **kwargs):
    try:
        return subprocess.run(arguments, check=True, capture_output=True, timeout=60, **kwargs)
    except subprocess.CalledProcessError as error:
        detail = error.stderr or b''
        if isinstance(detail, str):
            detail = detail.encode()
        with (ROOT / 'last-recovery-error.log').open('ab') as log:
            log.write(detail + b'\n')
        raise RuntimeError('Private staging subprocess failed; inspect last-recovery-error.log') from None


def sql(database, query):
    if database != 'pds_local':
        recovery_identifier(database)
    return run(['docker', 'exec', CONTAINER, 'psql', '-U', 'pds_staging', '-d', database,
                '-At', '-v', 'ON_ERROR_STOP=1', '-c', query], text=True).stdout.strip()


def inventory(database, tables):
    pairs = []
    for table in tables:
        if not re.fullmatch(r'[a-z][a-z0-9_]{0,62}', table):
            raise ValueError('Unexpected inventory identifier')
        value = "(SELECT json_build_object('count',count(*),'sha',md5(coalesce(string_agg(row_to_json(r)::text,E'\\n' ORDER BY row_to_json(r)::text),''))) FROM public.\"" + table + "\" r)"
        pairs.append("('" + table + "'," + value + ")")
    return json.loads(sql(database, 'SELECT json_object_agg(name,value) FROM (VALUES ' + ','.join(pairs) + ') entries(name,value)'))


def main():
    global ROOT
    ROOT = qualified_root(os.environ.get('PDS_MANAGED_RECOVERY_WORKSPACE', 'managed-routing'))
    os.umask(0o077)
    for directory in [PARENT, ROOT]:
        info = directory.lstat()
        if directory.is_symlink() or not directory.is_dir() or info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise ValueError('Private owned qualification directory required')
    environment = os.environ.copy()
    for line in (PARENT / 'database.env').read_text().splitlines():
        name, value = line.split('=', 1)
        environment[name] = value
    information = json.loads(run(['docker', 'inspect', CONTAINER], text=True).stdout)[0]
    validate_scope(information, environment['PDS_DATABASE_URL'])
    database = recovery_identifier('pds_recovery_' + uuid.uuid4().hex)
    work = ROOT / database
    work.mkdir(mode=0o700)
    tables = sql('pds_local', "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename").splitlines()
    before = inventory('pds_local', tables)
    if before.get('managed_public_endpoints', {}).get('count') != 1 or before.get('managed_edge_observations', {}).get('count', 0) < 1:
        raise ValueError('Expected the single retained native readback before snapshot')
    sequence_sql = "SELECT coalesce(jsonb_object_agg(sequencename,last_value),'{}'::jsonb) FROM pg_sequences WHERE schemaname='public'"
    sequences = json.loads(sql('pds_local', sequence_sql))
    retained = os.environ.get('PDS_MANAGED_RECOVERY_RETAINED_SNAPSHOT', '')
    if retained:
        dump = retained_snapshot(retained)
    else:
        dump = work / 'snapshot.dump'
        with dump.open('xb') as output:
            subprocess.run(['docker', 'exec', CONTAINER, 'pg_dump', '-U', 'pds_staging', '-Fc', 'pds_local'], check=True, stdout=output, stderr=subprocess.PIPE, timeout=60)
    digest = hashlib.sha256(dump.read_bytes()).hexdigest()
    sql('pds_local', 'CREATE DATABASE ' + database)
    with dump.open('rb') as source:
        subprocess.run(['docker', 'exec', '-i', CONTAINER, 'pg_restore', '-U', 'pds_staging', '--exit-on-error', '--no-owner', '-d', database], check=True, stdin=source, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
    if inventory(database, tables) != before or json.loads(sql(database, sequence_sql)) != sequences:
        raise ValueError('Restored complete inventory or sequences differ')
    parsed = urlparse(environment['PDS_DATABASE_URL'])
    environment.update(PDS_MODE='local', PDS_STAGING_PROVISIONING='', PDS_DATABASE_URL=urlunparse(parsed._replace(path='/' + database)), PDS_RECOVERY_SNAPSHOT_SHA256=digest)
    for phase in ['unmarked', 'held']:
        if phase == 'held':
            run([str(ROOT / 'management-recovery'), 'hold'], env=environment, text=True)
        target = work / (phase + '-refused-bundle')
        result = subprocess.run([str(ROOT / 'managed-route-admin'), 'export', '--out', str(target)], env=environment, capture_output=True, text=True, timeout=45)
        if result.returncode == 0 or 'management recovery requires host reconciliation' not in result.stderr or target.exists():
            raise ValueError('Unreconciled recovery exported public routing authority')
    for table in ['managed_public_endpoints', 'managed_edge_observations']:
        if inventory(database, [table])[table] != before[table]:
            raise ValueError('Recovery hold or refused export changed retained routing metadata')
    if inventory('pds_local', tables) != before or json.loads(sql('pds_local', sequence_sql)) != sequences:
        raise ValueError('Snapshot qualification changed the source')
    sql('pds_local', 'ALTER DATABASE ' + recovery_identifier(database) + ' ALLOW_CONNECTIONS false')
    sql('pds_local', 'DROP DATABASE ' + recovery_identifier(database) + ' WITH (FORCE)')
    if not retained:
        dump.unlink()
    report = {'database': database, 'snapshotSHA256': digest, 'schemaVersion': int(sql('pds_local', 'SELECT version FROM schema_version')), 'tablesVerified': len(tables), 'sequencesVerified': True, 'retainedRoutingVerified': True, 'conversionRecordsVerified': before.get('managed_native_conversions', {}).get('count', 0), 'retainedSnapshot': bool(retained), 'unmarkedAndHeldExportsRefused': True, 'sourceUnchanged': True, 'disposableCopyDropped': True, 'publicRoutePublished': False}
    (work / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()
