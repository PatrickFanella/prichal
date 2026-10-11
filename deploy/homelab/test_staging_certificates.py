import datetime as dt
import importlib.util
from pathlib import Path
import tempfile
import unittest
import io
import json
import subprocess
from types import SimpleNamespace
from unittest.mock import patch
from cryptography import x509
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID

spec = importlib.util.spec_from_file_location('staging_certificates', Path(__file__).with_name('renew-staging-certificates.py'))
operator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(operator)


class CertificateTests(unittest.TestCase):
    def test_only_retained_placements_are_admitted_by_renewal(self):
        capacity = {'maximumSlots':1,'cpus':3,'instanceIds':['retained-fixture']}
        self.assertTrue(operator.valid_custody(capacity,['retained-fixture']))
        self.assertFalse(operator.valid_custody(capacity,[]))
        self.assertFalse(operator.valid_custody({**capacity,'instanceIds':['retained-fixture','retained-fixture']},['retained-fixture']))
    def setUp(self):
        self.now = dt.datetime.now(dt.timezone.utc)
        self.key = ec.generate_private_key(ec.SECP256R1())
        name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, 'fixture-ca')])
        self.old = x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(self.key.public_key()).serial_number(1).not_valid_before(self.now-dt.timedelta(minutes=1)).not_valid_after(self.now+dt.timedelta(days=7)).add_extension(x509.BasicConstraints(ca=True,path_length=0),True).sign(self.key,hashes.SHA256())

    def test_retained_keys_and_strict_profiles(self):
        ca = operator.make_certificate('ca', self.old, self.key, self.old, self.key, self.now)
        self.assertEqual(operator.public_der(ca.public_key()), operator.public_der(self.old.public_key()))
        self.assertTrue(ca.extensions.get_extension_for_class(x509.KeyUsage).value.key_cert_sign)
        ca.extensions.get_extension_for_class(x509.SubjectKeyIdentifier)
        ca.extensions.get_extension_for_class(x509.AuthorityKeyIdentifier)
        listener = operator.make_certificate('listener', self.old, self.key, ca, self.key, self.now)
        self.assertEqual(listener.extensions.get_extension_for_class(x509.ExtendedKeyUsage).value[0], ExtendedKeyUsageOID.SERVER_AUTH)
        listener.verify_directly_issued_by(ca)

    def test_node_renewal_window(self):
        ca = operator.make_certificate('ca', self.old, self.key, self.old, self.key, self.now)
        leaf = operator.make_certificate('listener', self.old, self.key, ca, self.key, self.now)
        certs = {'ca': ca, 'listener': leaf, 'worker': leaf}
        node = SimpleNamespace(not_valid_after_utc=self.now+dt.timedelta(hours=24))
        self.assertFalse(operator.renewal_due(self.now, node, certs))
        self.assertFalse(operator.renewal_due(self.now+dt.timedelta(hours=15), node, certs))
        self.assertTrue(operator.renewal_due(self.now+dt.timedelta(hours=16), node, certs))
        self.assertTrue(operator.renewal_due(self.now, node, certs, force=True))
        self.assertTrue(operator.renewal_due(self.now, node, {'ca':self.old,'listener':leaf,'worker':leaf}))

    def test_symlink_publication_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root/'target'
            target.write_bytes(b'retained')
            target.chmod(0o600)
            link = root/'link'
            link.symlink_to(target)
            with self.assertRaises(ValueError):
                operator.publish(link,b'changed')
            self.assertEqual(target.read_bytes(),b'retained')

    def test_guest_activation_failure_restores_certificate_pair(self):
        guest_spec = importlib.util.spec_from_file_location('guest_publisher', Path(__file__).with_name('publish-certificate.py'))
        guest = importlib.util.module_from_spec(guest_spec)
        guest_spec.loader.exec_module(guest)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'enrolled').mkdir(mode=0o700)
            ca, cert, key = root/'ca.crt', root/'enrolled/node.crt', root/'enrolled/node.key'
            for path, content in [(ca,b'old-ca'),(cert,b'old-node'),(key,b'retained-key')]:
                path.write_bytes(content)
                path.chmod(0o600)
            restarts = []
            def command(*args):
                if args[:2] == ('systemctl','restart'):
                    restarts.append(True)
                    if len(restarts)==1:
                        raise subprocess.CalledProcessError(1,args)
                if '-subject' in args:
                    return ('subject=CN=pds-ass-node-'+guest.NODE).encode()
                if '-pubkey' in args or '-pubout' in args:
                    return b'fixture-public-key'
                return b''
            stdin = type('Input',(),{'buffer':io.BytesIO(json.dumps({'ca':'new-ca','certificate':'new-node'}).encode())})()
            with patch.object(guest,'ROOT',root), patch.object(guest.os,'geteuid',return_value=0), patch.object(guest,'output',side_effect=command), patch.object(guest.sys,'stdin',stdin):
                with self.assertRaises(subprocess.CalledProcessError):
                    guest.main()
            self.assertEqual(ca.read_bytes(),b'old-ca')
            self.assertEqual(cert.read_bytes(),b'old-node')
            self.assertEqual(key.read_bytes(),b'retained-key')
            self.assertEqual(len(restarts),2)


if __name__ == '__main__':
    unittest.main()
