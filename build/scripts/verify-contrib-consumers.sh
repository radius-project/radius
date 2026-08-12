#!/usr/bin/env bash

set -euo pipefail

readonly DEFAULTS_YAML="${DEFAULTS_YAML:-deploy/manifest/defaults.yaml}"
readonly CATALOG_HELPER="${CATALOG_HELPER:-.github/extension/scripts/contrib-catalog.sh}"
readonly EXTENSION_DIR="${EXTENSION_DIR:-.github/extension}"

export RADIUS_DEFAULTS_YAML="${DEFAULTS_YAML}"
# shellcheck source=.github/extension/scripts/contrib-catalog.sh
source "${CATALOG_HELPER}"

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

require_tools() {
    local tool
    for tool in curl docker git yq; do
        command -v "${tool}" >/dev/null 2>&1 ||
            fail "${tool} is required to verify contrib consumers."
    done
}

verify_single_source_of_truth() {
    local file run_blocks scalar_values violations
    local count=0
    while IFS= read -r -d '' file; do
        ((count += 1))

        violations="$(
            grep -Ein \
                -e 'resource-types-contrib' \
                -e 'ghcr\.io/radius-project/kube-recipes/' \
                "${file}" || true
        )"
        [[ -z "${violations}" ]] ||
            fail "${file} contains a direct contrib source instead of the defaults catalog:"$'\n'"${violations}"

        scalar_values="$(
            yq -r \
                '.. | select(tag != "!!map" and tag != "!!seq" and tag != "!!null")' \
                "${file}"
        )"
        violations="$(
            printf '%s\n' "${scalar_values}" |
                grep -Ein -e '^[0-9a-f]{40}$' -e '^(main|latest|edge)$' || true
        )"
        [[ -z "${violations}" ]] ||
            fail "${file} contains a standalone revision outside defaults.yaml:\n${violations}"

        run_blocks="$(
            yq -r \
                '.. | select(tag == "!!map") | .run? | select(tag == "!!str")' \
                "${file}"
        )"
        if printf '%s\n' "${run_blocks}" |
            grep -Eqi '(^|[^0-9a-f])[0-9a-f]{40}([^0-9a-f]|$)'; then
            fail "${file} embeds a commit SHA in a run block instead of resolving the defaults catalog."
        fi
    done < <(extension_yaml_files)
    ((count > 0)) || fail "no extension workflows or actions found under ${EXTENSION_DIR}."
}

extension_yaml_files() {
    find "${EXTENSION_DIR}" -type f \
        \( -name '*.yml' -o -name '*.yaml' \) -print0 |
        sort -z
}

extract_run_blocks() {
    local workflow
    while IFS= read -r -d '' workflow; do
        yq -r \
            '.. | select(tag == "!!map") | .run? | select(tag == "!!str")' \
            "${workflow}"
    done < <(extension_yaml_files)
}

verify_recipe_packs() {
    local pack file url count=0
    while read -r pack file; do
        [[ -n "${pack}" ]] || continue
        url="$(radius_contrib_recipe_pack_url "${pack}" "${file}")"
        curl -fsSL "${url}" -o /dev/null
        echo "  Verified recipe pack file ${pack}/${file}"
        ((count += 1))
    done < <(
        printf '%s\n' "${RUN_BLOCKS}" |
            sed -nE 's/.*radius_contrib_recipe_pack_url ([A-Za-z0-9._-]+) ([A-Za-z0-9._\/-]+).*/\1 \2/p' |
            sort -u
    )
    ((count > 0)) || fail "no recipe pack catalog consumers found."
}

verify_kube_recipes() {
    local resource_type artifact source count=0
    while read -r resource_type artifact; do
        [[ -n "${resource_type}" ]] || continue
        source="$(radius_contrib_kube_recipe_source "${resource_type}" "${artifact}")"
        docker manifest inspect "${source}" >/dev/null
        echo "  Verified OCI recipe ${source}"
        ((count += 1))
    done < <(
        printf '%s\n' "${RUN_BLOCKS}" |
            sed -nE \
                -e 's/.*radius_contrib_kube_recipe_source (Radius\.[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+) ([a-z0-9._-]+).*/\1 \2/p' \
                -e 's/^[[:space:]]*pin_kube_recipe (Radius\.[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+) ([a-z0-9._-]+).*/\1 \2/p' |
            sort -u
    )
    ((count > 0)) || fail "no OCI recipe catalog consumers found."
}

checkout_for() {
    local repo="$1" ref="$2" key dir
    key="${repo}@${ref}"
    if [[ -n "${CHECKOUTS[${key}]:-}" ]]; then
        CHECKOUT_DIR="${CHECKOUTS[${key}]}"
        return 0
    fi

    dir="${TMP_ROOT}/checkout_${#CHECKOUTS[@]}"
    git init -q "${dir}"
    git -C "${dir}" remote add origin "https://${repo}.git"
    git -C "${dir}" fetch -q --depth 1 origin "${ref}"
    git -C "${dir}" checkout -q FETCH_HEAD
    CHECKOUTS["${key}"]="${dir}"
    CHECKOUT_DIR="${dir}"
}

verify_git_recipes() {
    local resource_type recipe_path namespace repo ref resource_path count=0
    while read -r resource_type recipe_path; do
        [[ -n "${resource_type}" ]] || continue
        namespace="${resource_type%%/*}"
        resource_path="${resource_type#Radius.}"
        repo="$(radius_contrib_catalog_field resourceTypes "${namespace}" repo)"
        ref="$(radius_contrib_catalog_field resourceTypes "${namespace}" ref)"
        checkout_for "${repo}" "${ref}"
        [[ -d "${CHECKOUT_DIR}/${resource_path}/${recipe_path}" ]] ||
            fail "Recipe directory not found: ${resource_path}/${recipe_path} at ${repo}@${ref}."
        echo "  Verified Git recipe ${resource_type}/${recipe_path}"
        ((count += 1))
    done < <(
        printf '%s\n' "${RUN_BLOCKS}" |
            sed -nE 's/.*radius_contrib_resource_git_source (Radius\.[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+) ([A-Za-z0-9._\/-]+).*/\1 \2/p' |
            sort -u
    )
    ((count > 0)) || fail "no Git recipe catalog consumers found."
}

main() {
    command -v yq >/dev/null 2>&1 ||
        fail "yq is required to verify contrib consumers."
    [[ -d "${EXTENSION_DIR}" ]] || fail "extension directory not found: ${EXTENSION_DIR}"
    verify_single_source_of_truth
    if [[ "${1:-}" == --source-of-truth-only ]]; then
        echo "Extension workflows use defaults.yaml as their sole contrib ref source."
        return 0
    fi
    [[ -z "${1:-}" ]] || fail "unknown argument: $1"

    require_tools
    [[ -f "${DEFAULTS_YAML}" ]] || fail "defaults catalog not found: ${DEFAULTS_YAML}"
    [[ -f "${CATALOG_HELPER}" ]] || fail "catalog helper not found: ${CATALOG_HELPER}"

    TMP_ROOT="$(mktemp -d)"
    readonly TMP_ROOT
    trap 'rm -rf "${TMP_ROOT}"' EXIT
    declare -g -A CHECKOUTS=()
    CHECKOUT_DIR=""
    RUN_BLOCKS="$(extract_run_blocks)"
    readonly RUN_BLOCKS

    verify_recipe_packs
    verify_kube_recipes
    verify_git_recipes
    echo "Contrib workflow consumers are valid."
}

main "$@"
