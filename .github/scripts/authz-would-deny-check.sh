#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Authorization Dry-Run Would-Deny Check
#
# Fails when a Radius component logged that an authorization check would deny
# a request (the authzWouldDeny log field from pkg/authz). Run it after the
# functional tests on an installation with global.rbac.dryRun=true to catch
# false positives before enforcement is turned on.
#
# By default it reads current and previous container logs from every pod in
# the radius-system namespace. With --logs-dir it scans saved log files
# instead; add --cluster to scan both.
#
# Known, tracked would-deny lines can be listed in an allowlist file. Each
# non-comment line is "<issue> <pattern>", where <issue> is a GitHub issue
# URL or owner/repo#number and <pattern> is an extended regular expression
# matched against the log line.
#
# Exit codes: 0 no unexpected would-deny lines, 1 would-deny lines found,
# 2 usage or configuration error.
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly WOULD_DENY_PATTERN='authzWouldDeny"?[[:space:]]*[:=][[:space:]]*"?true'
readonly ISSUE_PATTERN='^(https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/issues/[0-9]+|[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+#[0-9]+)$'
# One line per pod: "<pod>\t<container>=<restartCount> ...".
readonly PODS_JSONPATH='{range .items[*]}{.metadata.name}{"\t"}{range .status.containerStatuses[*]}{.name}{"="}{.restartCount}{" "}{end}{"\n"}{end}'

NAMESPACE="${AUTHZ_NAMESPACE:-radius-system}"
ALLOWLIST="${AUTHZ_WOULD_DENY_ALLOWLIST:-${SCRIPT_DIR}/authz-would-deny-allowlist.txt}"
ALLOWLIST_REQUIRED=false
SCAN_CLUSTER=false
LOG_DIRS=()
ALLOW_ISSUES=()
ALLOW_PATTERNS=()

usage() {
    cat <<EOF
Usage: $(basename "$0") [--namespace NAME] [--allowlist FILE] [--logs-dir DIR]... [--cluster]

  --namespace NAME  Namespace to read pod logs from (default: radius-system, env: AUTHZ_NAMESPACE).
  --allowlist FILE  Allowlist of tracked would-deny lines (env: AUTHZ_WOULD_DENY_ALLOWLIST).
  --logs-dir DIR    Scan log files under DIR instead of the cluster. Repeatable.
  --cluster         Also scan cluster pod logs when --logs-dir is set.
EOF
}

die() {
    echo "error: $*" >&2
    exit 2
}

trim() {
    local value="$1"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    printf '%s' "${value}"
}

load_allowlist() {
    if [[ ! -f "${ALLOWLIST}" ]]; then
        [[ "${ALLOWLIST_REQUIRED}" != true ]] || die "allowlist not found: ${ALLOWLIST}"
        return 0
    fi

    local line line_no=0 issue pattern status
    while IFS= read -r line || [[ -n "${line}" ]]; do
        ((++line_no))
        line="$(trim "${line}")"
        [[ -z "${line}" || "${line}" == \#* ]] && continue

        issue="${line%%[[:space:]]*}"
        pattern="$(trim "${line#"${issue}"}")"
        [[ "${issue}" =~ ${ISSUE_PATTERN} ]] \
            || die "${ALLOWLIST}:${line_no}: entry must start with an issue reference (https://github.com/OWNER/REPO/issues/N or OWNER/REPO#N)"
        [[ -n "${pattern}" ]] || die "${ALLOWLIST}:${line_no}: entry is missing a pattern"

        status=0
        grep -Eq -- "${pattern}" </dev/null || status=$?
        [[ "${status}" -le 1 ]] || die "${ALLOWLIST}:${line_no}: invalid pattern: ${pattern}"

        ALLOW_ISSUES+=("${issue}")
        ALLOW_PATTERNS+=("${pattern}")
    done <"${ALLOWLIST}"
}

collect_cluster_logs() {
    local out_dir="$1" pods pod containers entry container restarts total=0 read_ok=0

    command -v kubectl >/dev/null || die "kubectl is required to read pod logs"
    pods="$(kubectl get pods --namespace "${NAMESPACE}" --output "jsonpath=${PODS_JSONPATH}")"
    [[ -n "${pods//[[:space:]]/}" ]] || die "no pods found in namespace ${NAMESPACE}"

    mkdir -p "${out_dir}"
    while IFS=$'\t' read -r pod containers; do
        [[ -n "${pod}" ]] || continue
        ((++total))

        if kubectl logs "${pod}" --namespace "${NAMESPACE}" --all-containers --prefix >"${out_dir}/${pod}.log"; then
            ((++read_ok))
        else
            echo "warning: could not read logs for pod ${pod}" >&2
        fi

        for entry in ${containers}; do
            container="${entry%%=*}"
            restarts="${entry#*=}"
            [[ "${restarts}" =~ ^[0-9]+$ && "${restarts}" -gt 0 ]] || continue
            kubectl logs "${pod}" --namespace "${NAMESPACE}" --container "${container}" --previous \
                >>"${out_dir}/${pod}.previous.log" \
                || echo "warning: could not read previous logs for container ${container} in pod ${pod}" >&2
        done
    done <<<"${pods}"

    [[ "${read_ok}" -gt 0 ]] || die "could not read logs for any pod in namespace ${NAMESPACE}"
    echo "Read logs from ${read_ok} of ${total} pod(s) in namespace ${NAMESPACE}."
}

# Prints the issue for the first allowlist entry that matches the line.
allowlisted_issue() {
    local i
    for i in "${!ALLOW_PATTERNS[@]}"; do
        if grep -Eq -- "${ALLOW_PATTERNS[i]}" <<<"$1"; then
            printf '%s' "${ALLOW_ISSUES[i]}"
            return 0
        fi
    done
    return 1
}

source_label() {
    local rel="${1#"$2"/}"
    case "${rel}" in
        *.previous.log) printf '%s (previous)' "${rel%.previous.log}" ;;
        *.log) printf '%s' "${rel%.log}" ;;
        *) printf '%s' "${rel}" ;;
    esac
}

FOUND=0
ALLOWED=0

scan_dir() {
    local dir="$1" file label line issue
    while IFS= read -r -d '' file; do
        label="$(source_label "${file}" "${dir}")"
        while IFS= read -r line; do
            if issue="$(allowlisted_issue "${line}")"; then
                echo "allowed (${issue}): ${label}: ${line}"
                ((++ALLOWED))
            else
                echo "would-deny: ${label}: ${line}"
                ((++FOUND))
            fi
        done < <(grep -Ea -- "${WOULD_DENY_PATTERN}" "${file}" || true)
    done < <(find "${dir}" -type f -print0 | sort -z)
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --namespace)
            [[ $# -ge 2 ]] || die "--namespace requires a value"
            NAMESPACE="$2"
            shift 2
            ;;
        --allowlist)
            [[ $# -ge 2 ]] || die "--allowlist requires a value"
            ALLOWLIST="$2"
            ALLOWLIST_REQUIRED=true
            shift 2
            ;;
        --logs-dir)
            [[ $# -ge 2 ]] || die "--logs-dir requires a value"
            LOG_DIRS+=("${2%/}")
            shift 2
            ;;
        --cluster)
            SCAN_CLUSTER=true
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            usage >&2
            die "unknown argument: $1"
            ;;
    esac
done

[[ "${#LOG_DIRS[@]}" -gt 0 ]] || SCAN_CLUSTER=true
for dir in "${LOG_DIRS[@]+"${LOG_DIRS[@]}"}"; do
    [[ -d "${dir}" ]] || die "logs directory not found: ${dir}"
done

load_allowlist

WORK_DIR="$(mktemp -d)"
readonly WORK_DIR
trap 'rm -rf "${WORK_DIR}"' EXIT

if [[ "${SCAN_CLUSTER}" == true ]]; then
    collect_cluster_logs "${WORK_DIR}/cluster"
    LOG_DIRS+=("${WORK_DIR}/cluster")
fi

for dir in "${LOG_DIRS[@]}"; do
    scan_dir "${dir}"
done

if [[ "${FOUND}" -gt 0 ]]; then
    echo ""
    echo "Found ${FOUND} authorization would-deny log line(s). In dry-run mode these requests would be rejected under enforcement."
    echo "Fix the check or the caller, or add an allowlist entry that references a tracking issue to ${ALLOWLIST}."
    exit 1
fi

if [[ "${ALLOWED}" -gt 0 ]]; then
    echo "No authorization would-deny log lines found (${ALLOWED} allowlisted)."
else
    echo "No authorization would-deny log lines found."
fi
