#!/usr/bin/env python3
"""Fixed private staging certificate operator; no private node key leaves the VM."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.x509.oid import ExtendedKeyUsageOID
import ipaddress

ROOT = Path.home() / '.local/state/prichal-staging-control'
VM = Path('/mnt/fast1/apps/prichal-staging')
NODE = '1ef77d45-c6d5-4bdb-b77b-ba1103b45a66'
SSH = ['ssh', '-p', '22250', '-i', str(VM / 'admin.key'), '-o', 'IdentitiesOnly=yes',
       '-o', 'StrictHostKeyChecking=yes', '-o', f'UserKnownHostsFile={VM}/known_hosts',
       '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', '-o', 'ControlMaster=no',
       '-o', 'ControlPath=none', 'prichal@127.0.0.1']


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=60, **kwargs)


def owned_private(path, directory=False):
    info = path.lstat()
    if path.is_symlink() or info.st_uid != os.getuid() or info.st_mode & 0o077 or path.is_dir() != directory:
        raise ValueError('Private owned file/directory required')


def public_der(key):
    return key.public_bytes(serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)


def publish(path, data):
    owned_private(path)
    previous = path.read_bytes()
    fd, name = tempfile.mkstemp(dir=path.parent, prefix='.candidate-')
    try:
        with os.fdopen(fd, 'wb') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        if path.read_bytes() != previous:
            raise ValueError('Concurrent certificate change; preserve candidate')
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        Path(name).unlink(missing_ok=True)


def load_material(role):
    cert_path, key_path = ROOT / f'pki/{role}.crt', ROOT / f'pki/{role}.key'
    owned_private(cert_path)
    owned_private(key_path)
    cert = x509.load_pem_x509_certificate(cert_path.read_bytes())
    key = serialization.load_pem_private_key(key_path.read_bytes(), password=None)
    if public_der(cert.public_key()) != public_der(key.public_key()):
        raise ValueError('Retained certificate/key mismatch')
    return cert, key


def make_certificate(role, old, key, ca, ca_key, now):
    root = role == 'ca'
    builder = x509.CertificateBuilder().subject_name(old.subject).issuer_name(old.subject if root else ca.subject)
    builder = builder.public_key(key.public_key()).serial_number(x509.random_serial_number())
    expiry = now + dt.timedelta(days=90 if root else 7)
    if not root:
        expiry = min(expiry, ca.not_valid_after_utc)
    builder = builder.not_valid_before(now - dt.timedelta(minutes=1)).not_valid_after(expiry)
    builder = builder.add_extension(x509.BasicConstraints(ca=root, path_length=0 if root else None), critical=True)
    builder = builder.add_extension(x509.SubjectKeyIdentifier.from_public_key(key.public_key()), critical=False)
    builder = builder.add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key((key if root else ca_key).public_key()), critical=False)
    builder = builder.add_extension(x509.KeyUsage(digital_signature=True, content_commitment=False,
        key_encipherment=False, data_encipherment=False, key_agreement=False, key_cert_sign=root,
        crl_sign=root, encipher_only=None, decipher_only=None), critical=True)
    if not root:
        usage = ExtendedKeyUsageOID.SERVER_AUTH if role == 'listener' else ExtendedKeyUsageOID.CLIENT_AUTH
        builder = builder.add_extension(x509.ExtendedKeyUsage([usage]), critical=False)
        if role == 'listener':
            builder = builder.add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address('127.0.0.1'))]), critical=False)
    return builder.sign(key if root else ca_key, hashes.SHA256())


def renewal_due(now, node, certs, force=False):
    if force or node.not_valid_after_utc - now <= dt.timedelta(hours=8):
        return True
    for cert in certs.values():
        try:
            for kind in [x509.SubjectKeyIdentifier, x509.AuthorityKeyIdentifier, x509.KeyUsage]:
                cert.extensions.get_extension_for_class(kind)
        except x509.ExtensionNotFound:
            return True
    return (certs['ca'].not_valid_after_utc - now <= dt.timedelta(days=30)
            or any(certs[r].not_valid_after_utc - now <= dt.timedelta(days=3)
                   for r in ['listener', 'worker']))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--force', action='store_true', help='qualify renewal with fresh credentials')
    args = parser.parse_args()
    os.umask(0o077)
    owned_private(ROOT, True)
    owned_private(ROOT / 'pki', True)
    with (ROOT / 'renewal.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        now = dt.datetime.now(dt.timezone.utc)
        certs, keys = {}, {}
        for role in ['ca', 'listener', 'worker']:
            certs[role], keys[role] = load_material(role)
        if certs['ca'].subject.rfc4514_string() != 'CN=prichal-staging-qualification-ca':
            raise ValueError('Wrong staging trust domain')
        for role, cn in [('listener', 'pds-ass-agent'), ('worker', 'pds-ass-worker')]:
            if certs[role].subject.rfc4514_string() != 'CN=' + cn:
                raise ValueError('Wrong workload identity')
        current = run(SSH + ['sudo cat /var/lib/prichal-agent/enrolled/node.crt']).stdout
        node = x509.load_pem_x509_certificate(current)
        if node.subject.rfc4514_string() != 'CN=pds-ass-node-' + NODE:
            raise ValueError('Wrong node identity')
        if not certs['ca'].extensions.get_extension_for_class(x509.BasicConstraints).value.ca:
            raise ValueError('Signing certificate is not a CA')
        due = renewal_due(now, node, certs, args.force)
        if not due:
            health()
            record(now, False, node)
            return
        receipt = ROOT / 'renewals' / now.strftime('%Y%m%dT%H%M%S%fZ')
        receipt.mkdir(parents=True, mode=0o700)
        for role in certs:
            shutil.copyfile(ROOT / f'pki/{role}.crt', receipt / f'{role}-before.crt')
        (receipt / 'node-before.crt').write_bytes(current)
        ca = make_certificate('ca', certs['ca'], keys['ca'], certs['ca'], keys['ca'], now)
        publish(ROOT / 'pki/ca.crt', ca.public_bytes(serialization.Encoding.PEM))
        for role in ['listener', 'worker']:
            cert = make_certificate(role, certs[role], keys[role], ca, keys['ca'], now)
            publish(ROOT / f'pki/{role}.crt', cert.public_bytes(serialization.Encoding.PEM))
        run(['systemctl', '--user', 'restart', 'prichal-staging-enrollment.service'])
        env = os.environ.copy()
        owned_private(ROOT / 'admin.env')
        for line in (ROOT / 'admin.env').read_text().splitlines():
            name, value = line.split('=', 1)
            env[name] = value
        candidate = receipt / 'node.crt'
        run([str(ROOT / 'bin/node-admin'), 'renew', '--node', NODE, '--out', str(candidate)], env=env)
        renewed = x509.load_pem_x509_certificate(candidate.read_bytes())
        if public_der(renewed.public_key()) != public_der(node.public_key()) or renewed.subject != node.subject:
            raise ValueError('Renewal changed retained node identity')
        # Public certificates only; the node key and signer never cross hosts.
        payload = json.dumps({'ca': (ROOT / 'pki/ca.crt').read_text(), 'certificate': candidate.read_text()}).encode()
        run(SSH + ['sudo /usr/bin/python3 /usr/local/libexec/prichal/publish-certificate.py'], input=payload)
        health()
        record(now, True, renewed)


def health():
    result = run(['curl', '-fsS', '--max-time', '10', '--cacert', str(ROOT / 'pki/ca.crt'),
        '--cert', str(ROOT / 'pki/worker.crt'), '--key', str(ROOT / 'pki/worker.key'),
        'https://127.0.0.1:22251/v1/capacity'])
    capacity = json.loads(result.stdout)
    if capacity['maximumSlots'] != 1 or capacity['cpus'] != 3 or capacity['instanceIds']:
        raise ValueError('Staging capacity/custody changed')


def record(now, renewed, node):
    path = ROOT / 'last-renewal.json'
    result = {'success': True, 'renewed': renewed, 'checkedAt': now.isoformat(),
              'nodeNotAfter': node.not_valid_after_utc.isoformat()}
    data = (json.dumps(result, indent=2) + '\n').encode()
    if not path.exists():
        path.write_bytes(data)
        path.chmod(0o600)
    else:
        publish(path, data)
    print(json.dumps(result))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        if ROOT.is_dir():
            path = ROOT / 'last-renewal-error.json'
            data = (json.dumps({'success': False, 'errorType': type(error).__name__,
                    'checkedAt': dt.datetime.now(dt.timezone.utc).isoformat()}) + '\n').encode()
            if path.exists():
                publish(path, data)
            else:
                with path.open('xb') as file:
                    file.write(data)
                path.chmod(0o600)
        raise
