#!/usr/bin/env bash

# Generic accessors for deploy/manifest/defaults.yaml. This file is sourced by
# generated workflows after load-contrib-catalog exports RADIUS_DEFAULTS_YAML.

radius_contrib_catalog_field() {
    local section="$1" name="$2" field="$3" value

    case "${section}" in
        resourceTypes | recipePacks) ;;
        *)
            echo "ERROR: unsupported contrib catalog section '${section}'." >&2
            return 1
            ;;
    esac
    case "${field}" in
        repo | ref) ;;
        *)
            echo "ERROR: unsupported contrib catalog field '${field}'." >&2
            return 1
            ;;
    esac
    [[ "${name}" =~ ^[A-Za-z0-9._-]+$ ]] || {
        echo "ERROR: invalid contrib catalog name '${name}'." >&2
        return 1
    }
    [[ -f "${RADIUS_DEFAULTS_YAML:-}" ]] || {
        echo "ERROR: Radius defaults catalog is unavailable." >&2
        return 1
    }
    command -v yq >/dev/null 2>&1 || {
        echo "ERROR: yq is required to read the Radius defaults catalog." >&2
        return 1
    }

    if ! value="$(
        yq -e -r "
            [.${section}[] | select(.name == \"${name}\")] as \$matches |
            select(\$matches | length == 1) |
            \$matches[0].${field} |
            select(type == \"!!str\" and length > 0)
        " "${RADIUS_DEFAULTS_YAML}"
    )"; then
        echo "ERROR: expected exactly one ${section} entry '${name}' with field '${field}'." >&2
        return 1
    fi

    if [[ "${field}" == ref && ! "${value}" =~ ^[0-9a-f]{40}$ ]]; then
        echo "ERROR: ${section} entry '${name}' does not pin a lowercase commit SHA." >&2
        return 1
    fi
    if [[ "${field}" == repo && ! "${value}" =~ ^github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
        echo "ERROR: ${section} entry '${name}' has unsupported repository '${value}'." >&2
        return 1
    fi
    printf '%s' "${value}"
}

radius_contrib_validate_path() {
    local path="$1"
    [[ -n "${path}" && "${path}" != /* && "${path}" != *..* && "${path}" =~ ^[A-Za-z0-9._/-]+$ ]]
}

radius_contrib_resource_git_source() {
    local resource_type="$1" recipe_path="$2" namespace repo ref resource_path
    [[ "${resource_type}" =~ ^Radius\.[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || {
        echo "ERROR: invalid Radius resource type '${resource_type}'." >&2
        return 1
    }
    radius_contrib_validate_path "${recipe_path}" || {
        echo "ERROR: invalid recipe path '${recipe_path}'." >&2
        return 1
    }

    namespace="${resource_type%%/*}"
    resource_path="${resource_type#Radius.}"
    repo="$(radius_contrib_catalog_field resourceTypes "${namespace}" repo)" || return 1
    ref="$(radius_contrib_catalog_field resourceTypes "${namespace}" ref)" || return 1
    printf 'git::https://%s.git//%s/%s?ref=%s' "${repo}" "${resource_path}" "${recipe_path}" "${ref}"
}

radius_contrib_kube_recipe_source() {
    local resource_type="$1" artifact="$2" namespace ref
    [[ "${resource_type}" =~ ^Radius\.[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || {
        echo "ERROR: invalid Radius resource type '${resource_type}'." >&2
        return 1
    }
    [[ "${artifact}" =~ ^[a-z0-9._-]+$ ]] || {
        echo "ERROR: invalid kube recipe artifact '${artifact}'." >&2
        return 1
    }

    namespace="${resource_type%%/*}"
    ref="$(radius_contrib_catalog_field resourceTypes "${namespace}" ref)" || return 1
    printf 'ghcr.io/radius-project/kube-recipes/%s:%s' "${artifact}" "${ref}"
}

radius_contrib_recipe_pack_url() {
    local pack="$1" file="$2" repo ref repo_path
    [[ "${pack}" =~ ^[A-Za-z0-9._-]+$ ]] || {
        echo "ERROR: invalid recipe pack '${pack}'." >&2
        return 1
    }
    radius_contrib_validate_path "${file}" || {
        echo "ERROR: invalid recipe pack file '${file}'." >&2
        return 1
    }

    repo="$(radius_contrib_catalog_field recipePacks "${pack}" repo)" || return 1
    ref="$(radius_contrib_catalog_field recipePacks "${pack}" ref)" || return 1
    repo_path="${repo#github.com/}"
    printf 'https://raw.githubusercontent.com/%s/%s/recipe-packs/%s/%s' \
        "${repo_path}" "${ref}" "${pack}" "${file}"
}
