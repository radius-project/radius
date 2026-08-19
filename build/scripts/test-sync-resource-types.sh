#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
# shellcheck source=build/scripts/sync-resource-types.sh
source "${SCRIPT_DIR}/sync-resource-types.sh"

assert_equal() {
    local expected="$1" actual="$2" message="$3"
    if [[ "${actual}" != "${expected}" ]]; then
        echo "FAIL: ${message}: expected '${expected}', got '${actual}'" >&2
        exit 1
    fi
}

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
    local edge_sha resolved_sha resolved_tag
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
