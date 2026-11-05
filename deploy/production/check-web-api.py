#!/usr/bin/env python3
"""Refuse frontend promotion if its retained management API contract changed."""
import hashlib
import json
from pathlib import Path
import re
import subprocess


def read(command):
    return subprocess.check_output(command, stderr=subprocess.PIPE, timeout=20)


def qualify(image_reference, revision):
    state = Path.home() / '.local/state/fleet-deploy/prichal-web'
    registration = json.loads((state / 'registration.json').read_text())
    api = json.loads(read(['docker', 'inspect', 'prichal-api-1']))[0]
    assert api['State']['Running']
    assert api['Image'] == registration['peers']['prichal-api-1']['image']
    assert hashlib.sha256(Path('/etc/prichal/production.env').read_bytes()).hexdigest() == registration['native_env_file_sha256']
    assert re.fullmatch(r'[0-9a-f]{40}', revision)
    image = json.loads(read(['docker', 'image', 'inspect', image_reference]))[0]
    assert image['Config']['Labels']['org.opencontainers.image.revision'] == revision
    assert image['Config']['Labels']['org.opencontainers.image.source'] == 'https://git.subcult.tv/subculture-collective/prichal'
    assert image['Config']['Labels']['tv.subcult.prichal.management.revision'] == api['Config']['Image'].removeprefix('prichal-api:')
    readiness = json.loads(read(['docker', 'exec', 'prichal-web-1', 'wget', '-q', '-T', '5', '-O', '-',
                                '--header', 'Host: prichal-app.subcult.tv', 'http://127.0.0.1/api/readiness']))
    assert readiness == registration['readiness'] and readiness['ready'] is True
    print('PASS: admitted management source matches the retained API, readiness and native configuration')


def main():
    revision = read(['git', 'ls-remote', '--exit-code', 'https://git.subcult.tv/subculture-collective/prichal.git', 'refs/heads/main']).decode().split()[0]
    qualify('fleet-deploy/prichal-web/web:' + revision, revision)


if __name__ == '__main__':
    try:
        main()
    except Exception:
        raise SystemExit('Management API contract changed; requalify the web deployment profile before activation') from None
