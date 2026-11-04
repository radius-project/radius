#!/bin/bash
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
#
# Run the isolated, test-only user authorization lab in the foreground.
set -euo pipefail
umask 077

usage() {
    cat <<'EOF'
Usage: bash hack/authz/user-lab.sh [Disabled|DryRun|Enforce|Unconfigured] [--missing-anchor]

Requires Go, kubectl, curl, a built rad (RAD_BIN), and KUBEBUILDER_ASSETS
containing executable kube-apiserver and etcd. Starts no existing cluster.
Prints exact second-terminal commands and accepts local fixture controls on stdin.
Use quit, EOF, Ctrl-C or SIGTERM to stop and remove private test credentials.
See docs/contributing/contributing-code/contributing-code-tests/manual-user-authorization.md.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    usage
    exit 0
fi
mode="${1:-DryRun}"
missing=false
case "${mode}" in
    Disabled | DryRun | Enforce | Unconfigured) ;;
    *) echo "Invalid lab mode: ${mode}" >&2; exit 2 ;;
esac
if [[ $# -gt 0 ]]; then
    shift
fi
if [[ "${1:-}" == "--missing-anchor" && "${mode}" != Unconfigured ]]; then
    missing=true
    shift
fi
if [[ $# -gt 0 ]]; then
    usage >&2
    exit 2
fi
for tool in go kubectl curl; do
    if ! command -v "${tool}" >/dev/null; then
        echo "Missing prerequisite: ${tool}" >&2
        exit 1
    fi
done
if [[ "$(go env GOOS)" == windows ]]; then
    echo "The isolated lab requires Linux or macOS." >&2
    exit 1
fi
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${root}"
export RAD_BIN="${RAD_BIN:-${root}/dist/$(go env GOOS)_$(go env GOARCH)/release/rad}"
if [[ ! -x "${RAD_BIN}" ]]; then
    echo "Missing executable RAD_BIN; build with make build-rad first." >&2
    exit 1
fi
# Resolve before changing directories or publishing commands to another terminal.
RAD_BIN="$(cd "$(dirname "${RAD_BIN}")" && pwd)/$(basename "${RAD_BIN}")"
export RAD_BIN
for binary in kube-apiserver etcd; do
    if [[ ! -x "${KUBEBUILDER_ASSETS:-}/${binary}" ]]; then
        echo "Set KUBEBUILDER_ASSETS to envtest binaries (${binary} missing)." >&2
        exit 1
    fi
done
export KUBEBUILDER_ASSETS
directory="$(mktemp -d "${TMPDIR:-/tmp}/radius-user-authz.XXXXXXXX")"
child=""
reader=""
# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
    local result=$?
    trap - EXIT
    trap '' INT TERM
    if [[ -n "${reader}" ]]; then
        if kill -0 "${reader}" 2>/dev/null; then
            kill -TERM "${reader}"
        fi
        local reader_status=0
        wait "${reader}" || reader_status=$?
        if [[ "${reader_status}" -ne 0 && "${reader_status}" -ne 143 ]]; then
            echo "Lab input reader exited unsuccessfully." >&2
            result=1
        fi
    fi
    if [[ -n "${child}" ]]; then
        if kill -0 "${child}" 2>/dev/null; then
            kill -TERM "${child}"
        fi
        # The test handles TERM and runs t.Cleanup, joining runtime, UCP,
        # API-server and etcd before credentials are removed.
        if ! wait "${child}"; then
            echo "Lab process exited unsuccessfully during cleanup." >&2
            result=1
        fi
    fi
    rm -rf -- "${directory}"
    exit "${result}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo "Compiling test-only lab; no cluster or global config is modified."
go test -c -o "${directory}/lab.test" ./pkg/ucp/frontend/api
export RADIUS_USER_AUTHZ_LAB_DIR="${directory}"
export RADIUS_USER_AUTHZ_LAB_MODE="${mode}"
export RADIUS_USER_AUTHZ_LAB_MISSING_ANCHOR="${missing}"
touch "${directory}/commands"
"${directory}/lab.test" -test.run '^TestManualUserAuthorizationLab$' \
    -test.v -test.timeout=0 </dev/null &
child=$!
# A separate owned reader keeps an idle terminal from hiding child failures.
exec 3<&0
(
    trap - EXIT INT TERM
    while IFS= read -r command; do
        printf '%s\n' "${command}" >>"${directory}/commands"
        if [[ "${command}" == quit ]]; then
            exit 0
        fi
    done
    printf 'quit\n' >>"${directory}/commands"
) <&3 &
reader=$!
result=0
wait "${child}" || result=$?
child=""
exit "${result}"
