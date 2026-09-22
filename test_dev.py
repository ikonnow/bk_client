"""Regression tests for local builds and release packaging.

Run with ``python -m unittest -v test_dev``.
"""

import argparse
import io
import json
import tempfile
import unittest
import zipfile
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

import dev

SUPPORTED_HOSTS = (
    ("Windows", "windows", "amd64", "bk_client-windows-x86_64.exe"),
    ("Windows", "windows", "arm64", "bk_client-windows-arm64.exe"),
    ("Darwin", "darwin", "amd64", "bk_client-macos-x86_64"),
    ("Darwin", "darwin", "arm64", "bk_client-macos-arm64"),
    ("Linux", "linux", "amd64", "bk_client-linux-x86_64"),
    ("Linux", "linux", "arm64", "bk_client-linux-arm64"),
)
ARCH_ALIASES = {
    "amd64": ("amd64", "AMD64", "x86_64", "X86_64"),
    "arm64": ("arm64", "ARM64", "aarch64", "AARCH64"),
}
UNSUPPORTED_HOSTS = (("FreeBSD", "x86_64"), ("Linux", "riscv64"), ("Windows", "i386"), ("Darwin", ""))


class HostBuildTargetTests(unittest.TestCase):
    """Check target selection and failure before output mutation."""

    def test_supported_targets_and_aliases(self):
        """Resolve every shipped target with canonical names and aliases."""
        for system, goos, goarch, filename in SUPPORTED_HOSTS:
            for machine in ARCH_ALIASES[goarch]:
                with (
                    self.subTest(system=system, machine=machine),
                    patch("dev.platform.system", return_value=system),
                    patch("dev.platform.machine", return_value=machine),
                ):
                    self.assertEqual(dev.get_host_build_target(), (goos, goarch, filename))

    def test_unsupported_hosts(self):
        """Report the original detected OS and architecture on failure."""
        for system, machine in UNSUPPORTED_HOSTS:
            with (
                self.subTest(system=system, machine=machine),
                patch("dev.platform.system", return_value=system),
                patch("dev.platform.machine", return_value=machine),
            ):
                with self.assertRaises(ValueError) as raised:
                    dev.get_host_build_target()
                self.assertIn(f"OS={system!r}", str(raised.exception))
                self.assertIn(f"architecture={machine!r}", str(raised.exception))

    def test_unsupported_build_preserves_output(self):
        """Leave existing artifacts intact and never create new output."""
        for system, machine in UNSUPPORTED_HOSTS:
            for existing in (False, True):
                with (
                    self.subTest(system=system, machine=machine, existing=existing),
                    tempfile.TemporaryDirectory() as temporary,
                ):
                    output = Path(temporary) / "out"
                    artifact = output / "v1.0.0" / "bk_client.zip"
                    if existing:
                        artifact.parent.mkdir(parents=True)
                        artifact.write_bytes(b"existing release")
                    with (
                        patch("dev.platform.system", return_value=system),
                        patch("dev.platform.machine", return_value=machine),
                        patch("dev.subprocess.Popen") as popen,
                        patch("dev.read_client_version", return_value="2.0.0"),
                    ):
                        with self.assertRaises(SystemExit) as raised:
                            dev.build(argparse.Namespace(out=str(output)))
                        self.assertIn("Unsupported build platform", str(raised.exception))
                        popen.assert_not_called()
                    if existing:
                        self.assertEqual(artifact.read_bytes(), b"existing release")
                        self.assertEqual(list(output.iterdir()), [artifact.parent])
                        self.assertEqual(list(artifact.parent.iterdir()), [artifact])
                    else:
                        self.assertFalse(output.exists())


class PackagingTests(unittest.TestCase):
    """Check package reporting and release completeness requirements."""

    def test_package_lists_included_binaries(self):
        """Report and archive one or multiple binaries without warnings."""
        for targets in (dev.BUILD_TARGETS[:1], dev.BUILD_TARGETS, dev.ALL_TARGETS):
            with self.subTest(targets=targets), tempfile.TemporaryDirectory() as temporary:
                for _, _, filename in targets:
                    (Path(temporary) / filename).write_bytes(b"test binary")
                output = io.StringIO()
                with redirect_stdout(output):
                    archive_path = dev.package(temporary, "test")
                names = [filename for _, _, filename in targets]
                self.assertNotIn("WARNING", output.getvalue())
                for name in names:
                    self.assertIn(f"  {name}\n", output.getvalue())
                with zipfile.ZipFile(archive_path) as archive:
                    manifest = json.loads(archive.read("manifest.json"))
                    self.assertEqual([entry["filename"] for entry in manifest["binaries"]], names)
                    for name in names:
                        self.assertEqual(archive.read(name), b"test binary")

    def test_release_rejects_missing_targets_before_output_changes(self):
        """Reject absent inputs even if stale output contains those binaries."""
        for missing_target in dev.BUILD_TARGETS:
            with self.subTest(target=missing_target), tempfile.TemporaryDirectory() as temporary:
                source = Path(temporary) / "binaries"
                source.mkdir()
                output = Path(temporary) / "out"
                version_dir = output / "vtest"
                version_dir.mkdir(parents=True)
                for target in dev.BUILD_TARGETS:
                    (version_dir / target[2]).write_bytes(b"old binary")
                    if target != missing_target:
                        (source / target[2]).write_bytes(b"new binary")
                # A directory named after a binary must not satisfy validation.
                (source / missing_target[2]).mkdir()
                with (
                    patch("dev.read_client_version", return_value="test"),
                    patch("dev.verify") as verify,
                    patch("dev.package") as package,
                ):
                    with self.assertRaises(SystemExit) as raised:
                        dev.release(argparse.Namespace(prebuilt_bin_dir=str(source), out=str(output)))
                    self.assertIn(missing_target[2], str(raised.exception))
                    verify.assert_not_called()
                    package.assert_not_called()
                for _, _, filename in dev.BUILD_TARGETS:
                    self.assertEqual((version_dir / filename).read_bytes(), b"old binary")

    def test_release_accepts_complete_inputs_with_optional_legacy(self):
        """Package complete releases with or without legacy binaries."""
        for targets in (dev.BUILD_TARGETS, dev.ALL_TARGETS):
            with self.subTest(targets=targets), tempfile.TemporaryDirectory() as temporary:
                source = Path(temporary) / "binaries"
                source.mkdir()
                output = Path(temporary) / "out"
                for _, _, filename in targets:
                    (source / filename).write_bytes(b"test binary")
                with (
                    patch("dev.read_client_version", return_value="test"),
                    patch("dev.verify") as verify,
                    redirect_stdout(io.StringIO()),
                ):
                    dev.release(argparse.Namespace(prebuilt_bin_dir=str(source), out=str(output)))
                    verify.assert_called_once_with(str(output / "vtest"))
                with zipfile.ZipFile(output / "vtest" / "bk_client.zip") as archive:
                    manifest = json.loads(archive.read("manifest.json"))
                    self.assertEqual(
                        [entry["filename"] for entry in manifest["binaries"]],
                        [filename for _, _, filename in targets],
                    )


if __name__ == "__main__":
    unittest.main()
