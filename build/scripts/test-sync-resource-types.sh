#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
# shellcheck source=build/scripts/sync-resource-types.sh
source "${SCRIPT_DIR}/sync-resource-types.sh"
export RADIUS_DEFAULTS_YAML="${REPO_ROOT}/deploy/manifest/defaults.yaml"
# shellcheck source=.github/extension/scripts/contrib-catalog.sh
source "${REPO_ROOT}/.github/extension/scripts/contrib-catalog.sh"

assert_equal() {
    local expected="$1" actual="$2" message="$3"
    if [[ "${actual}" != "${expected}" ]]; then
        echo "FAIL: ${message}: expected '${expected}', got '${actual}'" >&2
        exit 1
    fi
}

test_azure_recipe_pack_pinning() {
    local workflow run_block function_definition fixture function_file warning
    local sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    workflow="${REPO_ROOT}/.github/extension/run-rad-commands-azure.yml"
    run_block="$(
        yq -r '
            .jobs.deploy.steps[] |
            select(.name == "Create Radius environment and recipe pack") |
            .run
        ' "${workflow}"
    )"
    function_definition="$(
        printf '%s\n' "${run_block}" |
            sed -n '/^pin_kube_recipe() {$/,/^}$/p'
    )"
    [[ -n "${function_definition}" ]] ||
        fail "pin_kube_recipe function not found in ${workflow}."

    fixture="$(mktemp -d)"
    function_file="${fixture}/pin-kube-recipe.sh"
    printf '%s\n' "${function_definition}" >"${function_file}"
    (
        # shellcheck disable=SC2329 # Invoked by the sourced workflow function.
        radius_contrib_kube_recipe_source() {
            printf 'ghcr.io/radius-project/kube-recipes/%s:%s' "$2" "${sha}"
        }
        ENV_BICEP="${fixture}/recipe-pack.bicep"
        : >"${ENV_BICEP}"
        # shellcheck disable=SC1090 # Generated from the workflow under test.
        source "${function_file}"

        warning="$(pin_kube_recipe Radius.Compute/containers containers 2>&1)"
        grep -Fq \
            "WARNING: the Azure recipe pack has no Radius.Compute/containers Recipe" \
            <<<"${warning}" || fail "missing Recipe did not produce a warning."

        printf "source: 'ghcr.io/radius-project/kube-recipes/containers:latest'\n" \
            >"${ENV_BICEP}"
        pin_kube_recipe Radius.Compute/containers containers
        grep -Fq \
            "source: 'ghcr.io/radius-project/kube-recipes/containers:${sha}'" \
            "${ENV_BICEP}" || fail "mutable Recipe source was not pinned."

        warning="$(pin_kube_recipe Radius.Compute/containers containers 2>&1)"
        [[ -z "${warning}" ]] || fail "already-pinned Recipe produced a warning."
    )
    rm -rf "${fixture}"
}

# Stub the one git operation exercised by the selection helpers. The fixtures
# deliberately include a newer prerelease and numerically ambiguous versions.
git() {
    [[ "$1" == "ls-remote" ]] || return 2
    if [[ "$2" == "--tags" && "$3" == "--refs" ]]; then
        case "$5" in
            refs/tags/Radius.Compute/v\*)
                printf '%s\t%s\n' \
                    1111111111111111111111111111111111111111 \
                    refs/tags/Radius.Compute/v1.9.0
                printf '%s\t%s\n' \
                    2222222222222222222222222222222222222222 \
                    refs/tags/Radius.Compute/v1.10.0
                printf '%s\t%s\n' \
                    3333333333333333333333333333333333333333 \
                    refs/tags/Radius.Compute/v2.0.0-rc.1
                printf '%s\t%s\n' \
                    9999999999999999999999999999999999999999 \
                    refs/tags/v9.0.0
                ;;
            refs/tags/Radius.Data/v\*)
                printf '%s\t%s\n' \
                    4444444444444444444444444444444444444444 \
                    refs/tags/Radius.Data/v1.0.0-rc.1
                printf '%s\t%s\n' \
                    6666666666666666666666666666666666666666 \
                    refs/tags/v0.8.0
                ;;
            refs/tags/Radius.Security/v\*)
                printf '%s\t%s\n' \
                    7777777777777777777777777777777777777777 \
                    refs/tags/Radius.Security/v1.0.0-rc.1
                ;;
            refs/tags/Radius.Failure/v\*) return 42 ;;
            refs/tags/recipe-pack/azure/v\*)
                printf '%s\t%s\n' \
                    5555555555555555555555555555555555555555 \
                    refs/tags/recipe-pack/azure/v0.3.0
                ;;
            *) ;;
        esac
        return 0
    fi
    case "$3" in
        refs/heads/Radius.Compute/v1.10.0)
            # A same-named branch must not beat the annotated stable tag.
            printf '%s\t%s\n' \
                aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
                refs/heads/Radius.Compute/v1.10.0
            printf '%s\t%s\n' \
                bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
                refs/tags/Radius.Compute/v1.10.0
            printf '%s\t%s\n' \
                2222222222222222222222222222222222222222 \
                'refs/tags/Radius.Compute/v1.10.0^{}'
            ;;
        refs/heads/Radius.Compute/v3.0.0)
            printf '%s\t%s\n' \
                cccccccccccccccccccccccccccccccccccccccc \
                refs/heads/Radius.Compute/v3.0.0
            ;;
        *) ;;
    esac
}

main() {
    local edge_sha resolved_sha resolved_tag azure_ref kubernetes_ref
    local data_repo data_ref workflow
    edge_sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

    is_stable_tag resourceTypes Radius.Compute Radius.Compute/v1.10.0 ||
        fail "stable namespace tag was rejected."
    if is_stable_tag resourceTypes Radius.Compute Radius.Compute/v2.0.0-rc.1; then
        fail "prerelease namespace tag was accepted as stable."
    fi
    if is_stable_tag resourceTypes Radius.Compute Radius.Data/v1.0.0; then
        fail "a stable tag for the wrong namespace was accepted."
    fi

    assert_equal \
        Radius.Compute/v1.10.0 \
        "$(latest_stable_tag example.invalid resourceTypes Radius.Compute)" \
        "latest stable namespace release"
    assert_equal \
        v0.8.0 \
        "$(latest_stable_tag example.invalid resourceTypes Radius.Data)" \
        "repository-wide stable fallback"
    assert_equal \
        Radius.Compute/v1.10.0 \
        "$(preferred_ref example.invalid resourceTypes Radius.Compute main)" \
        "stable release preferred over edge"
    assert_equal \
        Radius.Compute/v1.9.0 \
        "$(preferred_ref example.invalid resourceTypes Radius.Compute Radius.Compute/v1.9.0)" \
        "explicit stable rollback"
    assert_equal \
        Radius.Compute/v1.10.0 \
        "$(preferred_ref example.invalid resourceTypes Radius.Compute v9.0.0)" \
        "scoped stable release preferred over explicit global tag"
    IFS=$'\t' read -r resolved_sha resolved_tag < <(
        resolve_pin example.invalid Radius.Compute/v1.10.0
    )
    assert_equal \
        2222222222222222222222222222222222222222 \
        "${resolved_sha}" \
        "stable tag SHA wins over same-named branch"
    assert_equal \
        Radius.Compute/v1.10.0 \
        "${resolved_tag}" \
        "resolved stable tag is retained"
    if (
        pin_field() { printf '%s' example.invalid; }
        write_pin() { :; }
        pin_one resourceTypes Radius.Compute Radius.Compute/v3.0.0
    ) >/dev/null 2>&1; then
        fail "a same-named branch was accepted without the stable tag."
    fi
    if latest_stable_tag \
        example.invalid \
        resourceTypes \
        Radius.Failure >/dev/null 2>&1; then
        fail "stable tag lookup failure was converted into success."
    fi
    if (
        verify_commit_exists() { return 1; }
        load_resolved_pin example.invalid "${edge_sha}"
    ) >/dev/null 2>&1; then
        fail "an unverified commit SHA was accepted."
    fi
    assert_equal \
        "${edge_sha}" \
        "$(preferred_ref example.invalid resourceTypes Radius.Security "${edge_sha}")" \
        "edge fallback without a stable release"
    if (
        preferred_ref \
            example.invalid \
            resourceTypes \
            Radius.Security \
            Radius.Security/v1.0.0-rc.1
    ) >/dev/null 2>&1; then
        fail "prerelease tag was accepted as the no-stable edge fallback."
    fi
    assert_equal \
        recipe-pack/azure/v0.3.0 \
        "$(preferred_ref example.invalid recipePacks azure main)" \
        "stable recipe pack release"

    has_pins '[{"name":"Radius.Compute","ref":"main"}]' ||
        fail "non-empty pin array was treated as empty."
    if has_pins '[]'; then
        fail "literal empty pin array was treated as a payload."
    fi

    if (
        # shellcheck disable=SC2329 # Invoked indirectly by validate_section_pins.
        entry_names() { printf '%s\n' Radius.Compute; }
        # shellcheck disable=SC2329 # Invoked indirectly by validate_section_pins.
        pin_field() {
            case "$3" in
                repo) printf '%s' example.invalid ;;
                ref) printf '%s' 9999999999999999999999999999999999999999 ;;
                tag) printf '%s' v9.0.0 ;;
            esac
        }
        validate_section_pins resourceTypes
    ) >/dev/null 2>&1; then
        fail "global release was accepted while a scoped release exists."
    fi

    azure_ref="$(radius_contrib_catalog_field recipePacks azure ref)"
    kubernetes_ref="$(radius_contrib_catalog_field recipePacks kubernetes ref)"
    assert_equal \
        "${azure_ref}" \
        "$(radius_contrib_recipe_pack_url azure aks-recipepack.bicep | sed -E 's|.*/([0-9a-f]{40})/recipe-packs/.*|\1|')" \
        "Azure recipe pack catalog ref"
    [[ "${kubernetes_ref}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "Kubernetes recipe pack is not pinned in defaults.yaml."
    assert_equal \
        "${kubernetes_ref}" \
        "$(radius_contrib_recipe_pack_url kubernetes default-recipepack.bicep | sed -E 's|.*/([0-9a-f]{40})/recipe-packs/.*|\1|')" \
        "Kubernetes recipe pack catalog ref"

    data_repo="$(radius_contrib_catalog_field resourceTypes Radius.Data repo)"
    data_ref="$(radius_contrib_catalog_field resourceTypes Radius.Data ref)"
    assert_equal \
        "git::https://${data_repo}.git//Data/mySqlDatabases/recipes/aws/terraform?ref=${data_ref}" \
        "$(radius_contrib_resource_git_source Radius.Data/mySqlDatabases recipes/aws/terraform)" \
        "resource Recipe source from catalog"

    for workflow in \
        "${REPO_ROOT}/.github/extension/run-rad-commands-azure.yml" \
        "${REPO_ROOT}/.github/extension/run-rad-commands-aws.yml"; do
        if grep -Eq '^[[:space:]]*(COMPUTE_RESOURCE_TYPES_REF|DATA_RESOURCE_TYPES_REF|SECURITY_RESOURCE_TYPES_REF|RECIPE_PACK_REF|RESOURCE_TYPES_CONTRIB_REF|RESOURCE_TYPES_CONTRIB_REPO):' "${workflow}"; then
            fail "workflow contains a mirrored contrib ref: ${workflow}"
        fi
        # shellcheck disable=SC2016 # Match the literal workflow variable.
        grep -Fq 'source "$RADIUS_CONTRIB_CATALOG_HELPER"' "${workflow}" ||
            fail "workflow does not consume the shared defaults catalog: ${workflow}"
    done

    test_azure_recipe_pack_pinning

    invariant_fixture="$(mktemp -d)"
    cat >"${invariant_fixture}/bad.yml" <<'EOF'
---
name: bad
env:
    renamed_pin: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
jobs: {}
EOF
    if EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null 2>&1; then
        fail "generic source-of-truth guard accepted a renamed SHA variable."
    fi
    cat >"${invariant_fixture}/bad.yml" <<'EOF'
---
name: bad
env:
  numeric_pin: 1111111111111111111111111111111111111111
jobs: {}
EOF
    if EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null 2>&1; then
        fail "generic source-of-truth guard accepted an unquoted numeric SHA."
    fi
    cat >"${invariant_fixture}/bad.yml" <<'EOF'
---
name: bad
jobs:
    test:
        runs-on: ubuntu-latest
        steps:
            - run: curl https://raw.githubusercontent.com/Radius-Project/Resource-Types-Contrib/main/file
EOF
    if EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null 2>&1; then
        fail "generic source-of-truth guard accepted a direct contrib URL."
    fi
    cat >"${invariant_fixture}/bad.yml" <<'EOF'
---
name: bad
env:
    renamed_ref: "main"
jobs: {}
EOF
    if EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null 2>&1; then
        fail "generic source-of-truth guard accepted a moving ref."
    fi
    cat >"${invariant_fixture}/bad.yml" <<'EOF'
---
name: bad
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCD
EOF
    if EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null 2>&1; then
        fail "generic source-of-truth guard accepted an uppercase SHA in a run block."
    fi
    cat >"${invariant_fixture}/good.yml" <<'EOF'
---
name: good
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
EOF
    rm "${invariant_fixture}/bad.yml"
    EXTENSION_DIR="${invariant_fixture}" \
        bash "${SCRIPT_DIR}/verify-contrib-consumers.sh" \
        --source-of-truth-only >/dev/null
    rm -rf "${invariant_fixture}"

    actual_order="$(
        (
            update_section() { echo "update:$1"; }
            apply_pins() { echo "apply:$1"; }
            promote_edge_pins() { echo "promote:$1"; }
            validate_section_pins() { echo "validate:$1"; }
            update_all_sections
        )
    )"
    expected_order="$(
        cat <<'EOF'
update:resourceTypes
update:recipePacks
promote:resourceTypes
promote:recipePacks
validate:resourceTypes
validate:recipePacks
EOF
    )"
    assert_equal \
        "${expected_order}" \
        "${actual_order}" \
        "atomic update ordering"

    echo "Stable-first resource type pin tests passed."
}

main "$@"
