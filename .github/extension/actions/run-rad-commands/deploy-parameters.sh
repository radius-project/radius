#!/bin/bash

# Discovers the parameters declared by the application template and appends only
# compatible extension-generated values to a rad deploy argument array.

declare -a DECLARED_APP_PARAMS=()
DECLARED_APP_PARAMS_LOADED=false
declare -a GENERATED_APP_PARAMS=()

load_declared_app_params() {
    local app_file="$1"
    local bicep_bin="${BICEP_BIN:-${BICEP:-${HOME}/.rad/bin/bicep}}"
    local compiled_template
    local declared_parameters

    if [[ "${DECLARED_APP_PARAMS_LOADED}" == "true" ]]; then
        return 0
    fi

    # Fast-fail with a clear message when the application file is missing on the
    # deployed branch/commit, rather than surfacing a confusing Bicep compile
    # error below. This commonly happens when a deploy is dispatched before the
    # generated app.bicep has been committed and pushed to the deployed branch.
    if [[ ! -f "${app_file}" ]]; then
        # Escape workflow-command metacharacters in the interpolated path before
        # embedding it in the ::error:: annotation: '%' is the escape character
        # (must be first), and CR/LF would otherwise break or inject the command.
        # The assignments are intentionally unquoted so the single quotes around
        # the patterns/replacements are treated as quoting, not literal text.
        local escaped_app_file="${app_file}"
        escaped_app_file=${escaped_app_file//'%'/'%25'}
        escaped_app_file=${escaped_app_file//$'\r'/'%0D'}
        escaped_app_file=${escaped_app_file//$'\n'/'%0A'}
        echo "::error::Application file ${escaped_app_file} not found on this branch/commit. Generate it (ask Copilot to \"create app.bicep\") and commit it to the deployed branch before deploying." >&2
        return 1
    fi

    if [[ -d "${bicep_bin}" ]]; then
        bicep_bin="${bicep_bin}/bicep"
    fi
    if [[ ! -x "${bicep_bin}" ]]; then
        echo "::error::Bicep compiler not found at ${bicep_bin}." >&2
        return 1
    fi

    if ! compiled_template="$(
        mktemp "${TMPDIR:-/tmp}/radius-app-template.XXXXXX"
    )"; then
        echo "::error::Failed to create temporary Bicep output file." >&2
        return 1
    fi
    if ! "${bicep_bin}" build "${app_file}" --outfile "${compiled_template}"; then
        rm -f "${compiled_template}"
        echo "::error::Failed to compile ${app_file} before deployment." >&2
        return 1
    fi

    if ! jq -e '(.parameters // {}) | type == "object"' \
        "${compiled_template}" >/dev/null; then
        rm -f "${compiled_template}"
        echo "::error::Compiled template parameters are not a JSON object." >&2
        return 1
    fi

    if ! declared_parameters="$(
        jq -r '(.parameters // {}) | keys[]' "${compiled_template}"
    )"; then
        rm -f "${compiled_template}"
        echo "::error::Failed to read parameters from compiled template." >&2
        return 1
    fi

    DECLARED_APP_PARAMS=()
    while IFS= read -r parameter; do
        if [[ -n "${parameter}" ]]; then
            DECLARED_APP_PARAMS+=("${parameter}")
        fi
    done <<<"${declared_parameters}"
    rm -f "${compiled_template}"
    DECLARED_APP_PARAMS_LOADED=true
}

app_declares_parameter() {
    local expected="$1"
    local parameter

    for parameter in "${DECLARED_APP_PARAMS[@]-}"; do
        if [[ "${parameter}" == "${expected}" ]]; then
            return 0
        fi
    done

    return 1
}

append_generated_app_params() {
    GENERATED_APP_PARAMS=()

    if [[ -n "${APP_IMAGE}" ]] && app_declares_parameter "image"; then
        GENERATED_APP_PARAMS+=(--parameters "image=${APP_IMAGE}")
    fi
    if [[ -n "${REGISTRY_USERNAME}" ]] &&
        app_declares_parameter "registryUsername" &&
        ! deploy_params_has_key "registryUsername"; then
        GENERATED_APP_PARAMS+=(
            --parameters "registryUsername=${REGISTRY_USERNAME}"
        )
    fi
    if [[ -n "${REGISTRY_PASSWORD}" ]] &&
        app_declares_parameter "registryPassword" &&
        ! deploy_params_has_key "registryPassword"; then
        GENERATED_APP_PARAMS+=(
            --parameters "registryPassword=${REGISTRY_PASSWORD}"
        )
    fi
    # Target-cluster architecture-aware container builds. When the deploy computed
    # an effective platform list (see compute-build-platforms.sh) and the app
    # declares a `platforms` parameter, pass it so Radius.Compute/containerImages
    # builds only the needed platform(s) instead of the recipe's multi-arch
    # default (which builds arm64 under QEMU emulation on an amd64 runner). Apps
    # that do not declare `platforms` are unaffected, and a value already supplied
    # via RADIUS_DEPLOY_PARAMS is not overridden.
    if [[ -n "${RADIUS_EFFECTIVE_BUILD_PLATFORMS:-}" ]] &&
        app_declares_parameter "platforms" &&
        ! deploy_params_has_key "platforms"; then
        GENERATED_APP_PARAMS+=(
            --parameters "platforms=${RADIUS_EFFECTIVE_BUILD_PLATFORMS}"
        )
    fi
}
