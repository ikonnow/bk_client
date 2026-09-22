"""Regression tests for local build platform detection.

Run with ``python -m unittest -v test_dev``.
"""

import argparse
import tempfile
import unittest
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


if __name__ == "__main__":
    unittest.main()
