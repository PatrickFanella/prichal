#!/usr/bin/env python3
"""Private, fixed-scope preview PostgreSQL backups and offline restore checks."""
import argparse
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import resource
import signal
import stat
import subprocess
import sys
import tempfile
import time
import uuid

IMAGE = "postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73"
ROOT = Path("/srv/apps/prichal/backups")
SOURCE = "prichal-postgres-1"
LABEL = "tv.subcult.prichal.preview-backup-qualification"
NAME = re.compile(r"^preview-[0-9]{8}T[0-9]{12}Z-[a-f0-9-]{36}\.dump$")
FIXTURE = re.compile(r"^prichal-preview-(source|restore)-[a-f0-9-]{36}$")
MAX_DUMP = 256 << 20
KEEP = 14


class Refused(RuntimeError):
    pass


def command(args, *, stdin=None, stdout=subprocess.PIPE, timeout=180, limited=False):
    def limit():
        resource.setrlimit(resource.RLIMIT_FSIZE, (MAX_DUMP, MAX_DUMP))
    try:
        # Ignore the caller's Docker context, remote host and registry credentials.
        environment = {key: value for key, value in os.environ.items() if not key.startswith("DOCKER_")}
        with tempfile.TemporaryDirectory(prefix="prichal-preview-docker-") as config:
            fixed = ["docker", "--host=unix:///var/run/docker.sock", "--config", config, *args[1:]]
            result = subprocess.run(fixed, stdin=stdin, stdout=stdout, stderr=subprocess.DEVNULL,
                                    env=environment, timeout=timeout, check=False, preexec_fn=limit if limited else None)
    except (OSError, subprocess.TimeoutExpired):
        raise Refused("Docker operation unavailable or timed out") from None
    if result.returncode:
        raise Refused("Docker operation failed; private database output was suppressed")
    return result.stdout


def inspect(name):
    try:
        return json.loads(command(["docker", "inspect", name], timeout=10))[0]
    except (ValueError, IndexError, TypeError):
        raise Refused("Container identity could not be verified") from None


def source_identity(name):
    value = inspect(name)
    config, host = value.get("Config", {}), value.get("HostConfig", {})
    labels = config.get("Labels") or {}
    production = name == SOURCE and labels.get("com.docker.compose.project") == "prichal" and labels.get("com.docker.compose.service") == "postgres"
    fixture = bool(FIXTURE.fullmatch(name)) and name.startswith("prichal-preview-source-") and labels.get(LABEL) == name and host.get("NetworkMode") == "none" and not value.get("Mounts")
    mounts = value.get("Mounts", [])
    if production:
        production = len(mounts) == 1 and mounts[0].get("Type") == "volume" and mounts[0].get("Name") == "prichal_management-data" and mounts[0].get("Destination") == "/var/lib/postgresql/data"
    if not (production or fixture) or config.get("Image") != IMAGE or not value.get("State", {}).get("Running") or host.get("PortBindings"):
        raise Refused("Refusing a container outside the fixed preview database policy")
    return value["Id"]


def sql(container, statement):
    return command(["docker", "exec", "-e", "PGOPTIONS=-c statement_timeout=30000", container,
                    "psql", "-XqAt", "-v", "ON_ERROR_STOP=1", "-U", "prichal", "-d", "prichal", "-c", statement], timeout=40).decode().strip()


def schema(container):
    value = sql(container, "SELECT version FROM schema_version WHERE singleton=true")
    if not re.fullmatch(r"[1-9][0-9]{0,5}", value):
        raise Refused("A single management schema version is required")
    return int(value)


def private_root(root):
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    value = root.lstat()
    if root.resolve() != root.absolute() or not stat.S_ISDIR(value.st_mode) or stat.S_IMODE(value.st_mode) != 0o700 or value.st_uid != os.geteuid():
        raise Refused("Backup directory must be a private owned directory without symlink parents")


@contextlib.contextmanager
def locked(root):
    private_root(root)
    fd = os.open(root / ".backup.lock", os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        value = os.fstat(fd)
        if not stat.S_ISREG(value.st_mode) or value.st_nlink != 1 or stat.S_IMODE(value.st_mode) != 0o600 or value.st_uid != os.geteuid():
            raise Refused("Unsafe backup lock")
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise Refused("A preview backup or verification is already running") from None
        yield
    finally:
        os.close(fd)


def private_file(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    value = os.fstat(fd)
    if not stat.S_ISREG(value.st_mode) or value.st_nlink != 1 or value.st_uid != os.geteuid() or stat.S_IMODE(value.st_mode) != 0o600:
        os.close(fd)
        raise Refused("Unsafe backup file")
    return os.fdopen(fd, "rb")


def digest(stream):
    stream.seek(0)
    value = hashlib.file_digest(stream, "sha256").hexdigest()
    stream.seek(0)
    return value


def publish_json(path, value):
    temporary = path.with_name(".receipt-" + str(uuid.uuid4()))
    try:
        with temporary.open("xb") as stream:
            os.chmod(temporary, 0o600)
            stream.write(json.dumps(value, sort_keys=True, indent=2).encode() + b"\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        temporary.unlink(missing_ok=True)


def read_receipt(root, name):
    if not NAME.fullmatch(name):
        raise Refused("Use an exact generated preview dump filename")
    with private_file(root / (name + ".json")) as stream:
        payload = stream.read(65537)
        if len(payload) > 65536:
            raise Refused("Oversized backup receipt")
        value = json.loads(payload)
    if not isinstance(value, dict):
        raise Refused("Backup receipt must be an object")
    if value.get("archive") != name or value.get("format") != 1 or not re.fullmatch(r"[a-f0-9]{64}", str(value.get("sha256"))) or type(value.get("schemaVersion")) is not int or not 1 <= value["schemaVersion"] <= 999999 or type(value.get("bytes")) is not int or not 1 <= value["bytes"] <= MAX_DUMP:
        raise Refused("Backup receipt could not be validated")
    return value


def start_fixture(kind):
    name = "prichal-preview-" + kind + "-" + str(uuid.uuid4())
    try:
        command(["docker", "run", "--pull=never", "-d", "--name", name, "--label", LABEL + "=" + name,
                 "--network", "none", "--read-only", "--user", "70:70", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                 "--memory", "768m", "--cpus", "1", "--pids-limit", "128",
                 "--tmpfs", "/var/lib/postgresql/data:rw,nosuid,noexec,size=512m,uid=70,gid=70,mode=0700",
                 "--tmpfs", "/var/run/postgresql:rw,nosuid,noexec,size=16m,uid=70,gid=70,mode=0770",
                 "--tmpfs", "/tmp:rw,nosuid,noexec,size=16m,uid=70,gid=70",
                 "-e", "POSTGRES_USER=prichal", "-e", "POSTGRES_DB=prichal", "-e", "POSTGRES_HOST_AUTH_METHOD=trust", IMAGE], timeout=30)
    except BaseException:
        try:
            stop_fixture(name)
        except Refused:
            raise Refused(f"Fixture allocation failed; inspect exact disposable container {name}") from None
        raise
    return name


def stop_fixture(name):
    value = inspect(name)
    if not FIXTURE.fullmatch(name) or (value.get("Config", {}).get("Labels") or {}).get(LABEL) != name or value.get("HostConfig", {}).get("NetworkMode") != "none":
        raise Refused("Refusing to stop an unowned verification container")
    command(["docker", "stop", "--time", "5", value["Id"]], timeout=15)
    if inspect(value["Id"]).get("State", {}).get("Running"):
        raise Refused("Verification container did not stop")


def wait_ready(name):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            if sql(name, "SELECT 1") == "1":
                return
        except Refused:
            time.sleep(0.25)
    raise Refused("Disposable PostgreSQL did not become ready")


def restore(root, name, *, canary=False):
    receipt = read_receipt(root, name)
    with private_file(root / name) as archive:
        if os.fstat(archive.fileno()).st_size != receipt["bytes"] or digest(archive) != receipt["sha256"]:
            raise Refused("Backup checksum or size mismatch; no restore was started")
        target = start_fixture("restore")
        try:
            wait_ready(target)
            command(["docker", "exec", "-i", target, "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "-U", "prichal", "-d", "prichal"], stdin=archive, stdout=subprocess.DEVNULL)
            if schema(target) != receipt["schemaVersion"]:
                raise Refused("Restored management schema does not match the archive receipt")
            if canary and sql(target, "SELECT count(*) FROM organizations o JOIN owners a ON a.id=o.owner_id WHERE a.issuer='https://fixture.invalid' AND a.subject='backup-qualification'") != "1":
                raise Refused("Disposable restored owner/organization relationship differs")
            # No rows or identities are returned; this is a structural inventory.
            if canary and sql(target, "SELECT value || ':' || nextval(pg_get_serial_sequence('backup_canary','id'))::text FROM backup_canary WHERE id=1") != "disposable preview canary:2":
                raise Refused("Disposable restored row or sequence differs")
            tables = int(sql(target, "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN ('public','river') AND c.relkind IN ('r','p')"))
        finally:
            stop_fixture(target)
    return {"container": target, "schemaVersion": receipt["schemaVersion"], "tables": tables, "stopped": True}


def retain(root, latest):
    # Validate every generated pair before deleting anything. Other files, old
    # hand-made snapshots and interrupted partials are never deletion targets.
    candidates = sorted(p.name for p in root.iterdir() if NAME.fullmatch(p.name))
    receipts = {name: read_receipt(root, name) for name in candidates}
    if latest not in receipts or not receipts[latest].get("verifiedAt") or latest != max(candidates):
        raise Refused("Retention requires the newest complete backup to be verified")
    for name in candidates:
        with private_file(root / name) as stream:
            if os.fstat(stream.fileno()).st_size != receipts[name]["bytes"] or digest(stream) != receipts[name]["sha256"]:
                raise Refused("Retention refused an inconsistent archive pair")
    for name in candidates[:-KEEP]:
        (root / (name + ".json")).unlink()
        (root / name).unlink()


def backup(root, source=SOURCE):
    with locked(root):
        entries = list(root.iterdir())
        archives = [path for path in entries if NAME.fullmatch(path.name)]
        if len(archives) >= KEEP + 1 or any(path.name.startswith((".partial-", ".receipt-")) for path in entries):
            raise Refused("Retained failed/interrupted backups need review before allocating another archive")
        for path in archives:
            read_receipt(root, path.name)
        identity = source_identity(source)
        version = schema(identity)
        name = "preview-" + dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ") + "-" + str(uuid.uuid4()) + ".dump"
        temporary = root / (".partial-" + str(uuid.uuid4()))
        try:
            with temporary.open("xb") as output:
                os.chmod(temporary, 0o600)
                command(["docker", "exec", "-e", "PGOPTIONS=-c statement_timeout=120000", identity, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "-U", "prichal", "-d", "prichal"], stdout=output, limited=True)
                output.flush()
                os.fsync(output.fileno())
            with private_file(temporary) as stream:
                checksum, size = digest(stream), os.fstat(stream.fileno()).st_size
            if not 0 < size <= MAX_DUMP:
                raise Refused("Empty or oversized preview backup")
            temporary.rename(root / name)
            receipt = {"format": 1, "archive": name, "sha256": checksum, "bytes": size, "schemaVersion": version, "sourceContainerID": identity, "verifiedAt": None}
            publish_json(root / (name + ".json"), receipt)
            result = restore(root, name, canary=source != SOURCE)
            receipt["verifiedAt"] = dt.datetime.now(dt.timezone.utc).isoformat()
            receipt["restore"] = result
            publish_json(root / (name + ".json"), receipt)
            retain(root, name)
            return receipt
        finally:
            temporary.unlink(missing_ok=True)


def qualify():
    root = Path(__file__).resolve().parents[2] / ".local" / "preview-backups" / str(uuid.uuid4())
    migrations = sorted((Path(__file__).resolve().parents[2] / "backend/internal/migrate/sql").glob("[0-9][0-9][0-9]_*.sql"))
    if not migrations:
        raise Refused("Run qualification from a complete repository checkout")
    source = start_fixture("source")
    try:
        wait_ready(source)
        sql(source, "CREATE TABLE schema_version(singleton boolean PRIMARY KEY,version integer NOT NULL); INSERT INTO schema_version VALUES(true,0)")
        for number, migration in enumerate(migrations, 1):
            if int(migration.name[:3]) != number:
                raise Refused("Qualification migrations must be contiguous")
            sql(source, "BEGIN;" + migration.read_text() + f"; UPDATE schema_version SET version={number}; COMMIT;")
        sql(source, "CREATE TABLE backup_canary(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, value text NOT NULL); INSERT INTO backup_canary(value) VALUES('disposable preview canary'); INSERT INTO owners(issuer,subject,display_name) VALUES('https://fixture.invalid','backup-qualification','Synthetic backup owner'); INSERT INTO organizations(id,name,owner_id) SELECT gen_random_uuid(),'Backup qualification',id FROM owners;")
        receipt = backup(root, source)
        return {"root": str(root), "archive": receipt["archive"], "verified": bool(receipt["verifiedAt"]), "schemaVersion": receipt["schemaVersion"], "restore": receipt["restore"]}
    finally:
        stop_fixture(source)


def main():
    def interrupted(_signum, _frame):
        raise Refused("Backup interrupted")
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["backup", "verify", "qualify"])
    parser.add_argument("archive", nargs="?")
    args = parser.parse_args()
    if args.action != "verify" and args.archive:
        parser.error("Only verify accepts an archive filename")
    try:
        if args.action == "backup":
            result = backup(ROOT)
        elif args.action == "qualify":
            result = qualify()
        else:
            if not args.archive:
                parser.error("verify requires an exact generated archive filename")
            with locked(ROOT):
                result = restore(ROOT, args.archive)
        print(json.dumps(result, sort_keys=True))
        return 0
    except Refused as error:
        print(str(error), file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError):
        print("Preview backup/verification failed; no success is recorded. Inspect private artifacts and container state before retrying.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
