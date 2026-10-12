#!/usr/bin/env python3
"""Plan/bootstrap one fresh public staging PDS VM; retain disks and keys on every retry."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time

ROOT = Path('/mnt/fast1/apps/prichal-public-pds')
MOUNT_UUID = '6d24966a-f7db-42e5-b136-e3a6f6c3ac01'
IMAGE_SHA = '6a81c37564db9b1ee84e141922625e1d7c5b389b99bb3c572e0243607d5bb4d2'
UNIT = 'prichal-public-pds-vm.service'


def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, **kwargs)


def output(*args):
    return run(*args, capture_output=True).stdout.strip()


def check_mount():
    if output('findmnt', '-no', 'UUID', '--target', '/mnt/fast1') != MOUNT_UUID:
        raise RuntimeError('Required fast1 filesystem is not mounted; refusing fallback storage')
    if not os.access('/dev/kvm', os.R_OK | os.W_OK):
        raise RuntimeError('KVM access required')


def private(path, directory=False):
    info = path.lstat()
    if path.is_symlink() or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise RuntimeError(f'Private owned path required: {path}')
    if directory != path.is_dir():
        raise RuntimeError(f'Unexpected file type: {path}')


def plan():
    check_mount()
    if shutil.disk_usage('/mnt/fast1').free < 50 * 1024**3:
        raise RuntimeError('Need 50 GiB free to admit the 40 GiB disk and reserve')
    memory = dict(line.split(':', 1) for line in Path('/proc/meminfo').read_text().splitlines())
    if int(memory['MemAvailable'].split()[0]) < 8 * 1024**2:
        raise RuntimeError('Need 8 GiB available host memory for initial admission')
    print(json.dumps({'name': 'prichal-public-pds-01', 'vcpus': 3, 'memoryMiB': 4096,
                      'diskGiB': 40, 'ssh': '127.0.0.1:22260',
                      'agent': '127.0.0.1:22261', 'directory': str(ROOT),
                      'providerSpendUSD': 0, 'existingState': ROOT.exists()}, indent=2))


def launch():
    check_mount()
    private(ROOT, True)
    private(ROOT / 'disk.qcow2')
    receipt = json.loads((ROOT / 'bootstrap.json').read_text())
    if receipt['imageSHA256'] != IMAGE_SHA or receipt['name'] != 'prichal-public-pds-01':
        raise RuntimeError('Bootstrap identity mismatch')
    info = json.loads(output('qemu-img', 'info', '--output=json', str(ROOT / 'disk.qcow2')))
    if info['virtual-size'] != 40 * 1024**3 or info.get('backing-filename'):
        raise RuntimeError('Disk must be independent and exactly 40 GiB')
    os.execv('/usr/bin/qemu-system-x86_64', [
        'qemu-system-x86_64', '-name', 'prichal-public-pds-01', '-enable-kvm',
        '-cpu', 'host', '-smp', '3', '-m', '4096', '-machine', 'q35',
        '-drive', f'file={ROOT}/disk.qcow2,if=virtio,format=qcow2,discard=unmap',
        '-drive', f'file={ROOT}/seed.iso,media=cdrom,readonly=on',
        '-nic', 'user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:22260-:22',
        '-display', 'none', '-serial', f'file:{ROOT}/serial.log',
        '-monitor', f'unix:{ROOT}/monitor.sock,server,nowait'])


def powerdown(pid):
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as conn:
        conn.settimeout(5)
        conn.connect(str(ROOT / 'monitor.sock'))
        conn.recv(4096)
        conn.sendall(b'system_powerdown\n')
    deadline = time.monotonic() + 110
    while Path(f'/proc/{pid}').exists() and time.monotonic() < deadline:
        time.sleep(1)
    if Path(f'/proc/{pid}').exists():
        raise RuntimeError('Guest shutdown timed out; inspect before any forced stop')


def bootstrap(image):
    plan()
    script = Path(__file__).resolve()
    script_sha = hashlib.sha256(script.read_bytes()).hexdigest()
    unit_dir = Path.home() / '.config/systemd/user'
    unit_path = unit_dir / UNIT
    if ROOT.exists():
        private(ROOT, True)
        receipt = json.loads((ROOT / 'bootstrap.json').read_text())
        if receipt != {'name': 'prichal-public-pds-01', 'imageSHA256': IMAGE_SHA,
                       'operatorSHA256': script_sha}:
            raise RuntimeError('Existing bootstrap differs; preserve state for explicit upgrade')
        for name in ['disk.qcow2', 'seed.iso', 'admin.key', 'host.key', 'vm-control.py']:
            private(ROOT / name)
        if hashlib.sha256((ROOT / 'vm-control.py').read_bytes()).hexdigest() != script_sha:
            raise RuntimeError('Installed operator differs; preserve state')
        print('Existing bootstrap verified; no disk, key or node recreated.')
    else:
        if unit_path.exists():
            raise RuntimeError('Existing unit without matching state; preserve it')
        for port in [22260, 22261]:
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', port))
        if not image.is_file() or hashlib.file_digest(image.open('rb'), 'sha256').hexdigest() != IMAGE_SHA:
            raise RuntimeError('Image checksum mismatch')
        ROOT.mkdir(mode=0o700)
        run('qemu-img', 'convert', '-O', 'qcow2', str(image), str(ROOT / 'disk.qcow2'))
        (ROOT / 'disk.qcow2').chmod(0o600)
        run('qemu-img', 'resize', str(ROOT / 'disk.qcow2'), '40G')
        for name in ['admin', 'host']:
            run('ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-C', f'prichal-public-pds-{name}',
                '-f', str(ROOT / f'{name}.key'))
        config = {
            'hostname': 'prichal-public-pds-01', 'manage_etc_hosts': True,
            'disable_root': True, 'ssh_pwauth': False,
            'users': [{'name': 'prichal', 'shell': '/bin/bash', 'lock_passwd': True,
                       'sudo': 'ALL=(ALL) NOPASSWD:ALL',
                       'ssh_authorized_keys': [(ROOT / 'admin.key.pub').read_text().strip()]}],
            'ssh_keys': {'ed25519_private': (ROOT / 'host.key').read_text(),
                         'ed25519_public': (ROOT / 'host.key.pub').read_text()},
            'write_files': [{'path': '/etc/ssh/sshd_config.d/00-prichal.conf', 'content':
                             'PasswordAuthentication no\nPermitRootLogin no\nKbdInteractiveAuthentication no\n'}],
        }
        (ROOT / 'user-data').write_text('#cloud-config\n' + json.dumps(config))
        (ROOT / 'user-data').chmod(0o600)
        (ROOT / 'meta-data').write_text('instance-id: prichal-public-pds-01\nlocal-hostname: prichal-public-pds-01\n')
        run('genisoimage', '-quiet', '-output', str(ROOT / 'seed.iso'), '-volid', 'cidata',
            '-joliet', '-rock', str(ROOT / 'user-data'), str(ROOT / 'meta-data'))
        (ROOT / 'seed.iso').chmod(0o600)
        (ROOT / 'known_hosts').write_text('[127.0.0.1]:22260 ' + (ROOT / 'host.key.pub').read_text())
        (ROOT / 'known_hosts').chmod(0o600)
        shutil.copyfile(script, ROOT / 'vm-control.py')
        (ROOT / 'vm-control.py').chmod(0o600)
        (ROOT / 'bootstrap.json').write_text(json.dumps({'name': 'prichal-public-pds-01',
            'imageSHA256': IMAGE_SHA, 'operatorSHA256': script_sha}, indent=2) + '\n')
        (ROOT / 'bootstrap.json').chmod(0o600)
    unit = f'''[Unit]
Description=Prichal dedicated Ubuntu staging VM
After=network-online.target
Wants=network-online.target
RequiresMountsFor=/mnt/fast1/apps/prichal-public-pds

[Service]
Type=simple
ExecStart=/usr/bin/python {ROOT}/vm-control.py launch
ExecStop=/usr/bin/python {ROOT}/vm-control.py powerdown --pid $MAINPID
Restart=on-failure
RestartSec=10
TimeoutStopSec=120
KillSignal=SIGTERM
UMask=0077

[Install]
WantedBy=default.target
'''
    unit_dir.mkdir(parents=True, exist_ok=True)
    if unit_path.exists() and (unit_path.is_symlink() or unit_path.read_text() != unit):
        raise RuntimeError('Existing unit differs; preserve it')
    unit_path.write_text(unit)
    run('systemctl', '--user', 'daemon-reload')
    run('systemctl', '--user', 'enable', '--now', UNIT)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['plan', 'bootstrap', 'launch', 'powerdown'])
    parser.add_argument('--image', type=Path)
    parser.add_argument('--pid', type=int)
    args = parser.parse_args()
    if args.command == 'plan':
        plan()
    elif args.command == 'launch':
        launch()
    elif args.command == 'powerdown' and args.pid:
        powerdown(args.pid)
    elif args.command == 'bootstrap' and args.image:
        bootstrap(args.image.resolve())
    else:
        parser.error('bootstrap needs --image; powerdown needs --pid')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
