#!/bin/bash

# Behavioral tests for the teardown action's state-persistence guard and the
# application-status listing, plus the restore-state marker they depend on.
# The actual `run:` blocks are extracted from the composite action YAML and
# executed with stubbed `rad`/`git` on PATH -- no cluster or rad CLI required.
#
# Invariants covered:
#   1. restore-state writes the marker after `rad startup` succeeds.
#   2. teardown skips `rad shutdown` when the marker is absent (a run that
#      failed before startup must not overwrite the durable state archive).
#   3. teardown runs `rad shutdown` when the marker is present.
#   4. teardown lists applications via the Radius.Core preview API surface
#      (`rad app list --preview`), not the legacy Applications.Core plane.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
readonly REPO_ROOT

if ! command -v python3 >/dev/null 2>&1; then
    echo "FAIL: python3 is required to parse action.yml" >&2
    exit 1
fi

python3 - "${REPO_ROOT}" <<'PYTHON'
import os
import pathlib
import re
import subprocess
import sys
import tempfile

repo_root = pathlib.Path(sys.argv[1])
restore_action = repo_root / ".github/extension/actions/restore-state/action.yml"
teardown_action = repo_root / ".github/extension/actions/teardown/action.yml"

failures = []


def fail(message):
    failures.append(message)


def extract_run_block(action_file, step_name):
    """Return the shell body of the `run: |` block for a named step."""
    lines = action_file.read_text(encoding="utf-8").splitlines()

    name_pattern = re.compile(r"^\s*-\s*name:\s*" + re.escape(step_name) + r"\s*$")
    name_idx = next((i for i, line in enumerate(lines) if name_pattern.match(line)), None)
    if name_idx is None:
        sys.exit(f"{action_file}: step '{step_name}' not found")

    run_pattern = re.compile(r"^\s*run:\s*\|\s*$")
    run_idx = next(
        (i for i in range(name_idx + 1, len(lines)) if run_pattern.match(lines[i])),
        None,
    )
    if run_idx is None:
        sys.exit(f"{action_file}: step '{step_name}' has no 'run: |' block")

    body, base_indent = [], None
    for line in lines[run_idx + 1:]:
        if not line.strip():
            body.append("")
            continue
        indent = len(line) - len(line.lstrip(" "))
        if base_indent is None:
            base_indent = indent
        if indent < base_indent:
            break
        body.append(line[base_indent:])

    if not any(chunk.strip() for chunk in body):
        sys.exit(f"{action_file}: step '{step_name}' run block is empty")

    return "\n".join(body).rstrip("\n") + "\n"


def run_block(script, runner_temp):
    """Execute a run block with stubbed rad/git; return (stdout, rad invocations)."""
    workdir = pathlib.Path(tempfile.mkdtemp())
    bin_dir = workdir / "bin"
    bin_dir.mkdir()
    rad_log = workdir / "rad.log"

    (bin_dir / "rad").write_text(
        "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"${RAD_LOG}\"\n", encoding="utf-8"
    )
    # Stub git so `git config --global` cannot mutate the real environment.
    (bin_dir / "git").write_text("#!/bin/bash\nexit 0\n", encoding="utf-8")
    for stub in ("rad", "git"):
        (bin_dir / stub).chmod(0o755)

    script_path = workdir / "block.sh"
    script_path.write_text(script, encoding="utf-8")

    env = dict(os.environ)
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    env["RUNNER_TEMP"] = str(runner_temp)
    env["RAD_LOG"] = str(rad_log)

    # GitHub runs composite bash `run:` blocks with `set -eo pipefail`.
    result = subprocess.run(
        ["bash", "-eo", "pipefail", str(script_path)],
        capture_output=True,
        text=True,
        env=env,
    )
    if result.returncode != 0:
        fail(
            f"run block exited {result.returncode}: {result.stderr.strip() or result.stdout.strip()}"
        )
    invocations = rad_log.read_text(encoding="utf-8") if rad_log.exists() else ""
    return result.stdout, invocations


restore_block = extract_run_block(restore_action, "Restore Radius state (rad startup)")
persist_block = extract_run_block(teardown_action, "Persist Radius state (rad shutdown)")
status_block = extract_run_block(teardown_action, "Show application status")

# 1. restore-state writes the marker after rad startup, in a shared RUNNER_TEMP.
runner_temp = pathlib.Path(tempfile.mkdtemp())
marker = runner_temp / "radius-state-restored"
_, restore_rad = run_block(restore_block, runner_temp)
if "startup" not in restore_rad:
    fail("restore-state block must call `rad startup`")
if not marker.exists():
    fail("restore-state block must write the state-restored marker after `rad startup`")

# 3. With the marker present (from the restore above), teardown runs rad shutdown.
_, persist_rad_present = run_block(persist_block, runner_temp)
if "shutdown" not in persist_rad_present:
    fail("teardown must run `rad shutdown` when the marker is present")

# 2. With no marker (fresh RUNNER_TEMP), teardown must skip rad shutdown.
empty_temp = pathlib.Path(tempfile.mkdtemp())
persist_stdout, persist_rad_absent = run_block(persist_block, empty_temp)
if "shutdown" in persist_rad_absent:
    fail("teardown must NOT run `rad shutdown` when the marker is absent")
if "skipping" not in persist_stdout.lower():
    fail("teardown should log that it is skipping persistence when the marker is absent")

# 4. Status listing uses the preview surface.
_, status_rad = run_block(status_block, runner_temp)
if "app list --preview" not in status_rad:
    fail("teardown status step must run `rad app list --preview`")
if re.search(r"(?m)^app list\s*$", status_rad):
    fail("teardown status step must not use the legacy `rad app list` (no --preview)")

if failures:
    for message in failures:
        sys.stderr.write(f"FAIL: {message}\n")
    sys.exit(1)

print("teardown state-persistence and status tests passed")
PYTHON
