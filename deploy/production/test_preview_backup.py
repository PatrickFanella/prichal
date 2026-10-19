import importlib.util
import json
import os
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("preview_backup", Path(__file__).with_name("preview-backup.py"))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


class PreviewBackupTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.root.chmod(0o700)

    def pair(self, number, verified=True):
        name = f"preview-20261019T{number:012d}Z-aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa.dump"
        path = self.root / name
        path.write_bytes(b"private fixture dump")
        path.chmod(0o600)
        with path.open("rb") as stream:
            checksum = backup.digest(stream)
        value = {"archive": name, "format": 1, "sha256": checksum, "bytes": path.stat().st_size, "schemaVersion": 35, "verifiedAt": "2026-10-19T01:00:00Z" if verified else None}
        backup.publish_json(self.root / (name + ".json"), value)
        return name

    def test_private_files_lock_and_paths(self):
        with backup.locked(self.root):
            with self.assertRaises(backup.Refused):
                with backup.locked(self.root):
                    pass
        self.root.chmod(0o755)
        with self.assertRaises(backup.Refused):
            backup.private_root(self.root)
        self.root.chmod(0o700)
        link = self.root / "linked"
        link.symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(backup.Refused):
            backup.private_root(link)
        name = self.pair(1)
        path = self.root / name
        path.chmod(0o644)
        with self.assertRaises(backup.Refused):
            backup.private_file(path)
        path.chmod(0o600)
        hard = self.root / "hard"
        os.link(path, hard)
        with self.assertRaises(backup.Refused):
            backup.private_file(path)
        with self.assertRaises(backup.Refused):
            backup.read_receipt(self.root, "../outside.dump")

    def test_restore_rejects_tampering_before_docker(self):
        name = self.pair(1)
        (self.root / name).write_bytes(b"tampered")
        with patch.object(backup, "start_fixture") as start:
            with self.assertRaises(backup.Refused):
                backup.restore(self.root, name)
            start.assert_not_called()

    def test_restore_failure_stops_only_allocated_fixture(self):
        name = self.pair(1)
        with patch.object(backup, "start_fixture", return_value="allocated") as start, patch.object(backup, "wait_ready"), patch.object(backup, "command", side_effect=backup.Refused("restore failure")), patch.object(backup, "stop_fixture") as stop:
            with self.assertRaises(backup.Refused):
                backup.restore(self.root, name)
            start.assert_called_once_with("restore")
            stop.assert_called_once_with("allocated")

    def test_retention_preserves_other_files_and_requires_verified_latest(self):
        names = [self.pair(i) for i in range(17)]
        legacy = self.root / "before-release.sql"
        legacy.write_bytes(b"retained legacy snapshot")
        partial = self.root / ".partial-interrupted"
        partial.write_bytes(b"retained interrupted run")
        backup.retain(self.root, names[-1])
        self.assertEqual(sorted(p.name for p in self.root.glob("*.dump")), names[-14:])
        self.assertTrue(legacy.exists())
        self.assertTrue(partial.exists())
        newest = self.pair(18, verified=False)
        before = sorted(p.name for p in self.root.iterdir())
        with self.assertRaises(backup.Refused):
            backup.retain(self.root, newest)
        self.assertEqual(before, sorted(p.name for p in self.root.iterdir()))

    def test_retention_validates_all_pairs_before_any_deletion(self):
        names = [self.pair(i) for i in range(16)]
        (self.root / names[0]).write_bytes(b"private fixture dumo")  # Same byte length.
        before = sorted(p.name for p in self.root.iterdir())
        with self.assertRaises(backup.Refused):
            backup.retain(self.root, names[-1])
        self.assertEqual(before, sorted(p.name for p in self.root.iterdir()))

    def test_source_and_stop_scope(self):
        value = {"Id": "fixed-id", "Config": {"Image": backup.IMAGE, "Labels": {"com.docker.compose.project": "prichal", "com.docker.compose.service": "postgres"}}, "HostConfig": {"PortBindings": {}}, "State": {"Running": True}, "Mounts": [{"Type": "volume", "Name": "prichal_management-data", "Destination": "/var/lib/postgresql/data"}]}
        with patch.object(backup, "inspect", return_value=value):
            self.assertEqual(backup.source_identity(backup.SOURCE), "fixed-id")
            with self.assertRaises(backup.Refused):
                backup.source_identity("unrelated-database")
            with patch.object(backup, "command") as command:
                with self.assertRaises(backup.Refused):
                    backup.stop_fixture(backup.SOURCE)
                command.assert_not_called()
            value["Config"]["Image"] = "postgres:latest"
            with self.assertRaises(backup.Refused):
                backup.source_identity(backup.SOURCE)

    def test_dump_failure_never_publishes_or_prunes(self):
        old = self.pair(1)
        with patch.object(backup, "source_identity", return_value="fixed-id"), patch.object(backup, "schema", return_value=35), patch.object(backup, "command", side_effect=backup.Refused("dump failed")), patch.object(backup, "retain") as retain:
            with self.assertRaises(backup.Refused):
                backup.backup(self.root)
            retain.assert_not_called()
        self.assertEqual(list(self.root.glob("*.dump")), [self.root / old])
        self.assertEqual(list(self.root.glob(".partial-*")), [])

    def test_receipt_metadata_is_bounded_and_exact(self):
        name = self.pair(1)
        path = self.root / (name + ".json")
        value = json.loads(path.read_text())
        path.write_text("[]")
        with self.assertRaises(backup.Refused):
            backup.read_receipt(self.root, name)
        value["schemaVersion"] = True
        path.write_text(json.dumps(value))
        with self.assertRaises(backup.Refused):
            backup.read_receipt(self.root, name)

    def test_failed_restore_never_marks_backup_verified_or_prunes(self):
        previous = self.pair(1)

        def dump(_args, **kwargs):
            kwargs["stdout"].write(b"disposable archive")

        with patch.object(backup, "source_identity", return_value="fixed-id"), patch.object(backup, "schema", return_value=36), patch.object(backup, "command", side_effect=dump), patch.object(backup, "restore", side_effect=backup.Refused("verification failed")), patch.object(backup, "retain") as retain:
            with self.assertRaises(backup.Refused):
                backup.backup(self.root)
            retain.assert_not_called()
        names = sorted(path.name for path in self.root.glob("*.dump"))
        self.assertEqual(len(names), 2)
        self.assertIn(previous, names)
        current = next(name for name in names if name != previous)
        self.assertIsNone(backup.read_receipt(self.root, current)["verifiedAt"])

    def test_allocation_failure_attempts_scoped_cleanup(self):
        with patch.object(backup, "command", side_effect=backup.Refused("allocation failed")), patch.object(backup, "stop_fixture") as stop:
            with self.assertRaises(backup.Refused):
                backup.start_fixture("restore")
            self.assertEqual(stop.call_count, 1)
            self.assertRegex(stop.call_args.args[0], backup.FIXTURE)

    def test_docker_context_and_credentials_cannot_redirect_host_access(self):
        with patch.dict(os.environ, {"DOCKER_HOST": "tcp://foreign.invalid:2375", "DOCKER_CONTEXT": "foreign", "DOCKER_CONFIG": "/private/credentials"}), patch.object(backup.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"safe")) as run:
            self.assertEqual(backup.command(["docker", "inspect", "fixed"], timeout=10), b"safe")
        args, kwargs = run.call_args.args[0], run.call_args.kwargs
        self.assertEqual(args[:2], ["docker", "--host=unix:///var/run/docker.sock"])
        self.assertEqual(args[2], "--config")
        self.assertNotEqual(args[3], "/private/credentials")
        self.assertFalse(any(key.startswith("DOCKER_") for key in kwargs["env"]))
        self.assertEqual(kwargs["timeout"], 10)

    def test_fifo_and_oversized_receipt_are_refused(self):
        name = self.pair(1)
        fifo = self.root / "fifo"
        os.mkfifo(fifo, 0o600)
        with self.assertRaises(backup.Refused):
            backup.private_file(fifo)
        (self.root / (name + ".json")).write_text(" " * 65537)
        with self.assertRaises(backup.Refused):
            backup.read_receipt(self.root, name)

    def test_failed_archive_allocation_is_bounded_without_deleting_evidence(self):
        for number in range(15):
            self.pair(number, verified=number < 14)
        before = sorted(path.name for path in self.root.iterdir())
        with patch.object(backup, "source_identity") as source:
            with self.assertRaises(backup.Refused):
                backup.backup(self.root)
            source.assert_not_called()
        self.assertEqual([name for name in sorted(path.name for path in self.root.iterdir()) if name != ".backup.lock"], before)

    def test_interrupted_partial_blocks_new_allocation_without_removal(self):
        partial = self.root / ".partial-interrupted"
        partial.write_bytes(b"retained evidence")
        with patch.object(backup, "source_identity") as source:
            with self.assertRaises(backup.Refused):
                backup.backup(self.root)
            source.assert_not_called()
        self.assertTrue(partial.exists())


if __name__ == "__main__":
    unittest.main()
