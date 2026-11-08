#!/usr/bin/env python3
"""Read-only check that generated managed hostnames are served by the subcult.tv wildcard route and edge certificate.

Generated `pds-` and `at-` names have no DNS records of their own. They resolve through the proxied
`*.subcult.tv` CNAME and are covered by the zone's Universal SSL certificate. This check confirms that
route still exists, that no explicit record shadows an inventory hostname (which would bypass the edge
or let another target answer a handle proof), and that the edge certificate is active and not near expiry.
It never writes to Cloudflare and never prints the token.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import urllib.error
import urllib.request

ZONE = 'subcult.tv'
WILDCARD = '*.' + ZONE
TOKEN = Path.home() / '.local/state/prichal-cloudflare/subcult-dns.token'
API = 'https://api.cloudflare.com/client/v4'
GENERATED = re.compile(r'^(pds|at)-[0-9a-f]{32}\.' + re.escape(ZONE) + r'$')
# Universal certificates renew about 30 days before expiry; under 14 days means renewal has stalled.
CERTIFICATE_MARGIN = dt.timedelta(days=14)


def hostnames_from_inventory(path):
    value = json.loads(Path(path).read_text())
    if not isinstance(value, dict) or value.get('version') != 1:
        raise ValueError('Managed inventory version 1 required')
    names = [item['hostname'] for item in value.get('endpoints', [])] + [item['hostname'] for item in value.get('handles', [])]
    return names


def evaluate(zone, records, universal, packs, hostnames, now):
    """Return (problems, evidence) for the fetched zone state; pure so tests need no network."""
    problems = []
    if not hostnames:
        problems.append('no hostnames to check')
    for name in hostnames:
        if not GENERATED.fullmatch(name):
            problems.append(f'{name}: not a generated managed hostname')
    if zone.get('name') != ZONE or zone.get('status') != 'active' or zone.get('type') != 'full':
        problems.append('zone is not an active full setup')
    wildcard = [r for r in records if r.get('name') == WILDCARD]
    evidence = {'zone': ZONE, 'records': len(records)}
    if len(wildcard) != 1 or wildcard[0].get('type') != 'CNAME' or wildcard[0].get('proxied') is not True \
            or not str(wildcard[0].get('content', '')).endswith('.cfargotunnel.com'):
        problems.append('wildcard is not exactly one proxied CNAME to a Cloudflare Tunnel')
    else:
        evidence['wildcardTargetSha256'] = hashlib.sha256(wildcard[0]['content'].encode()).hexdigest()
    for name in hostnames:
        shadows = sorted({(r.get('type'), r.get('name')) for r in records
                          if r.get('name') in (name, '_atproto.' + name) or str(r.get('name', '')).endswith('.' + name)})
        if shadows:
            problems.append(f'{name}: shadowed by {", ".join(f"{t} {n}" for t, n in shadows)}')
    if universal.get('enabled') is not True:
        problems.append('Universal SSL is disabled')
    covering = []
    for pack in packs:
        if pack.get('status') != 'active' or WILDCARD not in pack.get('hosts', []):
            continue
        expiries = [dt.datetime.fromisoformat(c['expires_on'].replace('Z', '+00:00')) for c in pack.get('certificates', []) if c.get('expires_on')]
        if expiries:
            covering.append(min(expiries))
    if not covering:
        problems.append('no active edge certificate covers ' + WILDCARD)
    else:
        expiry = max(covering)
        evidence['certificateExpiresAt'] = expiry.isoformat()
        if expiry - now < CERTIFICATE_MARGIN:
            problems.append('edge certificate for ' + WILDCARD + ' expires within 14 days')
    evidence['hostnames'] = len(hostnames)
    return problems, evidence


def token():
    info = TOKEN.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Owner-only regular token file required')
    return TOKEN.read_text().strip()


def get(path, secret):
    request = urllib.request.Request(API + path, headers={'Authorization': 'Bearer ' + secret})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            value = json.load(response)
    except urllib.error.HTTPError as error:
        value = json.load(error)
    if not value.get('success'):
        raise RuntimeError('Cloudflare read refused: ' + path.split('?')[0].rsplit('/', 1)[-1])
    return value


def fetch(secret):
    zones = get('/zones?name=' + ZONE, secret)['result']
    if len(zones) != 1:
        raise RuntimeError('Zone not visible to this token')
    zone = zones[0]
    records, page = [], 1
    while True:
        value = get(f"/zones/{zone['id']}/dns_records?per_page=500&page={page}", secret)
        records += value['result']
        if page >= value['result_info']['total_pages']:
            break
        page += 1
    universal = get(f"/zones/{zone['id']}/ssl/universal/settings", secret)['result']
    packs = get(f"/zones/{zone['id']}/ssl/certificate_packs?status=all", secret)['result']
    return zone, records, universal, packs


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--inventory', help='exported managed inventory.json')
    parser.add_argument('--hostname', action='append', default=[], help='generated hostname to check (repeatable)')
    args = parser.parse_args(argv)
    names = list(args.hostname)
    if args.inventory:
        names += hostnames_from_inventory(args.inventory)
    problems, evidence = evaluate(*fetch(token()), names, dt.datetime.now(dt.timezone.utc))
    print(json.dumps({'ok': not problems, 'problems': problems, **evidence}, indent=2))
    return 1 if problems else 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:
        # The message names the failing step only; no token or response body is echoed.
        print(json.dumps({'ok': False, 'error': type(error).__name__, 'detail': str(error)[:200]}))
        sys.exit(2)
