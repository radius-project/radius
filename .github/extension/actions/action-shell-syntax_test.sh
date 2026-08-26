#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 is required to parse action.yml"
fi

python3 - "${SCRIPT_DIR}" <<'PYTHON'
import pathlib
import re
import subprocess
import sys
import tempfile

actions_dir = pathlib.Path(sys.argv[1])
action_files = sorted(actions_dir.rglob("action.yml"))
if not action_files:
    sys.exit(f"no action.yml files found under {actions_dir}")

run_pattern = re.compile(r"^\s*run:\s*\|\s*$")
checked = 0

for action_file in action_files:
    lines = action_file.read_text(encoding="utf-8").splitlines()
    starts = [i for i, line in enumerate(lines) if run_pattern.match(line)]

    for block_index, start in enumerate(starts, 1):
        body, base_indent = [], None
        for line in lines[start + 1:]:
            if not line.strip():
                body.append("")
                continue
            indent = len(line) - len(line.lstrip(" "))
            if base_indent is None:
                base_indent = indent
            if indent < base_indent:
                break
            body.append(line[base_indent:])

        if not body:
            sys.exit(f"{action_file}: run block #{block_index} is empty")

        script = "\n".join(body).rstrip("\n") + "\n"
        with tempfile.NamedTemporaryFile(
            "w", suffix=".sh", encoding="utf-8", delete=False
        ) as tmp:
            tmp.write(script)
            tmp_path = pathlib.Path(tmp.name)

        try:
            parsed = subprocess.run(
                ["bash", "-n", str(tmp_path)], capture_output=True, text=True
            )
        finally:
            tmp_path.unlink(missing_ok=True)

        if parsed.returncode != 0:
            sys.stderr.write(
                f"{action_file}: run block #{block_index} failed bash -n\n"
            )
            sys.stderr.write(parsed.stderr)
            sys.exit(parsed.returncode)

        checked += 1

if checked == 0:
    sys.exit(f"no run: | blocks found under {actions_dir}")

print(f"validated {checked} run blocks across {len(action_files)} action.yml files")
PYTHON

echo "extension action shell syntax tests passed"
