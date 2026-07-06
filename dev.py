# ##### BEGIN GPL LICENSE BLOCK #####
#
#  This program is free software; you can redistribute it and/or
#  modify it under the terms of the GNU General Public License
#  as published by the Free Software Foundation; either version 2
#  of the License, or (at your option) any later version.
#
#  This program is distributed in the hope that it will be useful,
#  but WITHOUT ANY WARRANTY; without even the implied warranty of
#  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#  GNU General Public License for more details.
#
#  You should have received a copy of the GNU General Public License
#  along with this program; if not, write to the Free Software Foundation,
#  Inc., 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA.
#
# ##### END GPL LICENSE BLOCK #####
# type: ignore

"""Developer helper for the standalone Blendkit-Client repository.

This script builds, verifies, tests and lints the Go Client and its bundled
Python recipes (client/tools). It is intentionally self-contained: the parent
add-on repositories (blendkit_addon, bk_maya, blendkit_rhino) keep their own
dev.py for packaging the Client into their distributions.

Commands:
    build   Cross-compile the Client for all supported platforms.
    run     Build for the current platform and run the Client standalone.
    live    Run live integration tests against a real server (creds from .env).
    verify  Verify code-signing/notarization of built Client binaries.
    test    Run Go unit tests and lint the Python recipes.
    lint    Lint Python recipes with ruff and pydoclint (no changes written).
    format  Format and auto-fix Python recipes with ruff.
    docs    Regenerate the API documentation (go generate).
"""

import argparse
import os
import shutil
import subprocess
import sys

CLIENT_DIR = "client"
TOOLS_DIR = os.path.join(CLIENT_DIR, "tools")

# Maps keys found in the local .env file to the environment variables the Client
# and its live tests understand. The .env file is gitignored and intended to hold
# a *dedicated test account's* key pointed at the devel server.
ENV_FILE = ".env"
DEVEL_SERVER = "https://devel.blendkit.com"


def load_dotenv(path: str = ENV_FILE) -> dict:
    """Parse a simple KEY=VALUE .env file.

    Blank lines and lines starting with ``#`` are ignored. Values are taken
    verbatim (no quote stripping beyond surrounding single/double quotes). This
    avoids a third-party dependency for the handful of keys we need.

    Args:
        path: Path to the .env file.

    Returns:
        A dict of the parsed keys, or an empty dict if the file is missing.
    """
    values: dict = {}
    if not os.path.isfile(path):
        return values
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, _, value = line.partition("=")
            value = value.strip().strip('"').strip("'")
            values[key.strip()] = value
    return values


def live_env() -> dict:
    """Build an environment for live testing/running from the local .env file.

    Reads the gitignored .env and exposes its key/server under the names the Go
    code expects (BLENDKIT_API_KEY, BLENDKIT_SERVER), defaulting the server
    to the devel instance so live work never targets production by accident.

    Returns:
        A copy of os.environ overlaid with the live-test variables.
    """
    dotenv = load_dotenv()
    env = {**os.environ}
    api_key = dotenv.get("API_KEY") or dotenv.get("BLENDKIT_API_KEY")
    server = dotenv.get("BLENDKIT_SERVER") or DEVEL_SERVER
    if api_key:
        env["BLENDKIT_API_KEY"] = api_key
    env["BLENDKIT_SERVER"] = server
    return env


def _run_py_tool(tool: str, tool_args: list[str]) -> int:
    """Run a Python dev tool (ruff, pydoclint), preferring uv.

    The dev tools are not required to be installed globally. When ``uv`` is
    available the tool is run via ``uvx`` (ephemeral, no global install);
    otherwise it falls back to invoking the tool directly from PATH.

    Args:
        tool: The tool name, e.g. "ruff" or "pydoclint".
        tool_args: Arguments passed to the tool.

    Returns:
        The tool's process exit code (0 on success).
    """
    if shutil.which("uv"):
        return subprocess.call(["uv", "tool", "run", tool, *tool_args])
    if shutil.which(tool):
        return subprocess.call([tool, *tool_args])
    print(
        f"error: neither 'uv' nor '{tool}' found on PATH; "
        f"install uv (https://docs.astral.sh/uv/) or '{tool}' to lint Python recipes",
    )
    return 1


# (GOOS, GOARCH, output filename) for every shipped platform.
BUILD_TARGETS = [
    ("windows", "amd64", "blenderkit-client-windows-x86_64.exe"),
    ("windows", "arm64", "blenderkit-client-windows-arm64.exe"),
    ("darwin", "amd64", "blenderkit-client-macos-x86_64"),
    ("darwin", "arm64", "blenderkit-client-macos-arm64"),
    ("linux", "amd64", "blenderkit-client-linux-x86_64"),
    ("linux", "arm64", "blenderkit-client-linux-arm64"),
]


def _kill_existing_dev_clients(binary: str) -> None:
    """Terminate dev Client processes left over from a previous ``run``.

    During development the freshly built binary should replace any still-running
    instance (the opposite of the Client's production single-instance guard,
    which keeps the original). On Windows a running .exe also locks the file, so
    this must happen before the rebuild. Failures are ignored.

    Args:
        binary: The dev binary file name to terminate.
    """
    if os.name == "nt":
        subprocess.run(["taskkill", "/F", "/IM", binary], capture_output=True, check=False)
    else:
        subprocess.run(["pkill", "-f", binary], capture_output=True, check=False)


def read_client_version() -> str:
    """Read the Client version from client/VERSION.

    Returns:
        The version string, e.g. "1.9.0".
    """
    with open(os.path.join(CLIENT_DIR, "VERSION")) as f:
        return f.read().strip()


def build(args):
    """Cross-compile the Client for all supported platforms.

    Binaries are written to ``<out>/v<version>/`` so the directory name matches
    the format expected by the add-on repos' ``copy_client_binaries`` step.

    Attributes:
        args: Parsed CLI arguments. Uses ``args.out`` as the output directory.
    """
    version = read_client_version()
    out_dir = os.path.abspath(os.path.join(args.out, f"v{version}"))
    os.makedirs(out_dir, exist_ok=True)
    ldflags = f"-X main.ClientVersion={version}"

    processes = []
    for goos, goarch, output in BUILD_TARGETS:
        build_path = os.path.join(out_dir, output)
        env = {**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"}
        proc = subprocess.Popen(
            ["go", "build", "-o", build_path, "-ldflags", ldflags, "."],
            env=env,
            cwd=CLIENT_DIR,
        )
        processes.append(((goos, goarch), proc))

    print(f"Blendkit-Client v{version} build started for {len(processes)} platforms.")
    builds_ok = True
    for target, proc in processes:
        proc.wait()
        if proc.returncode != 0:
            print(f"Client build {target} failed")
            builds_ok = False

    if not builds_ok:
        sys.exit(1)
    print(f"Blendkit-Client v{version} builds completed in {out_dir}.")


def run(args):
    """Build the Client for the current platform and run it standalone.

    Compiles a development binary into ``client/`` (gitignored) with the real
    version baked in, then launches it. Because no add-on version is passed, the
    Client runs in standalone mode: a system tray icon (Windows), a per-version
    single-instance guard, and no inactivity auto-shutdown.

    The server is taken from .env (BLENDKIT_SERVER) when set, so pointing the
    Client at the devel server is just a matter of editing .env. Any arguments
    after ``--`` are forwarded to the Client, e.g. ``python dev.py run -- --port 62000``.

    Args:
        args: Parsed CLI arguments. Uses ``args.client_args`` as the list of
            extra arguments forwarded to the Client.
    """
    version = read_client_version()
    binary = "blenderkit-client-dev.exe" if os.name == "nt" else "blenderkit-client-dev"
    binary_path = os.path.join(CLIENT_DIR, binary)
    ldflags = f"-X main.ClientVersion={version}"

    # A running instance locks the binary on Windows and would also win the
    # single-instance guard; replace it with the fresh dev build.
    _kill_existing_dev_clients(binary)

    print(f"=== Building Blendkit-Client v{version} for the current platform ===")
    build_proc = subprocess.Popen(
        ["go", "build", "-o", binary, "-ldflags", ldflags, "."],
        cwd=CLIENT_DIR,
    )
    build_proc.wait()
    if build_proc.returncode != 0:
        sys.exit(1)

    # Expose the server (and key, for parity) from .env to the Client via the
    # environment; the Client reads BLENDKIT_SERVER when --server is not given.
    env = {**os.environ}
    dotenv = load_dotenv()
    server = dotenv.get("BLENDKIT_SERVER")
    if server:
        env["BLENDKIT_SERVER"] = server
        print(f"=== Using server from .env: {server} ===")
    api_key = dotenv.get("API_KEY") or dotenv.get("BLENDKIT_API_KEY")
    if api_key:
        env["BLENDKIT_API_KEY"] = api_key

    print(f"=== Running {binary_path} (Ctrl+C to stop) ===")
    run_proc = subprocess.Popen([binary_path, *args.client_args], env=env)
    try:
        run_proc.wait()
    except KeyboardInterrupt:
        run_proc.terminate()
        run_proc.wait()
    sys.exit(run_proc.returncode or 0)


def live(args):
    """Run the Go live integration tests against a real Blendkit server.

    Loads credentials from the gitignored .env (API_KEY / BLENDKIT_SERVER),
    exposes them as BLENDKIT_API_KEY / BLENDKIT_SERVER, and runs the
    ``live``-tagged tests. Tests that need a key skip themselves when none is set.

    Args:
        args: Parsed CLI arguments (unused).
    """
    env = live_env()
    if "BLENDKIT_API_KEY" not in env:
        print("warning: no API_KEY found in .env; auth-requiring live tests will skip")
    print(f"=== Running live tests against {env['BLENDKIT_SERVER']} ===")
    proc = subprocess.Popen(
        ["go", "test", "-tags=live", "-run", "TestLive", "-v", "./..."],
        cwd=CLIENT_DIR,
        env=env,
    )
    proc.wait()
    sys.exit(proc.returncode or 0)


def verify(args):
    """Verify code-signing/notarization of built Client binaries.

    - On Windows binaries, osslsigncode must be on PATH
      (https://github.com/mtrojnar/osslsigncode).
    - On macOS binaries, codesign and spctl are used.

    Attributes:
        args: Parsed CLI arguments. Uses ``args.path`` as the directory holding
            the ``blenderkit-client-*`` binaries to verify.
    """
    binaries_path = args.path
    print("===== VERIFYING CLIENT BINARIES =====")
    signatures_ok = True
    client_files = [f for f in os.listdir(binaries_path) if f.startswith("blenderkit-client")]
    for file_name in client_files:
        print(f"\n\n==={file_name}")
        file_path = os.path.join(binaries_path, file_name)

        if file_path.endswith(".exe"):
            if not _verify_windows(file_path):
                signatures_ok = False
            continue

        if "macos" in file_path:
            if not _verify_macos(file_path):
                signatures_ok = False
            continue

        print(">>> SKIPPED (Linux binaries are not signed)")

    if not signatures_ok:
        print("\n>>>>> Verification failed for one or more files, exiting.")
        sys.exit(1)
    print("\n>>>>> Verification OK for all files!\n\n")


def _verify_windows(file_path: str) -> bool:
    """Verify the Authenticode signature of a Windows Client binary.

    Attributes:
        file_path: Path to the .exe binary to verify.

    Returns:
        True if the signature matches the expected Blendkit identity.
    """
    process = subprocess.Popen(
        ["osslsigncode", "verify", "-in", file_path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    output, _ = process.communicate()
    stdout = str(output)
    expected_fields = [
        "CN=BlenderKit s.r.o.",
        "O=BlenderKit s.r.o.",
        "L=Prague",
        "ST=Prague",
        "C=CZ",
    ]
    if all(field in stdout for field in expected_fields):
        print(">>> OK!")
        return True
    print(">>> ERROR")
    return False


def _verify_macos(file_path: str) -> bool:
    """Verify codesigning and notarization of a macOS Client binary.

    Attributes:
        file_path: Path to the macOS binary to verify.

    Returns:
        True if both codesigning and notarization checks pass.
    """
    ok = True

    process = subprocess.Popen(
        ["codesign", "--verify", "-vvvv", file_path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    output, error = process.communicate()
    expected = "satisfies its Designated Requirement"
    if expected in str(output) or expected in str(error):
        print(">>> OK on codesigning")
    else:
        print(">>> ERROR on codesigning")
        ok = False

    process = subprocess.Popen(
        ["spctl", "--assess", "-vvv", "--ignore-cache", file_path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    output, error = process.communicate()
    expected = "origin=Developer ID Application: BlenderKit s.r.o. (A839AY9877)"
    if expected in str(output) or expected in str(error):
        print(">>> OK notarization!")
    else:
        print(">>> ERROR notarization")
        ok = False

    return ok


def run_go_tests() -> None:
    """Run the Client Go unit tests.

    Exits the process with code 1 if the tests fail.
    """
    print("\n=== Running Client Go unit tests ===")
    proc = subprocess.Popen(["go", "test", "./..."], cwd=CLIENT_DIR)
    proc.wait()
    if proc.returncode != 0:
        sys.exit(1)
    print("=== Go tests passed ===\n")


def lint_python(*, fix: bool) -> bool:
    """Lint (and optionally auto-fix) the bundled Python recipes.

    Runs ruff and pydoclint against client/tools. Linting the recipes keeps the
    background scripts consistent and well-documented.

    Attributes:
        fix: When True, run ruff in formatting/auto-fix mode instead of
            check-only mode.

    Returns:
        True if all linters passed (or auto-fix succeeded).
    """
    ok = True
    if fix:
        print("=== Formatting Python recipes (ruff format) ===")
        ok = _run_py_tool("ruff", ["format", TOOLS_DIR]) == 0 and ok
        print("=== Auto-fixing Python recipes (ruff check --fix) ===")
        ok = _run_py_tool("ruff", ["check", "--fix", TOOLS_DIR]) == 0 and ok
        return ok

    print("=== Linting Python recipes (ruff check) ===")
    ok = _run_py_tool("ruff", ["check", TOOLS_DIR]) == 0 and ok
    print("=== Checking docstrings (pydoclint) ===")
    ok = _run_py_tool("pydoclint", [TOOLS_DIR]) == 0 and ok
    return ok


def test(args):
    """Run Go unit tests and lint the Python recipes.

    Attributes:
        args: Parsed CLI arguments (unused).
    """
    run_go_tests()
    if not lint_python(fix=False):
        sys.exit(1)


def lint(args):
    """Lint the Python recipes without writing changes.

    Attributes:
        args: Parsed CLI arguments (unused).
    """
    if not lint_python(fix=False):
        sys.exit(1)


def format_code(args):
    """Format and auto-fix the Python recipes with ruff.

    Attributes:
        args: Parsed CLI arguments (unused).
    """
    if not lint_python(fix=True):
        sys.exit(1)


def docs(args):
    """Regenerate the API documentation via ``go generate``.

    Attributes:
        args: Parsed CLI arguments (unused).
    """
    print("=== Regenerating API documentation (go generate) ===")
    proc = subprocess.Popen(["go", "generate", "./..."], cwd=CLIENT_DIR)
    proc.wait()
    if proc.returncode != 0:
        sys.exit(1)
    print("=== API documentation regenerated ===")


def main():
    """Parse CLI arguments and dispatch to the selected command."""
    parser = argparse.ArgumentParser(description="Blendkit-Client developer helper.")
    sub = parser.add_subparsers(dest="command", required=True)

    p_build = sub.add_parser("build", help="Cross-compile the Client for all platforms.")
    p_build.add_argument("--out", default="out", help="Output directory (default: ./out).")
    p_build.set_defaults(func=build)

    p_run = sub.add_parser("run", help="Build for the current platform and run the Client standalone.")
    p_run.add_argument(
        "client_args",
        nargs=argparse.REMAINDER,
        help="Arguments forwarded to the Client (e.g. -- --port 62000).",
    )
    p_run.set_defaults(func=run)

    sub.add_parser(
        "live",
        help="Run live integration tests against a real server (creds from .env).",
    ).set_defaults(func=live)

    p_verify = sub.add_parser("verify", help="Verify signing/notarization of built binaries.")
    p_verify.add_argument("path", help="Directory containing the blenderkit-client-* binaries.")
    p_verify.set_defaults(func=verify)

    sub.add_parser("test", help="Run Go unit tests and lint Python recipes.").set_defaults(func=test)
    sub.add_parser("lint", help="Lint Python recipes (ruff + pydoclint).").set_defaults(func=lint)
    sub.add_parser("format", help="Format/auto-fix Python recipes with ruff.").set_defaults(func=format_code)
    sub.add_parser("docs", help="Regenerate API documentation (go generate).").set_defaults(func=docs)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
