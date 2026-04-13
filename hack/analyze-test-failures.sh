#!/bin/bash

# ============================================================================
# Analyze Functional Test Failures
#
# Downloads and analyzes logs from failed GitHub Actions functional test runs
# (cloud and non-cloud) for a given PR, branch, or run ID.
#
# Prerequisites: gh (GitHub CLI, authenticated), jq
#
# Usage:
#   ./hack/analyze-test-failures.sh                # auto-detect from branch
#   ./hack/analyze-test-failures.sh --pr 1234      # explicit PR number
#   ./hack/analyze-test-failures.sh --run-id 12345 # explicit run ID
#   ./hack/analyze-test-failures.sh --keep          # preserve downloaded logs
# ============================================================================

set -euo pipefail

# Default values
PR_NUMBER=""
RUN_ID=""
KEEP_LOGS=false
REPO="radius-project/radius"

readonly SCRIPT_NAME="$(basename "$0")"
readonly CLOUD_WORKFLOW="functional-test-cloud.yaml"
readonly NONCLOUD_WORKFLOW="functional-test-noncloud.yaml"

TEMP_DIR=""

# Error patterns for log analysis
readonly GO_TEST_FAIL_PATTERN='--- FAIL:|^FAIL\s|FAIL\t'
readonly PANIC_PATTERN='panic:|goroutine [0-9]+ \[running\]'
readonly TIMEOUT_PATTERN='context deadline exceeded|test timed out|timeout|i/o timeout'
readonly K8S_ERROR_PATTERN='CrashLoopBackOff|OOMKilled|ImagePullBackOff|ErrImagePull|BackOff|Error:'
readonly RADIUS_ERROR_PATTERN='level.*error|"error":|failed to'

cleanup() {
    if [[ "${KEEP_LOGS}" == "true" && -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        echo ""
        echo "Logs preserved at: ${TEMP_DIR}"
        return
    fi
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        echo ""
        echo "Cleaning up temporary logs..."
        rm -rf "${TEMP_DIR}"
    fi
}

trap cleanup EXIT

usage() {
    echo "Usage: ${SCRIPT_NAME} [OPTIONS]"
    echo ""
    echo "Analyze functional test failures from GitHub Actions runs."
    echo ""
    echo "Options:"
    echo "  --pr NUMBER       PR number to analyze"
    echo "  --run-id ID       Specific workflow run ID to analyze"
    echo "  --repo OWNER/REPO Repository (default: ${REPO})"
    echo "  --keep            Preserve downloaded logs (skip cleanup)"
    echo "  -h, --help        Show this help"
    echo ""
    echo "If no PR or run ID is specified, auto-detects from the current git branch."
    exit 0
}

validate_requirements() {
    local missing=false

    if ! command -v gh &>/dev/null; then
        echo "Error: 'gh' (GitHub CLI) is required but not installed." >&2
        echo "  Install: https://cli.github.com/" >&2
        missing=true
    fi

    if ! command -v jq &>/dev/null; then
        echo "Error: 'jq' is required but not installed." >&2
        echo "  Install: https://jqlang.github.io/jq/download/" >&2
        missing=true
    fi

    if [[ "${missing}" == "true" ]]; then
        exit 1
    fi

    # Verify gh is authenticated
    if ! gh auth status &>/dev/null 2>&1; then
        echo "Error: 'gh' is not authenticated. Run 'gh auth login' first." >&2
        exit 1
    fi
}

detect_pr_from_branch() {
    local branch
    branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"

    if [[ -z "${branch}" || "${branch}" == "HEAD" ]]; then
        echo "Error: Could not detect current branch." >&2
        echo "  Use --pr NUMBER or --run-id ID to specify explicitly." >&2
        exit 1
    fi

    echo "Detecting PR for branch: ${branch}"

    local pr_json
    pr_json="$(gh pr list --repo "${REPO}" --head "${branch}" --json number,url --limit 1 2>/dev/null || true)"

    if [[ -z "${pr_json}" || "${pr_json}" == "[]" ]]; then
        # Try with the user's fork prefix
        local gh_user
        gh_user="$(gh api user --jq '.login' 2>/dev/null || true)"
        if [[ -n "${gh_user}" ]]; then
            pr_json="$(gh pr list --repo "${REPO}" --head "${gh_user}:${branch}" --json number,url --limit 1 2>/dev/null || true)"
        fi
    fi

    if [[ -z "${pr_json}" || "${pr_json}" == "[]" ]]; then
        echo "Error: No open PR found for branch '${branch}' in ${REPO}." >&2
        echo "  Use --pr NUMBER or --run-id ID to specify explicitly." >&2
        exit 1
    fi

    PR_NUMBER="$(echo "${pr_json}" | jq -r '.[0].number')"
    local pr_url
    pr_url="$(echo "${pr_json}" | jq -r '.[0].url')"
    echo "Found PR #${PR_NUMBER}: ${pr_url}"
}

# Find workflow runs for a PR. Outputs JSON array of runs.
find_runs_for_pr() {
    local workflow_file="$1"
    local pr_number="$2"

    # List recent runs for this workflow, filter by the PR's head branch
    local runs_json
    runs_json="$(gh api \
        "repos/${REPO}/actions/workflows/${workflow_file}/runs?per_page=10&event=pull_request,pull_request_target" \
        --jq ".workflow_runs | map(select(.pull_requests[]?.number == ${pr_number} or .display_title == (.display_title // \"\"))) | sort_by(.created_at) | reverse" \
        2>/dev/null || echo "[]")"

    # If the PR-based filter didn't work, try matching by head_sha from the PR
    if [[ "${runs_json}" == "[]" || "${runs_json}" == "null" ]]; then
        local pr_head_sha
        pr_head_sha="$(gh pr view "${pr_number}" --repo "${REPO}" --json headRefOid --jq '.headRefOid' 2>/dev/null || true)"

        if [[ -n "${pr_head_sha}" ]]; then
            runs_json="$(gh api \
                "repos/${REPO}/actions/workflows/${workflow_file}/runs?per_page=10&head_sha=${pr_head_sha}" \
                --jq '.workflow_runs | sort_by(.created_at) | reverse' \
                2>/dev/null || echo "[]")"
        fi
    fi

    # If still nothing, try matching by branch name
    if [[ "${runs_json}" == "[]" || "${runs_json}" == "null" ]]; then
        local pr_branch
        pr_branch="$(gh pr view "${pr_number}" --repo "${REPO}" --json headRefName --jq '.headRefName' 2>/dev/null || true)"

        if [[ -n "${pr_branch}" ]]; then
            runs_json="$(gh api \
                "repos/${REPO}/actions/workflows/${workflow_file}/runs?per_page=10&branch=${pr_branch}" \
                --jq '.workflow_runs | sort_by(.created_at) | reverse' \
                2>/dev/null || echo "[]")"
        fi
    fi

    echo "${runs_json}"
}

# Get failed jobs for a run. Outputs JSON array.
get_failed_jobs() {
    local run_id="$1"

    gh api "repos/${REPO}/actions/runs/${run_id}/jobs?per_page=100" \
        --jq '.jobs | map(select(.conclusion == "failure"))' \
        2>/dev/null || echo "[]"
}

# Download job logs for a specific job
download_job_logs() {
    local job_id="$1"
    local job_name="$2"
    local output_dir="$3"

    local safe_name
    safe_name="$(echo "${job_name}" | tr ' /' '__')"
    local log_file="${output_dir}/${safe_name}.log"

    gh api "repos/${REPO}/actions/jobs/${job_id}/logs" >"${log_file}" 2>/dev/null || {
        echo "  Warning: Could not download logs for job '${job_name}' (ID: ${job_id})" >&2
        return 1
    }

    echo "${log_file}"
}

# Download artifacts for a run
download_artifacts() {
    local run_id="$1"
    local output_dir="$2"

    local artifacts_json
    artifacts_json="$(gh api "repos/${REPO}/actions/runs/${run_id}/artifacts" \
        --jq '.artifacts | map(select(.name | test("container_logs|pod-logs|test_results")))' \
        2>/dev/null || echo "[]")"

    local count
    count="$(echo "${artifacts_json}" | jq 'length')"

    if [[ "${count}" -eq 0 ]]; then
        echo "  No relevant artifacts found."
        return
    fi

    echo "  Downloading ${count} artifact(s)..."

    echo "${artifacts_json}" | jq -r '.[].name' | while read -r artifact_name; do
        local artifact_dir="${output_dir}/artifacts/${artifact_name}"
        mkdir -p "${artifact_dir}"
        gh run download "${run_id}" --repo "${REPO}" --name "${artifact_name}" --dir "${artifact_dir}" 2>/dev/null || {
            echo "    Warning: Failed to download artifact '${artifact_name}'" >&2
        }
    done
}

# Analyze a log file for error patterns
analyze_log_file() {
    local log_file="$1"
    local job_name="$2"
    local indent="$3"

    if [[ ! -f "${log_file}" ]]; then
        return
    fi

    # Go test failures
    local test_failures
    test_failures="$(grep -En "${GO_TEST_FAIL_PATTERN}" "${log_file}" 2>/dev/null | head -20 || true)"
    if [[ -n "${test_failures}" ]]; then
        echo "${indent}Test Failures:"
        echo "${test_failures}" | while IFS= read -r line; do
            echo "${indent}  ${line}"
        done
        echo ""
    fi

    # Panics
    local panics
    panics="$(grep -En "${PANIC_PATTERN}" "${log_file}" 2>/dev/null | head -10 || true)"
    if [[ -n "${panics}" ]]; then
        echo "${indent}Panics Detected:"
        echo "${panics}" | while IFS= read -r line; do
            echo "${indent}  ${line}"
        done
        echo ""
    fi

    # Timeouts
    local timeouts
    timeouts="$(grep -Ein "${TIMEOUT_PATTERN}" "${log_file}" 2>/dev/null | head -10 || true)"
    if [[ -n "${timeouts}" ]]; then
        echo "${indent}Timeouts:"
        echo "${timeouts}" | while IFS= read -r line; do
            echo "${indent}  ${line}"
        done
        echo ""
    fi
}

# Analyze artifact container/pod logs
analyze_artifacts() {
    local artifacts_dir="$1"
    local indent="$2"

    if [[ ! -d "${artifacts_dir}" ]]; then
        return
    fi

    # Look for container logs with K8s errors
    local found_errors=false
    while IFS= read -r -d '' log_file; do
        local k8s_errors
        k8s_errors="$(grep -Ein "${K8S_ERROR_PATTERN}" "${log_file}" 2>/dev/null | head -5 || true)"
        local radius_errors
        radius_errors="$(grep -Ein "${RADIUS_ERROR_PATTERN}" "${log_file}" 2>/dev/null | head -5 || true)"

        if [[ -n "${k8s_errors}" || -n "${radius_errors}" ]]; then
            local relative_path="${log_file#"${artifacts_dir}"/}"
            echo "${indent}Errors in ${relative_path}:"
            found_errors=true

            if [[ -n "${k8s_errors}" ]]; then
                echo "${k8s_errors}" | while IFS= read -r line; do
                    echo "${indent}  ${line}"
                done
            fi
            if [[ -n "${radius_errors}" ]]; then
                echo "${radius_errors}" | head -5 | while IFS= read -r line; do
                    echo "${indent}  ${line}"
                done
            fi
            echo ""
        fi
    done < <(find "${artifacts_dir}" -type f \( -name "*.log" -o -name "*.txt" \) -print0 2>/dev/null)

    if [[ "${found_errors}" == "false" ]]; then
        echo "${indent}No obvious errors found in artifact logs."
    fi
}

# Analyze a single workflow run
analyze_run() {
    local run_id="$1"
    local run_name="$2"
    local run_url="$3"
    local run_conclusion="$4"

    local run_dir="${TEMP_DIR}/run-${run_id}"
    mkdir -p "${run_dir}/jobs" "${run_dir}/artifacts"

    echo ""
    echo "--- ${run_name} ---"
    echo "  URL: ${run_url}"
    echo "  Status: ${run_conclusion}"
    echo ""

    if [[ "${run_conclusion}" != "failure" ]]; then
        echo "  No failures detected in this run."
        return
    fi

    # Get failed jobs
    local failed_jobs_json
    failed_jobs_json="$(get_failed_jobs "${run_id}")"

    local num_failed
    num_failed="$(echo "${failed_jobs_json}" | jq 'length')"

    if [[ "${num_failed}" -eq 0 ]]; then
        echo "  No individually failed jobs found (run may have been cancelled)."
        return
    fi

    echo "  Found ${num_failed} failed job(s). Downloading logs..."
    echo ""

    # Download and analyze each failed job's logs
    echo "${failed_jobs_json}" | jq -c '.[]' | while IFS= read -r job_json; do
        local job_id job_name job_url
        job_id="$(echo "${job_json}" | jq -r '.id')"
        job_name="$(echo "${job_json}" | jq -r '.name')"
        job_url="$(echo "${job_json}" | jq -r '.html_url')"

        echo "  === Failed Job: ${job_name} ==="
        echo "  Job URL: ${job_url}"
        echo ""

        local log_file
        log_file="$(download_job_logs "${job_id}" "${job_name}" "${run_dir}/jobs")" || continue

        analyze_log_file "${log_file}" "${job_name}" "    "
    done

    # Download and analyze artifacts
    echo "  Downloading artifacts..."
    download_artifacts "${run_id}" "${run_dir}"

    if [[ -d "${run_dir}/artifacts" ]] && find "${run_dir}/artifacts" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | head -1 | grep -q .; then
        echo ""
        echo "  === Artifact Analysis ==="
        analyze_artifacts "${run_dir}/artifacts" "    "
    fi
}

main() {
    validate_requirements

    TEMP_DIR="$(mktemp -d)"
    if [[ ! -d "${TEMP_DIR}" ]]; then
        echo "Error: Failed to create temporary directory." >&2
        exit 1
    fi

    echo "============================================================================"
    echo "Functional Test Failure Analysis"
    echo "============================================================================"

    # If a specific run ID was provided, analyze just that run
    if [[ -n "${RUN_ID}" ]]; then
        echo "Analyzing specific run: ${RUN_ID}"

        local run_json
        run_json="$(gh api "repos/${REPO}/actions/runs/${RUN_ID}" 2>/dev/null || true)"

        if [[ -z "${run_json}" ]]; then
            echo "Error: Could not fetch run ${RUN_ID}." >&2
            exit 1
        fi

        local run_name run_url run_conclusion
        run_name="$(echo "${run_json}" | jq -r '.name')"
        run_url="$(echo "${run_json}" | jq -r '.html_url')"
        run_conclusion="$(echo "${run_json}" | jq -r '.conclusion')"

        analyze_run "${RUN_ID}" "${run_name}" "${run_url}" "${run_conclusion}"

        echo ""
        echo "============================================================================"
        echo "Analysis complete."
        echo "============================================================================"
        return
    fi

    # Auto-detect PR if not specified
    if [[ -z "${PR_NUMBER}" ]]; then
        detect_pr_from_branch
    fi

    echo ""
    echo "Searching for functional test runs for PR #${PR_NUMBER}..."
    echo ""

    local found_failures=false

    # Check cloud functional tests
    echo "Checking cloud functional tests (${CLOUD_WORKFLOW})..."
    local cloud_runs
    cloud_runs="$(find_runs_for_pr "${CLOUD_WORKFLOW}" "${PR_NUMBER}")"

    if [[ -n "${cloud_runs}" && "${cloud_runs}" != "[]" && "${cloud_runs}" != "null" ]]; then
        local latest_cloud
        latest_cloud="$(echo "${cloud_runs}" | jq '.[0]')"

        local cloud_id cloud_conclusion cloud_url cloud_name
        cloud_id="$(echo "${latest_cloud}" | jq -r '.id')"
        cloud_conclusion="$(echo "${latest_cloud}" | jq -r '.conclusion // "in_progress"')"
        cloud_url="$(echo "${latest_cloud}" | jq -r '.html_url')"
        cloud_name="$(echo "${latest_cloud}" | jq -r '.name')"

        echo "  Latest run: ${cloud_url}"
        echo "  Status:     ${cloud_conclusion}"

        if [[ "${cloud_conclusion}" == "failure" ]]; then
            found_failures=true
            analyze_run "${cloud_id}" "${cloud_name}" "${cloud_url}" "${cloud_conclusion}"
        elif [[ "${cloud_conclusion}" == "success" ]]; then
            echo "  ✅ Latest cloud test run passed — no failures to analyze."
        elif [[ "${cloud_conclusion}" == "in_progress" || "${cloud_conclusion}" == "null" ]]; then
            echo "  ⏳ Latest cloud test run is still in progress."
        elif [[ "${cloud_conclusion}" == "cancelled" ]]; then
            echo "  ⚠️  Latest cloud test run was cancelled."
        fi
    else
        echo "  No cloud test runs found for this PR."
    fi

    # Check non-cloud functional tests
    echo ""
    echo "Checking non-cloud functional tests (${NONCLOUD_WORKFLOW})..."
    local noncloud_runs
    noncloud_runs="$(find_runs_for_pr "${NONCLOUD_WORKFLOW}" "${PR_NUMBER}")"

    if [[ -n "${noncloud_runs}" && "${noncloud_runs}" != "[]" && "${noncloud_runs}" != "null" ]]; then
        local latest_noncloud
        latest_noncloud="$(echo "${noncloud_runs}" | jq '.[0]')"

        local noncloud_id noncloud_conclusion noncloud_url noncloud_name
        noncloud_id="$(echo "${latest_noncloud}" | jq -r '.id')"
        noncloud_conclusion="$(echo "${latest_noncloud}" | jq -r '.conclusion // "in_progress"')"
        noncloud_url="$(echo "${latest_noncloud}" | jq -r '.html_url')"
        noncloud_name="$(echo "${latest_noncloud}" | jq -r '.name')"

        echo "  Latest run: ${noncloud_url}"
        echo "  Status:     ${noncloud_conclusion}"

        if [[ "${noncloud_conclusion}" == "failure" ]]; then
            found_failures=true
            analyze_run "${noncloud_id}" "${noncloud_name}" "${noncloud_url}" "${noncloud_conclusion}"
        elif [[ "${noncloud_conclusion}" == "success" ]]; then
            echo "  ✅ Latest non-cloud test run passed — no failures to analyze."
        elif [[ "${noncloud_conclusion}" == "in_progress" || "${noncloud_conclusion}" == "null" ]]; then
            echo "  ⏳ Latest non-cloud test run is still in progress."
        elif [[ "${noncloud_conclusion}" == "cancelled" ]]; then
            echo "  ⚠️  Latest non-cloud test run was cancelled."
        fi
    else
        echo "  No non-cloud test runs found for this PR."
    fi

    echo ""
    echo "============================================================================"

    if [[ "${found_failures}" == "false" ]]; then
        echo "No failures found in the latest functional test runs for PR #${PR_NUMBER}."
    else
        echo "Analysis complete. Review the errors above to identify root cause."
        if [[ "${KEEP_LOGS}" == "true" ]]; then
            echo "Logs preserved at: ${TEMP_DIR}"
        fi
    fi

    echo "============================================================================"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --pr)
            PR_NUMBER="$2"
            shift 2
            ;;
        --run-id)
            RUN_ID="$2"
            shift 2
            ;;
        --repo)
            REPO="$2"
            shift 2
            ;;
        --keep)
            KEEP_LOGS=true
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            ;;
    esac
done

main "$@"
