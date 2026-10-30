import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


operator = load('node02', 'renew-staging-02-node.py')
guest = load('guest02', 'publish-certificate.py')


class Staging02Certificates(unittest.TestCase):
    def test_custody_must_be_unchanged_and_match_authenticated_capacity(self):
        fresh = '0f1e2d3c-4b5a-4697-a8b9-cadbecfd0e1f'
        for placements in ([], [operator.INSTANCE], [fresh]):
            capacity = {'maximumSlots': 1, 'cpus': 3, 'instanceIds': placements}
            self.assertTrue(operator.valid_custody(capacity, placements, placements))
            for changed in [[], [fresh], ['foreign'], [operator.INSTANCE, operator.INSTANCE]]:
                if changed != placements:
                    self.assertFalse(operator.valid_custody({**capacity, 'instanceIds': changed}, placements, placements))
                    self.assertFalse(operator.valid_custody(capacity, changed, placements))
                    self.assertFalse(operator.valid_custody({**capacity, 'instanceIds': changed}, changed, placements))
            self.assertFalse(operator.valid_custody({**capacity, 'maximumSlots': 2}, placements, placements))
            self.assertFalse(operator.valid_custody({**capacity, 'cpus': 4}, placements, placements))
        for invalid in [None, {}, ['foreign'], [fresh.upper()], [fresh, operator.INSTANCE], [7]]:
            self.assertFalse(operator.valid_placements(invalid))
            self.assertFalse(operator.valid_custody({'maximumSlots': 1, 'cpus': 3, 'instanceIds': invalid}, invalid, invalid))

    def test_registry_scope_refuses_other_hosts_ports_databases_and_query_overrides(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            file = root / 'database.env'
            with patch.object(operator, 'ROOT', root):
                for url in ['postgres://pds_staging:x@127.0.0.1:35435/pds_local',
                            'postgres://pds_staging:x@remote:35437/pds_local',
                            'postgres://pds_staging:x@127.0.0.1:35437/production',
                            'postgres://pds_staging:x@127.0.0.1:35437/pds_local?host=remote']:
                    file.write_text('PDS_DATABASE_URL=' + url)
                    file.chmod(0o600)
                    with self.assertRaises(ValueError):
                        operator.environment()
                file.write_text('PDS_DATABASE_URL=postgres://pds_staging:x@127.0.0.1:35437/pds_local')
                self.assertEqual(operator.environment()['PDS_MODE'], 'local')
                file.write_text(file.read_text() + '?sslmode=disable')
                self.assertEqual(operator.environment()['PDS_MODE'], 'local')
                file.chmod(0o644)
                with self.assertRaises(ValueError):
                    operator.environment()

    def test_real_certificate_guards_and_activation_rollback(self):
        with tempfile.TemporaryDirectory() as temporary:
            material = Path(temporary)
            def openssl(*args):
                subprocess.run(['openssl', *map(str, args)], check=True, capture_output=True)
            for name in ('ca', 'node', 'foreign'):
                openssl('genpkey', '-algorithm', 'EC', '-pkeyopt', 'ec_paramgen_curve:P-256', '-out', material/(name+'.key'))
            ca = material/'authority.crt'
            foreign_ca = material/'foreign.crt'
            for key, path in [('ca', ca), ('foreign', foreign_ca)]:
                openssl('req', '-new', '-x509', '-key', material/(key+'.key'), '-subj', '/CN='+key, '-days', '2', '-addext', 'basicConstraints=critical,CA:TRUE', '-out', path)
            extensions = material/'leaf.cnf'
            extensions.write_text('basicConstraints=critical,CA:FALSE\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1\n')
            def certificate(cn, key, name):
                csr, cert = material/(name+'.csr'), material/(name+'.crt')
                openssl('req', '-new', '-key', material/(key+'.key'), '-subj', '/CN='+cn, '-out', csr)
                openssl('x509', '-req', '-in', csr, '-CA', ca, '-CAkey', material/'ca.key', '-set_serial', str(len(list(material.iterdir()))), '-days', '2', '-extfile', extensions, '-out', cert)
                return cert
            old = certificate('pds-ass-node-'+operator.NODE, 'node', 'old')
            new = certificate('pds-ass-node-'+operator.NODE, 'node', 'new')
            wrong_subject = certificate('pds-ass-node-'+guest.NODE, 'node', 'wrong-subject')
            wrong_key = certificate('pds-ass-node-'+operator.NODE, 'foreign', 'wrong-key')
            extensions.write_text('basicConstraints=critical,CA:FALSE\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.2\n')
            wrong_ip = certificate('pds-ass-node-'+operator.NODE, 'node', 'wrong-ip')
            root = material/'state'
            root.mkdir(mode=0o700)
            (root / 'enrolled').mkdir(mode=0o700)
            files = {root/'ca.crt': ca.read_bytes(), root/'enrolled/node.crt': old.read_bytes(), root/'enrolled/node.key': (material/'node.key').read_bytes()}
            for path, data in files.items():
                path.write_bytes(data)
                path.chmod(0o600)
            commands = []
            real_output = guest.output
            def output(*args):
                if args[0] == 'systemctl':
                    commands.append(args)
                    if len(commands) == 1:
                        raise subprocess.CalledProcessError(1, args)
                    return b''
                return real_output(*args)
            def invoke(ca_value, leaf):
                stdin = type('Input', (), {'buffer': io.BytesIO(json.dumps({'ca': ca_value.read_text(), 'certificate': leaf.read_text()}).encode())})()
                with patch.object(guest, 'ROOT', root), patch.object(guest.os, 'geteuid', return_value=0), patch.object(guest, 'output', side_effect=output), patch.object(guest.sys, 'stdin', stdin):
                    guest.main(staging_02=True)
            for bad_ca, bad_leaf in [(ca, wrong_subject), (ca, wrong_key), (foreign_ca, new), (ca, wrong_ip)]:
                with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                    invoke(bad_ca, bad_leaf)
                self.assertEqual({path: path.read_bytes() for path in files}, files)
            self.assertEqual(commands, [])
            with self.assertRaises(subprocess.CalledProcessError):
                invoke(ca, new)
            self.assertEqual({path: path.read_bytes() for path in files}, files)
            self.assertEqual(commands, [('systemctl', 'restart', 'prichal-agent.service')]*2)
            invoke(ca, new)
            self.assertEqual((root/'enrolled/node.crt').read_bytes(), new.read_bytes())
            self.assertEqual((root/'enrolled/node.key').read_bytes(), files[root/'enrolled/node.key'])


if __name__ == '__main__':
    unittest.main()
