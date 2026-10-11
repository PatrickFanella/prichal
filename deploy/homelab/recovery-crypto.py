#!/usr/bin/env python3
"""Authenticated staging VM archives; publish plaintext only after verification."""
import argparse
import hashlib
import os
from pathlib import Path
import stat
import tempfile
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

MAGIC = b'PRICHAL-STAGING-VM-1\n'


def private_key(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Key must be an owned private regular file')
    key = path.read_bytes()
    if len(key) != 32:
        raise ValueError('Recovery key must contain exactly 32 bytes')
    return key


def transform(action, source, destination, key_path, expected_sha=None):
    key = private_key(key_path)
    if destination.exists() or destination.is_symlink():
        raise ValueError('Destination exists; preserve it')
    parent = destination.parent
    info = parent.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Destination parent must be an owned private directory')
    source_info = source.lstat()
    if not stat.S_ISREG(source_info.st_mode):
        raise ValueError('Source must be a regular file')
    fd, temporary = tempfile.mkstemp(prefix='.pending-', dir=parent)
    temporary = Path(temporary)
    digest = hashlib.sha256()
    try:
        with source.open('rb') as incoming, os.fdopen(fd, 'wb') as outgoing:
            if action == 'encrypt':
                nonce = os.urandom(12)
                cipher = Cipher(algorithms.AES(key), modes.GCM(nonce)).encryptor()
                cipher.authenticate_additional_data(MAGIC)
                outgoing.write(MAGIC + nonce)
                while chunk := incoming.read(1024 * 1024):
                    digest.update(chunk)
                    outgoing.write(cipher.update(chunk))
                outgoing.write(cipher.finalize())
                outgoing.write(cipher.tag)
            else:
                if incoming.read(len(MAGIC)) != MAGIC:
                    raise ValueError('Archive scope/version mismatch')
                nonce = incoming.read(12)
                incoming.seek(-16, os.SEEK_END)
                tag = incoming.read(16)
                incoming.seek(len(MAGIC) + 12)
                remaining = source_info.st_size - len(MAGIC) - 12 - 16
                if remaining < 0:
                    raise ValueError('Truncated archive')
                cipher = Cipher(algorithms.AES(key), modes.GCM(nonce, tag)).decryptor()
                cipher.authenticate_additional_data(MAGIC)
                while remaining:
                    chunk = incoming.read(min(remaining, 1024 * 1024))
                    if not chunk:
                        raise ValueError('Truncated archive')
                    remaining -= len(chunk)
                    plain = cipher.update(chunk)
                    digest.update(plain)
                    outgoing.write(plain)
                plain = cipher.finalize()
                digest.update(plain)
                outgoing.write(plain)
                if not expected_sha or digest.hexdigest() != expected_sha:
                    raise ValueError('Restored archive digest mismatch')
            outgoing.flush()
            os.fsync(outgoing.fileno())
        # Exclusive publication prevents a concurrent destination replacement.
        os.link(temporary, destination)
        directory = os.open(parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
        return digest.hexdigest()
    finally:
        temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['encrypt', 'decrypt'])
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--destination', type=Path, required=True)
    parser.add_argument('--key', type=Path, required=True)
    parser.add_argument('--expected-sha256')
    args = parser.parse_args()
    print(transform(args.action, args.source, args.destination, args.key, args.expected_sha256))


if __name__ == '__main__':
    main()
