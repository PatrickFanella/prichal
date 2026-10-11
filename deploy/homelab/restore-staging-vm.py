#!/usr/bin/env python3
"""Boot only an independent staging checkpoint with outbound QEMU networking denied."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import time

ROOT = Path('/mnt/fast1/apps/prichal-connection-restore-20261011')
VM = ROOT / 'extracted/mnt/fast1/apps/prichal-staging'


def launch():
    spec = importlib.util.spec_from_file_location('staging_vm', Path(__file__).with_name('staging-vm.py'))
    operator = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(operator)
    operator.check_mount()
    for directory in [ROOT, ROOT / 'extracted', VM]:
        operator.private(directory, True)
    for name in ['disk.qcow2', 'seed.iso', 'admin.key', 'host.key']:
        operator.private(VM / name)
    info = json.loads(subprocess.check_output(['qemu-img', 'info', '--output=json', str(VM / 'disk.qcow2')]))
    if info['virtual-size'] != 40 * 1024**3 or info.get('backing-filename'):
        raise RuntimeError('Restore requires an independent 40 GiB disk')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 22253))
    os.execv('/usr/bin/qemu-system-x86_64', [
        'qemu-system-x86_64', '-name', 'prichal-connection-restore', '-enable-kvm',
        '-cpu', 'host', '-smp', '3', '-m', '4096', '-machine', 'q35',
        '-drive', f'file={VM}/disk.qcow2,if=virtio,format=qcow2',
        '-drive', f'file={VM}/seed.iso,media=cdrom,readonly=on',
        '-nic', 'user,restrict=yes,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:22253-:22',
        '-display', 'none', '-serial', f'file:{ROOT}/serial.log',
        '-monitor', f'unix:{ROOT}/monitor.sock,server,nowait'])


def powerdown(pid):
    command = Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0')
    if str(VM / 'disk.qcow2').encode() not in b'\0'.join(command) or b'prichal-connection-restore' not in command:
        raise RuntimeError('Refusing to stop a foreign process')
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as conn:
        conn.settimeout(5)
        conn.connect(str(ROOT / 'monitor.sock'))
        conn.recv(4096)
        conn.sendall(b'system_powerdown\n')
    deadline = time.monotonic() + 110
    while Path(f'/proc/{pid}').exists() and time.monotonic() < deadline:
        time.sleep(1)
    if Path(f'/proc/{pid}').exists():
        raise RuntimeError('Restore shutdown timed out; preserve the fixture for inspection')


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['launch', 'powerdown'])
    parser.add_argument('--pid', type=int)
    args = parser.parse_args()
    if args.command == 'launch':
        launch()
    elif args.pid and args.pid > 1:
        powerdown(args.pid)
    else:
        parser.error('powerdown requires --pid greater than one')


if __name__ == '__main__':
    main()
