#!/usr/bin/env python3
"""Stage certificates for the one disposable Dozor pilot; never activate them."""
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys

INSTANCE = "e8e82a65-7aa8-4e37-9306-0de701bf1b19"
PKI = Path("/etc/prichal/pilot/pki")
INGRESS = Path("/srv/apps/prichal/pilot/state") / (INSTANCE + "-tls.pem")


def command(*args):
    result = subprocess.run(["openssl", *map(str, args)], capture_output=True, check=False)
    if result.returncode:
        raise RuntimeError("Certificate operation failed: " + args[0])
    return result.stdout


def private(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise RuntimeError("Private operator-owned regular file required")
    if not 0 < info.st_size <= 16384:
        raise RuntimeError("Unexpected identity size")
    return path.read_bytes()


def public_key(path, certificate=False):
    if certificate:
        return command("x509", "-in", path, "-pubkey", "-noout")
    return command("pkey", "-in", path, "-pubout")


def write(path, value):
    with path.open("xb") as file:
        file.write(value)
        file.flush()
        os.fsync(file.fileno())
    path.chmod(0o600)


def require(condition):
    if not condition:
        raise RuntimeError("Certificate identity verification failed")


def main():
    if len(sys.argv) != 2:
        raise RuntimeError("Supply a new private absolute staging directory")
    target = Path(sys.argv[1])
    if not target.is_absolute():
        raise RuntimeError("Absolute staging directory required")
    parent = target.parent.lstat()
    if not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid() or parent.st_mode & 0o077:
        raise RuntimeError("Private operator-owned parent required")
    inputs = {name: PKI / name for name in ["ca.crt", "ca.key", "agent.crt", "agent.key", "worker.crt", "worker.key"]}
    inputs["ingress.pem"] = INGRESS
    original = {name: private(path) for name, path in inputs.items()}
    for role in ["ca", "agent", "worker"]:
        require(public_key(PKI / (role + ".crt"), True) == public_key(PKI / (role + ".key")))
    require(public_key(INGRESS, True) == public_key(INGRESS))
    require(command("x509", "-in", PKI / "ca.crt", "-noout", "-subject").strip() == b"subject=CN=prichal-dozor-pilot-ca")
    for role, purpose in [("agent", "sslserver"), ("worker", "sslclient")]:
        command("verify", "-CAfile", PKI / "ca.crt", "-purpose", purpose, PKI / (role + ".crt"))
        require(command("x509", "-in", PKI / (role + ".crt"), "-noout", "-subject").strip() == ("subject=CN=pds-ass-" + role).encode())
    command("verify", "-CAfile", INGRESS, "-verify_hostname", INSTANCE + ".localhost", INGRESS)
    target.mkdir(mode=0o700)  # Refuse existing paths, including symlinks.
    before = target / "before"
    before.mkdir(mode=0o700)
    for name, value in original.items():
        write(before / name, value)
    command("req", "-x509", "-new", "-key", PKI / "ca.key", "-days", "90", "-subj", "/CN=prichal-dozor-pilot-ca", "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-out", target / "ca.crt")
    for role, usage in [("agent", "serverAuth"), ("worker", "clientAuth")]:
        command("req", "-new", "-key", PKI / (role + ".key"), "-subj", "/CN=pds-ass-" + role, "-out", target / (role + ".csr"))
        extensions = "basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=" + usage + "\n"
        if role == "agent":
            extensions += "subjectAltName=IP:127.0.0.1,DNS:localhost\n"
        write(target / (role + ".cnf"), extensions.encode())
        command("x509", "-req", "-in", target / (role + ".csr"), "-CA", target / "ca.crt", "-CAkey", PKI / "ca.key", "-set_serial", "0x" + os.urandom(16).hex(), "-days", "7", "-extfile", target / (role + ".cnf"), "-out", target / (role + ".crt"))
    command("pkey", "-in", INGRESS, "-out", target / "ingress.key")
    command("req", "-x509", "-new", "-key", target / "ingress.key", "-days", "7", "-subj", "/CN=" + INSTANCE + ".localhost", "-addext", "basicConstraints=critical,CA:FALSE", "-addext", "keyUsage=critical,digitalSignature", "-addext", "extendedKeyUsage=serverAuth", "-addext", "subjectAltName=DNS:" + INSTANCE + ".localhost", "-out", target / "ingress.crt")
    write(target / "ingress.pem", (target / "ingress.crt").read_bytes() + (target / "ingress.key").read_bytes())
    for role, purpose in [("agent", "sslserver"), ("worker", "sslclient")]:
        command("verify", "-CAfile", target / "ca.crt", "-purpose", purpose, target / (role + ".crt"))
        require(public_key(target / (role + ".crt"), True) == public_key(PKI / (role + ".key")))
    command("verify", "-CAfile", target / "ingress.crt", "-purpose", "sslserver", "-verify_hostname", INSTANCE + ".localhost", target / "ingress.crt")
    require(public_key(target / "ca.crt", True) == public_key(PKI / "ca.key"))
    require(public_key(target / "ingress.pem", True) == public_key(INGRESS))
    require(all(private(path) == original[name] for name, path in inputs.items()))
    for path in target.iterdir():
        if path.is_file():
            path.chmod(0o600)
    report = {"instanceId": INSTANCE, "stagedOnly": True, "existingKeysRetained": True,
              "sourceHashes": {name: hashlib.sha256(value).hexdigest() for name, value in original.items()},
              "newCertificateDates": {name: command("x509", "-in", target / name, "-noout", "-dates").decode().strip() for name in ["ca.crt", "agent.crt", "worker.crt", "ingress.crt"]}}
    write(target / "staging-report.json", json.dumps(report, indent=2).encode())
    print(json.dumps({key: report[key] for key in ["instanceId", "stagedOnly", "existingKeysRetained", "newCertificateDates"]}))


if __name__ == "__main__":
    os.umask(0o077)
    try:
        main()
    except Exception:
        print("Pilot certificate staging failed; live files were not modified. Retain private staging evidence.", file=sys.stderr)
        sys.exit(1)
