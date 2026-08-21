#!/bin/bash

# Shared live deployment progress generation and polling lifecycle.

radius_resolve_application_name() {
    local app_file="$1"

    sed -nE "s/.*name:[[:space:]]*'([^']+)'.*/\1/p" \
        "${app_file}" 2>/dev/null | head -1 || true
}

radius_deploy_artifact_name() {
    local environment="$1"
    local application="$2"
    local base_name

    base_name=$(printf '%s-%s' "${environment}" "${application}" |
        LC_ALL=C tr '[:upper:]' '[:lower:]' |
        LC_ALL=C sed -E \
            's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//' |
        LC_ALL=C cut -c1-80)
    [[ -n "${base_name}" ]] || base_name="deploy-status"
    printf 'radius-deploy-status-%s' "${base_name}"
}

radius_progress_dir() {
    printf '%s/radius-deploy-progress' "${RUNNER_TEMP:-/tmp}"
}

radius_progress_checkpoint() {
    printf '%s/sequence' "$(radius_progress_dir)"
}

radius_progress_stop_file() {
    printf '%s/stop' "$(radius_progress_dir)"
}

radius_artifact_runtime_file() {
    printf '%s/artifact-runtime.json' "$(radius_progress_dir)"
}

radius_load_artifact_runtime() {
    local runtime_file runtime_token results_url

    runtime_file=$(radius_artifact_runtime_file)
    if [[ ! -f "${runtime_file}" ]]; then
        echo "::warning::Artifact runtime was not prepared; live deployment progress is disabled."
        return 1
    fi
    if ! runtime_token=$(jq -er \
        '.runtimeToken | select(type == "string" and length > 0)' \
        "${runtime_file}") ||
        ! results_url=$(jq -er \
            '.resultsUrl | select(type == "string" and length > 0)' \
            "${runtime_file}"); then
        echo "::warning::Artifact runtime is invalid; live deployment progress is disabled."
        return 1
    fi

    export ACTIONS_RUNTIME_TOKEN="${runtime_token}"
    export ACTIONS_RESULTS_URL="${results_url}"
}

radius_clear_artifact_runtime() {
    rm -f "$(radius_artifact_runtime_file)"
    unset ACTIONS_RUNTIME_TOKEN ACTIONS_RESULTS_URL
}

radius_last_live_sequence() {
    local checkpoint
    local sequence="0"

    checkpoint=$(radius_progress_checkpoint)
    if [[ -f "${checkpoint}" ]]; then
        sequence=$(<"${checkpoint}")
    fi
    [[ "${sequence}" =~ ^[0-9]+$ ]] || sequence="0"
    printf '%s' "${sequence}"
}

radius_next_terminal_sequence() {
    printf '%s' "$(( $(radius_last_live_sequence) + 1 ))"
}

radius_normalize_resources() {
    jq -ce '
        [ .[]? | {
            id: (.id // ""),
            name: (.name // ""),
            type: (.type // ""),
            provisioningState: (.properties.provisioningState // ""),
            outputResourceIds: (
                [
                    (.properties.status.outputResources // [])[]?
                    | .id // empty
                    | select(type == "string" and length > 0)
                ]
                | sort
                | unique
            ),
            status: (
                (.properties.provisioningState // "") as $state
                | if $state == "Succeeded" then "success"
                  elif $state == "Failed"
                    or $state == "Canceled"
                    or $state == "Cancelled" then "failed"
                  else "in_progress"
                  end
            ),
            message: (.properties.status.message // "")
        } ] | sort_by(.id, .type, .name)
    '
}

radius_publish_live_progress_once() {
    local application="$1"
    local environment="$2"
    local terminal_artifact_name="$3"
    local progress_dir raw_resources resources previous sequence slot
    local progress_file artifact_name replace_existing updated_at output

    progress_dir=$(radius_progress_dir)
    mkdir -p "${progress_dir}"

    if ! raw_resources=$(rad resource list --preview \
        --application "${application}" --output json 2>/dev/null); then
        echo "::warning::Live Radius resource polling failed; retaining the previous snapshot."
        return 0
    fi
    if ! resources=$(printf '%s' "${raw_resources}" |
        radius_normalize_resources 2>/dev/null); then
        echo "::warning::Live Radius resource polling returned invalid JSON; retaining the previous snapshot."
        return 0
    fi

    previous="${progress_dir}/last-resources.json"
    if [[ -f "${previous}" ]] && cmp -s <(printf '%s\n' "${resources}") \
        "${previous}"; then
        return 0
    fi

    sequence=$(( $(radius_last_live_sequence) + 1 ))
    slot=$(( (sequence - 1) % 8 ))
    progress_file="${progress_dir}/deploy-progress.json"
    artifact_name="${terminal_artifact_name}-live-${GITHUB_RUN_ID:-0}-slot-${slot}"
    replace_existing=false
    (( sequence > 8 )) && replace_existing=true
    updated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n \
        --argjson resources "${resources}" \
        --arg application "${application}" \
        --arg environment "${environment}" \
        --argjson runId "${GITHUB_RUN_ID:-0}" \
        --argjson sequence "${sequence}" \
        --arg updatedAt "${updated_at}" \
        '{
            schemaVersion: 1,
            application: $application,
            environment: $environment,
            runId: $runId,
            sequence: $sequence,
            updatedAt: $updatedAt,
            state: "in_progress",
            resources: $resources
        }' >"${progress_file}"

    if output=$("${RADIUS_PROGRESS_NODE:-node}" \
        "${RADIUS_PROGRESS_UPLOADER}" \
        "${artifact_name}" \
        "${progress_file}" \
        "1" \
        "${replace_existing}"); then
        printf '%s\n' "${resources}" >"${previous}"
        printf '%s\n' "${sequence}" >"$(radius_progress_checkpoint)"
        echo "Published live deployment progress sequence ${sequence}."
    else
        echo "::warning::Live deployment progress upload failed; deployment will continue."
        [[ -z "${output}" ]] || echo "${output}"
    fi
}

start_live_deploy_progress() {
    local app_file="$1"
    local environment="$2"
    local application terminal_artifact_name interval stop_file

    if ! radius_load_artifact_runtime; then
        return 0
    fi

    application=$(radius_resolve_application_name "${app_file}")
    if [[ -z "${application}" ]]; then
        echo "::warning::Could not determine application name; live deployment progress is disabled."
        return 0
    fi

    terminal_artifact_name=$(radius_deploy_artifact_name \
        "${environment}" "${application}")
    interval="${RADIUS_PROGRESS_INTERVAL_SECONDS:-5}"
    stop_file=$(radius_progress_stop_file)
    rm -f "${stop_file}"
    (
        set +e
        while [[ ! -f "${stop_file}" ]]; do
            radius_publish_live_progress_once "${application}" \
                "${environment}" "${terminal_artifact_name}"
            sleep "${interval}"
        done
    ) &
    RADIUS_PROGRESS_PID=$!
    RADIUS_PROGRESS_APPLICATION="${application}"
    RADIUS_PROGRESS_ENVIRONMENT="${environment}"
    RADIUS_PROGRESS_ARTIFACT_NAME="${terminal_artifact_name}"
}

stop_live_deploy_progress() {
    if [[ -z "${RADIUS_PROGRESS_PID:-}" ]]; then
        return 0
    fi

    touch "$(radius_progress_stop_file)"
    wait "${RADIUS_PROGRESS_PID}" 2>/dev/null || true
    RADIUS_PROGRESS_PID=""
    rm -f "$(radius_progress_stop_file)"
    radius_publish_live_progress_once \
        "${RADIUS_PROGRESS_APPLICATION}" \
        "${RADIUS_PROGRESS_ENVIRONMENT}" \
        "${RADIUS_PROGRESS_ARTIFACT_NAME}" || true
}
