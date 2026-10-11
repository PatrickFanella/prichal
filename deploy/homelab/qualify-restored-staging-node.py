#!/usr/bin/env python3
"""Check the fixed isolated checkpoint guest, never the connected live node."""
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path('/mnt/fast1/apps/prichal-connection-restore-20261011')
VM = ROOT / 'extracted/mnt/fast1/apps/prichal-staging'
INSTANCE = '53a1f2d3-181b-4cf2-b242-2d35ca55c784'
NODE = '1ef77d45-c6d5-4bdb-b77b-ba1103b45a66'


def output(args):
    return subprocess.check_output(args, timeout=30)


def ssh(command):
    return output(['ssh', '-p', '22253', '-i', str(VM / 'admin.key'),
                   '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes',
                   '-o', 'UserKnownHostsFile='+str(ROOT / 'known_hosts'),
                   'prichal@127.0.0.1', command])


def sql(query):
    return output(['docker', 'exec', 'prichal-connection-restore-postgres-1',
                   'psql', '-U', 'pds_staging', '-d', 'pds_local', '-At',
                   '-v', 'ON_ERROR_STOP=1', '-c', query]).decode().strip()


def main():
    os.umask(0o077)
    pid = int(output(['systemctl', '--user', 'show', 'prichal-connection-restore-qualification.service', '--property=MainPID', '--value']))
    if pid <= 1:
        raise RuntimeError('Isolated restore guest must be running')
    command = Path(f'/proc/{pid}/cmdline').read_bytes()
    if b'restrict=yes' not in command or str(VM / 'disk.qcow2').encode() not in command:
        raise RuntimeError('Refusing an unverified restore transport')
    isolated = json.loads(output(['docker', 'network', 'inspect', 'prichal-connection-restore_isolated']))
    assert isolated[0]['Internal'] is True, 'Management restore requires an internal network'
    for role in ['api', 'issuer', 'web', 'sandbox', 'postgres']:
        metadata = json.loads(output(['docker', 'inspect', 'prichal-connection-restore-'+role+'-1']))[0]
        assert not metadata['HostConfig']['PortBindings']
        assert metadata['HostConfig']['NetworkMode'] != 'host'
    curl = 'curl -fsS --max-time 15 --cacert /tmp/prichal-restore-client/ca.crt --cert /tmp/prichal-restore-client/worker.crt --key /tmp/prichal-restore-client/worker.key '
    status = json.loads(ssh(curl+'https://127.0.0.1:9091/v1/instances/'+INSTANCE))
    assert status['id'] == INSTANCE and status['healthy'] and status['state'] == 'running'
    assert status['serviceGeneration'] == 2 and status['servicePhase'] == 'applied'
    usage = json.loads(ssh(curl+'https://127.0.0.1:9091/v1/instances/'+INSTANCE+'/usage'))
    assert usage['instanceId'] == INSTANCE and usage['accounts'] == 0
    containers = ssh('sudo docker ps --format "{{.ID}} {{.Names}}"').decode().splitlines()
    assert sorted(containers) == sorted([
        'ef37ab59e4cc pds-ass-tenant-'+INSTANCE,
        '51c5e82a2ad0 pds-ass-edge-'+INSTANCE])
    certificate = ssh('sudo cat /var/lib/prichal-agent/enrolled/node.crt')
    registered = base64.b64decode(sql("SELECT encode(certificate_pem,'base64') FROM hosting_nodes WHERE id='"+NODE+"'"))
    assert certificate == registered, 'Restored registry and guest certificate differ'
    public_key = ssh('sudo openssl pkey -in /var/lib/prichal-agent/enrolled/node.key -pubout -outform DER')
    pin = sql("SELECT encode(public_key_hash,'hex') FROM hosting_nodes WHERE id='"+NODE+"'")
    assert hashlib.sha256(public_key).hexdigest() == pin, 'Restored node key differs from registry pin'
    assert sql("SELECT node_id::text FROM instance_placements WHERE deployment_id='"+INSTANCE+"'") == NODE
    assert sql("SELECT count(*) FROM node_bootstrap_tokens WHERE consumed_at IS NOT NULL") == '1'
    assert sql("SELECT count(*) FROM service_operations WHERE state='applied'") == '2'
    version = json.loads(ssh('curl -fsS --max-time 15 --cacert /tmp/prichal-restore-client/pds.crt --resolve '+INSTANCE+'.localhost:37409:127.0.0.1 https://'+INSTANCE+'.localhost:37409/xrpc/_health'))
    assert version['version'] == '0.4.5036'
    report = {'nativeHealth': version['version'], 'runtimeIDsRetained': True,
              'registryCertificateAndKeyPinMatch': True, 'placementRetained': True,
              'consumedBootstrapRetained': True, 'appliedServiceGeneration': 2,
              'accounts': 0, 'outboundRestrictedGuest': True,
              'internalManagementNetwork': True, 'publishedManagementPorts': False}
    (ROOT / 'native-restore-report.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()
