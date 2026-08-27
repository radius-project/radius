#!/bin/bash

# Behavioral tests for the teardown action's state-persistence guard and the
# application-status listing, plus the restore-state output they depend on.
# The actual `run:` blocks are extracted from the composite action YAML and
# executed with stubbed `rad`/`git` on PATH -- no cluster or rad CLI required.
#
# Invariants covered:
#   1. restore-state sets `state-restored=true` on $GITHUB_OUTPUT after
#      `rad startup` succeeds, and creates the `default` group AFTER startup.
#   2. First run: `rad startup` is a no-op restore that still exits 0, so the
#      output is still set (teardown then seeds the archive).
#   3. Negative path: when `rad startup` fails, the block exits non-zero and
#      never sets `state-restored`, so teardown will skip persistence.
#   4. teardown runs `rad shutdown` only when state-restored == "true".
#   5. teardown skips `rad shutdown` (with a ::warning::) when it is not "true"
#      (a run that failed before startup must not overwrite the state archive).
#   6. teardown lists applications via the Radius.Core preview API surface
#      (`rad app list --preview`), and surfaces a warning if that listing fails
#      instead of swallowing it silently.
#   7. Every workflow that uses the teardown action (discovered dynamically)
#      wires state-restored from a restore-state step (id: restore-state) in the
#      SAME job, uses both actions together, and the check is proven to reject
#      broken wiring (missing id, missing output pass-through, cross-job split).

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
import shutil
import subprocess
import sys
import tempfile

repo_root = pathlib.Path(sys.argv[1])
ext_root = repo_root / ".github/extension"
restore_action = ext_root / "actions/restore-state/action.yml"
teardown_action = ext_root / "actions/teardown/action.yml"

# Discover the workflow files dynamically instead of hardcoding, so a newly
# added workflow that uses the teardown action is covered automatically.
teardown_use = re.compile(r"actions/teardown@")
restore_use = re.compile(r"actions/restore-state@")
all_workflows = sorted(p for p in ext_root.glob("*.yml"))
teardown_workflows = [p for p in all_workflows if teardown_use.search(p.read_text(encoding="utf-8"))]
restore_workflows = [p for p in all_workflows if restore_use.search(p.read_text(encoding="utf-8"))]

failures = []

# Single scratch root cleaned up on exit, so no per-case temp dirs leak.
scratch_root = pathlib.Path(tempfile.mkdtemp(prefix="teardown-test-"))


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


_case_counter = [0]


def run_block(script, env_extra=None, rad_fail_on="", capture_output=False,
              expect_failure=False):
    """Execute a run block with stubbed rad/git.

    rad_fail_on: substring of the joined rad args that makes the stub exit 1
    (used to simulate `rad startup` or `rad app list` failing).
    Returns (stdout, stderr, rad_invocations, github_output_contents).
    """
    _case_counter[0] += 1
    workdir = scratch_root / f"case-{_case_counter[0]}"
    bin_dir = workdir / "bin"
    bin_dir.mkdir(parents=True)
    rad_log = workdir / "rad.log"
    github_output = workdir / "github_output"
    github_output.write_text("", encoding="utf-8")

    (bin_dir / "rad").write_text(
        "#!/bin/bash\n"
        'printf \'%s\\n\' "$*" >> "${RAD_LOG}"\n'
        'if [ -n "${RAD_FAIL_ON}" ] && [[ "$*" == *"${RAD_FAIL_ON}"* ]]; then\n'
        "  exit 1\n"
        "fi\n"
        "exit 0\n",
        encoding="utf-8",
    )
    # Stub git so `git config --global` cannot mutate the real environment.
    (bin_dir / "git").write_text("#!/bin/bash\nexit 0\n", encoding="utf-8")
    for stub in ("rad", "git"):
        (bin_dir / stub).chmod(0o755)

    script_path = workdir / "block.sh"
    script_path.write_text(script, encoding="utf-8")

    env = dict(os.environ)
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    env["RAD_LOG"] = str(rad_log)
    env["GITHUB_OUTPUT"] = str(github_output)
    env["RAD_FAIL_ON"] = rad_fail_on
    if env_extra:
        env.update(env_extra)

    # GitHub runs composite bash `run:` blocks with `set -eo pipefail`.
    result = subprocess.run(
        ["bash", "-eo", "pipefail", str(script_path)],
        capture_output=True,
        text=True,
        env=env,
    )
    if expect_failure and result.returncode == 0:
        fail("expected run block to fail but it exited 0")
    if not expect_failure and result.returncode != 0:
        fail(
            f"run block exited {result.returncode}: "
            f"{result.stderr.strip() or result.stdout.strip()}"
        )
    invocations = rad_log.read_text(encoding="utf-8") if rad_log.exists() else ""
    output = github_output.read_text(encoding="utf-8")
    return result.stdout, result.stderr, invocations, output


def job_at_line(lines, target_idx):
    """Return the job name that owns the line at target_idx, or None."""
    job = None
    in_jobs = False
    for line in lines[: target_idx + 1]:
        if re.match(r"^jobs:\s*$", line):
            in_jobs = True
            continue
        if in_jobs and re.match(r"^\S", line):
            in_jobs = False
        if in_jobs:
            m = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
            if m:
                job = m.group(1)
    return job


restore_block = extract_run_block(restore_action, "Restore Radius state (rad startup)")
persist_block = extract_run_block(teardown_action, "Persist Radius state (rad shutdown)")
status_block = extract_run_block(teardown_action, "Show application status")

# 1. restore-state: on success, sets the output and creates the group after startup.
_, _, restore_rad, restore_out = run_block(restore_block)
if "startup" not in restore_rad:
    fail("restore-state block must call `rad startup`")
if "state-restored=true" not in restore_out:
    fail("restore-state must write `state-restored=true` to $GITHUB_OUTPUT after startup")
# Group create/switch must come AFTER `rad startup` in the invocation log.
rad_calls = [c for c in restore_rad.splitlines() if c.strip()]
startup_idx = next((i for i, c in enumerate(rad_calls) if c.startswith("startup")), None)
group_idx = next((i for i, c in enumerate(rad_calls) if c.startswith("group create")), None)
if startup_idx is None or group_idx is None:
    fail("restore-state must call both `rad startup` and `rad group create default`")
elif group_idx < startup_idx:
    fail("restore-state must create the `default` group AFTER `rad startup`, not before")

# 2. First run: `rad startup` no-op still exits 0, so the output is still set.
#    (The stub models the no-op restore by succeeding without side effects.)
_, _, _, first_run_out = run_block(restore_block)
if "state-restored=true" not in first_run_out:
    fail("first-run no-op `rad startup` must still set `state-restored=true`")

# 3. Negative path: `rad startup` failing stops the block (set -e) before the
#    echo, so the output is never set and teardown will skip persistence.
_, _, _, failed_out = run_block(restore_block, rad_fail_on="startup", expect_failure=True)
if "state-restored=true" in failed_out:
    fail("restore-state must NOT set `state-restored` when `rad startup` fails")

# 4. teardown persists when state-restored == "true".
_, _, persist_true, _ = run_block(persist_block, env_extra={"STATE_RESTORED": "true"})
if "shutdown" not in persist_true:
    fail("teardown must run `rad shutdown` when state-restored is true")

# 5. teardown skips (with a warning) when state-restored is not "true".
for value in ("", "false"):
    persist_stdout, _, persist_rad, _ = run_block(
        persist_block, env_extra={"STATE_RESTORED": value}
    )
    if "shutdown" in persist_rad:
        fail(f"teardown must NOT run `rad shutdown` when state-restored='{value}'")
    if "::warning" not in persist_stdout:
        fail(f"teardown must emit a ::warning:: when skipping persistence (value='{value}')")

# 6. Status listing uses the preview surface and warns (does not swallow) on failure.
_, _, status_rad, _ = run_block(status_block)
if "app list --preview" not in status_rad:
    fail("teardown status step must run `rad app list --preview`")
if re.search(r"(?m)^app list\s*$", status_rad):
    fail("teardown status step must not use the legacy `rad app list` (no --preview)")
status_fail_stdout, _, _, _ = run_block(status_block, rad_fail_on="app list")
if "::warning" not in status_fail_stdout:
    fail("teardown status step must warn (not swallow) when `rad app list --preview` fails")

# 7. Every workflow using the teardown action wires state-restored from a
#    same-job restore-state step. Checked against every workflow discovered
#    dynamically, with negative cases proving the check rejects broken wiring.
restore_id = re.compile(r"^\s*id:\s*restore-state\s*$")
wiring = re.compile(r"state-restored:\s*\$\{\{\s*steps\.restore-state\.outputs\.state-restored\s*\}\}")


def wiring_problems(lines):
    """Return a list of wiring problems for a workflow's lines (empty == OK)."""
    problems = []
    teardown_idx = next((i for i, l in enumerate(lines) if teardown_use.search(l)), None)
    if teardown_idx is None:
        return problems  # workflow doesn't use teardown; nothing to wire
    id_idx = next((i for i, l in enumerate(lines) if restore_id.match(l)), None)
    wire_idx = next((i for i, l in enumerate(lines) if wiring.search(l)), None)
    if id_idx is None:
        problems.append("uses teardown but has no `id: restore-state` step")
        return problems
    if wire_idx is None:
        problems.append("teardown is not passed the state-restored output")
        return problems
    teardown_job = job_at_line(lines, teardown_idx)
    restore_job = job_at_line(lines, id_idx)
    if teardown_job is None or teardown_job != restore_job:
        problems.append(
            f"restore-state ({restore_job}) and teardown ({teardown_job}) are not in the same job"
        )
    return problems


# Coverage guard: there must be workflows to check, and any workflow that uses
# the teardown action must also use the restore-state action (and vice versa),
# so a new caller cannot land wired to one but not the other.
if not teardown_workflows:
    fail("no workflow uses the teardown action; expected the deploy/delete workflows to")
if set(teardown_workflows) != set(restore_workflows):
    only_teardown = sorted(p.name for p in set(teardown_workflows) - set(restore_workflows))
    only_restore = sorted(p.name for p in set(restore_workflows) - set(teardown_workflows))
    fail(
        "teardown and restore-state must be used by the same workflows; "
        f"teardown-only={only_teardown}, restore-only={only_restore}"
    )

for wf in teardown_workflows:
    lines = wf.read_text(encoding="utf-8").splitlines()

    # Positive: the real workflow wires it correctly.
    for problem in wiring_problems(lines):
        fail(f"{wf.name}: {problem}")

    # Negative cases: each mutation of the real workflow must be rejected.
    without_id = [l for l in lines if not restore_id.match(l)]
    if not wiring_problems(without_id):
        fail(f"{wf.name}: check must reject a workflow missing `id: restore-state`")

    without_wire = [l for l in lines if not wiring.search(l)]
    if not wiring_problems(without_wire):
        fail(f"{wf.name}: check must reject a workflow that does not pass the state-restored output")

    # Split restore-state and teardown into different jobs by inserting a new
    # job header immediately before the teardown step.
    teardown_idx = next((i for i, l in enumerate(lines) if teardown_use.search(l)), None)
    # Walk back to the `- name:` line that starts the teardown step.
    step_start = teardown_idx
    while step_start > 0 and not re.match(r"^\s*-\s*name:", lines[step_start]):
        step_start -= 1
    split = lines[:step_start] + ["  injected-other-job:", "    runs-on: ubuntu-latest", "    steps:"] + lines[step_start:]
    if not wiring_problems(split):
        fail(f"{wf.name}: check must reject restore-state and teardown living in different jobs")

shutil.rmtree(scratch_root, ignore_errors=True)

if failures:
    for message in failures:
        sys.stderr.write(f"FAIL: {message}\n")
    sys.exit(1)

print("teardown state-persistence, status, and workflow-wiring tests passed")
PYTHON
