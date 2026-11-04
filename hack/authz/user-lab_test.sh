#!/bin/bash
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
#
# Test launcher contracts without starting Kubernetes or modifying global config.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
directory="$(mktemp -d)"
trap 'rm -rf -- "${directory}"' EXIT
mkdir -p "${directory}/bin" "${directory}/assets" "${directory}/tmp"
for tool in kubectl curl; do
    ln -s /usr/bin/true "${directory}/bin/${tool}"
done
for tool in kube-apiserver etcd; do
    ln -s /usr/bin/true "${directory}/assets/${tool}"
done
cat >"${directory}/bin/go" <<'STUB'
#!/bin/bash
set -euo pipefail
if [[ "$1" == env ]]; then
    case "$2" in
        GOOS) echo linux ;;
        GOARCH) echo arm64 ;;
        *) exit 2 ;;
    esac
    exit
fi
if [[ "${LAB_COMPILE_FAIL:-false}" == true ]]; then
    echo "fixture compile failure" >&2
    exit 1
fi
[[ "$1" == test && "$2" == -c && "$3" == -o ]]
cat >"$4" <<'CHILD'
#!/bin/bash
set -euo pipefail
trap 'exit 0' INT TERM
if [[ "${LAB_CHILD_FAIL:-false}" == true ]]; then
    echo "fixture startup failure" >&2
    exit 1
fi
printf '%s\n' "$$" >"${RADIUS_USER_AUTHZ_LAB_DIR}/child.pid"
printf 'TEST-ONLY-PRIVATE\n' >"${RADIUS_USER_AUTHZ_LAB_DIR}/private.key"
printf 'ready\n' >"${RADIUS_USER_AUTHZ_LAB_DIR}/ready"
while true; do
    if grep -q '^quit$' "${RADIUS_USER_AUTHZ_LAB_DIR}/commands"; then
        exit 0
    fi
    sleep 0.02
done
CHILD
chmod +x "$4"
STUB
chmod +x "${directory}/bin/go"
export PATH="${directory}/bin:${PATH}"
export KUBEBUILDER_ASSETS="${directory}/assets"
export RAD_BIN="${directory}/bin/curl"
export TMPDIR="${directory}/tmp"
export LAB_TEST_SCRIPT="${root}/hack/authz/user-lab.sh"
bash -n "${LAB_TEST_SCRIPT}"
bash "${LAB_TEST_SCRIPT}" --help | grep -q 'Unconfigured'
python3 - <<'PY'
import os
import pathlib
import signal
import subprocess
import time

script = os.environ["LAB_TEST_SCRIPT"]
temp = pathlib.Path(os.environ["TMPDIR"])

def rejected(args, text, **updates):
    env = dict(os.environ, **updates)
    result = subprocess.run(["bash", script, *args], env=env, capture_output=True, text=True)
    assert result.returncode != 0, result
    assert text in result.stderr, result.stderr
    assert not list(temp.iterdir()), "failed startup left private artifacts"

rejected(["unknown"], "Invalid lab mode")
rejected(["Unconfigured", "--missing-anchor"], "Usage:")
rejected([], "Missing executable RAD_BIN", RAD_BIN="/nonexistent/rad")
rejected([], "Set KUBEBUILDER_ASSETS", KUBEBUILDER_ASSETS="/nonexistent")
rejected([], "fixture compile failure", LAB_COMPILE_FAIL="true")
rejected([], "fixture startup failure", LAB_CHILD_FAIL="true")

for stop in ("quit", "eof", signal.SIGTERM, signal.SIGINT, "group-int"):
    process = subprocess.Popen(
        ["bash", script, "DryRun"], stdin=subprocess.PIPE,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        start_new_session=True,
    )
    try:
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            ready = list(temp.glob("radius-user-authz.*/ready"))
            if ready:
                break
            assert process.poll() is None, process.communicate()
            time.sleep(0.02)
        assert ready, "launcher never became ready"
        lab = ready[0].parent
        assert lab.stat().st_mode & 0o777 == 0o700
        assert (lab / "private.key").stat().st_mode & 0o777 == 0o600
        child = int((lab / "child.pid").read_text())
        if stop == "quit":
            process.stdin.write("quit\n")
            process.stdin.flush()
        elif stop == "eof":
            process.stdin.close()
            process.stdin = None
        elif stop == "group-int":
            os.killpg(process.pid, signal.SIGINT)
        else:
            process.send_signal(stop)
        stdout, stderr = process.communicate(timeout=10)
        assert process.returncode == {"quit": 0, "eof": 0, signal.SIGINT: 130, signal.SIGTERM: 143, "group-int": 130}[stop], stderr
        assert "TEST-ONLY-PRIVATE" not in stdout + stderr
        assert not lab.exists(), "launcher left private artifacts"
        try:
            os.kill(child, 0)
        except ProcessLookupError:
            pass
        else:
            raise AssertionError("launcher left its child running")
    finally:
        if process.poll() is None:
            process.terminate()
            process.communicate(timeout=10)

print("user lab: help, inputs, prerequisites, compile/start failure, private permissions, quit/EOF/INT/TERM cleanup passed")
PY
