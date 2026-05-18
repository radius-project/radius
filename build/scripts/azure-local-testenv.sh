#!/usr/bin/env bash
# ------------------------------------------------------------
# Copyright 2024 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
# ------------------------------------------------------------
#
# Orchestrates an ephemeral Azure environment for local functional tests.
#
# Subcommands:
#   setup      Create an ephemeral resource group, deploy test fixtures, configure
#              the current rad environment with the Azure provider scope, and write
#              state to debug_files/logs/azure-local.env.
#   run        Source the state file and run the Test_Azure* subset of the
#              corerp-cloud functional tests against the locally-running stack.
#   teardown   Delete the resource group (no-wait), clear the env-update on the
#              current rad environment, and remove the state file.
#
# Auth model: this script assumes the caller is authenticated via `az login`.
# Radius components (RP/UCP) authenticate to Azure via the Azure CLI fallback in
# pkg/azure/armauth/auth.go. The Deployment Engine must also be running locally
# (not in a container) so it can use the same DefaultAzureCredential -> az CLI
# path. See debug_files/logs/de-external.marker for the external-DE indicator.
#
# Environment variables:
#   AZURE_LOCATION                 Azure region (default: westus3).
#   AZURE_SUBSCRIPTION_ID          Subscription to use (default: az account show).
#   AZURE_LOCAL_PREPROVISIONED_RG  If set, reuse an existing RG and skip fixture
#                                  deployment / teardown of that RG. The script
#                                  will still write the state file so `run` works.
#   RAD_ENV                        rad environment to configure (default: default).
#
# Note: AWS support is intentionally out of scope for this iteration.
# Note: Test_AzureMSSQL_* tests are not configured here; they auto-skip when the
#       AZURE_MSSQL_* env vars are absent.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STATE_DIR="${REPO_ROOT}/debug_files/logs"
STATE_FILE="${STATE_DIR}/azure-local.env"
BICEP_TEMPLATE="${REPO_ROOT}/test/createAzureTestResources.bicep"
TF_MODULE_SERVER_NS="radius-test-tf-module-server"
TF_MODULE_SERVER_PORT_FORWARD_PID_FILE="${STATE_DIR}/tf-module-server-pf.pid"
TF_MODULE_SERVER_PORT_FORWARD_LOG="${STATE_DIR}/tf-module-server-pf.log"

AZURE_LOCATION="${AZURE_LOCATION:-westus3}"
RAD_ENV="${RAD_ENV:-default}"

log()  { printf '\033[0;34mℹ\033[0m %s\n' "$*"; }
ok()   { printf '\033[0;32m✔\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m⚠\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[0;31m✖\033[0m %s\n' "$*" >&2; }

require_cmd() {
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || { err "required command not found: $c"; exit 1; }
  done
}

require_az_login() {
  if ! az account show >/dev/null 2>&1; then
    err "not logged in to Azure. Run 'az login' first."
    exit 1
  fi
}

resolve_subscription() {
  if [[ -n "${AZURE_SUBSCRIPTION_ID:-}" ]]; then
    echo "${AZURE_SUBSCRIPTION_ID}"
    return
  fi
  az account show --query id -o tsv
}

ensure_tf_module_server() {
  # Terraform-recipe tests fetch zipped modules from http://localhost:8999.
  # In CI an in-cluster nginx serves them; locally we deploy the same nginx
  # into the debug k3d cluster and port-forward it to localhost:8999.
  require_cmd kubectl make
  if curl -sf -o /dev/null -m 2 http://localhost:8999/azure-rg.zip; then
    log "tf-module-server already reachable at http://localhost:8999"
    return 0
  fi
  if ! kubectl get ns "${TF_MODULE_SERVER_NS}" >/dev/null 2>&1 \
      || ! kubectl -n "${TF_MODULE_SERVER_NS}" get deploy tf-module-server >/dev/null 2>&1; then
    log "Deploying tf-module-server into the debug cluster (publish-test-terraform-recipes)..."
    (cd "${REPO_ROOT}" && make publish-test-terraform-recipes >/dev/null) \
      || { err "make publish-test-terraform-recipes failed"; exit 1; }
  fi
  log "Waiting for tf-module-server rollout..."
  kubectl -n "${TF_MODULE_SERVER_NS}" rollout status deploy/tf-module-server --timeout=120s >/dev/null \
    || { err "tf-module-server rollout did not become ready"; exit 1; }
  # Stop any stale port-forward before starting a new one.
  if [[ -f "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}" ]]; then
    local old_pid
    old_pid="$(cat "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}" 2>/dev/null || true)"
    if [[ -n "${old_pid}" ]] && kill -0 "${old_pid}" 2>/dev/null; then
      kill "${old_pid}" 2>/dev/null || true
    fi
    rm -f "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}"
  fi
  log "Starting kubectl port-forward svc/tf-module-server 8999:80 -n ${TF_MODULE_SERVER_NS}"
  ( kubectl -n "${TF_MODULE_SERVER_NS}" port-forward svc/tf-module-server 8999:80 \
      >"${TF_MODULE_SERVER_PORT_FORWARD_LOG}" 2>&1 ) &
  echo $! > "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}"
  # Wait briefly for the port-forward to come up.
  local i
  for i in $(seq 1 20); do
    if curl -sf -o /dev/null -m 1 http://localhost:8999/azure-rg.zip; then
      ok "tf-module-server reachable at http://localhost:8999 (pid $(cat "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}"))"
      return 0
    fi
    sleep 0.5
  done
  err "tf-module-server port-forward did not become reachable; see ${TF_MODULE_SERVER_PORT_FORWARD_LOG}"
  exit 1
}

stop_tf_module_server_port_forward() {
  if [[ -f "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}" ]]; then
    local pid
    pid="$(cat "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}" 2>/dev/null || true)"
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      log "Stopping tf-module-server port-forward (pid ${pid})"
      kill "${pid}" 2>/dev/null || true
    fi
    rm -f "${TF_MODULE_SERVER_PORT_FORWARD_PID_FILE}"
  fi
}

cmd_setup() {
  require_cmd az jq rad
  require_az_login
  mkdir -p "${STATE_DIR}"

  if [[ -f "${STATE_FILE}" ]]; then
    err "state file already exists at ${STATE_FILE}. Run 'teardown' first or remove it manually."
    exit 1
  fi

  local sub
  sub="$(resolve_subscription)"
  local tenant
  tenant="$(az account show --query tenantId -o tsv)"
  log "Subscription: ${sub}"

  local rg
  if [[ -n "${AZURE_LOCAL_PREPROVISIONED_RG:-}" ]]; then
    rg="${AZURE_LOCAL_PREPROVISIONED_RG}"
    log "Reusing pre-provisioned resource group: ${rg}"
    if ! az group show --subscription "${sub}" --name "${rg}" >/dev/null 2>&1; then
      err "pre-provisioned resource group ${rg} not found in subscription ${sub}"
      exit 1
    fi
  else
    local user_slug epoch
    user_slug="$(echo "${USER:-local}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '-' | sed 's/-\{2,\}/-/g;s/^-//;s/-$//')"
    epoch="$(date +%s)"
    rg="radlocal-${user_slug}-${epoch}"
    log "Creating resource group: ${rg} in ${AZURE_LOCATION}"
    az group create \
      --subscription "${sub}" \
      --location "${AZURE_LOCATION}" \
      --name "${rg}" \
      --tags creationTime="${epoch}" creator="${USER:-unknown}" purpose=radius-local-test \
      -o none
    while [[ "$(az group exists --subscription "${sub}" --name "${rg}")" != "true" ]]; do
      sleep 2
    done
    ok "Resource group created: ${rg}"
  fi

  local cosmos_id=""
  if [[ -z "${AZURE_LOCAL_PREPROVISIONED_RG:-}" ]]; then
    # Cosmos DB account names are globally unique. Derive a stable-but-unique
    # name from the RG (which already includes user + epoch) and trim to the
    # 3-44 char limit. Cosmos requires lowercase alphanumerics + hyphens, and
    # disallows leading or trailing hyphens.
    local cosmos_name
    cosmos_name="$(echo "radlocal-${rg#radlocal-}" \
      | tr '[:upper:]' '[:lower:]' \
      | tr -c 'a-z0-9-' '-' \
      | cut -c1-44 \
      | sed -E 's/^-+//; s/-+$//')"
    log "Deploying test fixtures (Cosmos Mongo account ${cosmos_name}) — this typically takes 3-5 minutes..."
    local deploy_json
    deploy_json="$(az deployment group create \
      --subscription "${sub}" \
      --resource-group "${rg}" \
      --template-file "${BICEP_TEMPLATE}" \
      --parameters cosmosAccountName="${cosmos_name}" \
      -o json)"
    cosmos_id="$(echo "${deploy_json}" | jq -r '.properties.outputs.cosmosMongoAccountID.value')"
    ok "Cosmos Mongo account deployed: ${cosmos_id}"
  else
    # Reuse: look up by tag/kind in the pre-provisioned RG.
    cosmos_id="$(az resource list \
      --subscription "${sub}" \
      --resource-group "${rg}" \
      --resource-type Microsoft.DocumentDB/databaseAccounts \
      --query '[?kind==`MongoDB`] | [0].id' -o tsv || true)"
    if [[ -z "${cosmos_id}" || "${cosmos_id}" == "null" ]]; then
      warn "no Cosmos Mongo account found in ${rg}; Test_AzureConnections will fail."
    else
      ok "Found Cosmos Mongo account: ${cosmos_id}"
    fi
  fi

  log "Configuring rad environment '${RAD_ENV}' with Azure scope"
  rad env update "${RAD_ENV}" \
    --azure-subscription-id "${sub}" \
    --azure-resource-group "${rg}"

  ensure_tf_module_server

  cat > "${STATE_FILE}" <<EOF
# Generated by build/scripts/azure-local-testenv.sh on $(date -u +%FT%TZ)
# Source this file before running Azure-targeted local functional tests.
# Scoped to azure only so AWS-required tests still skip via CheckRequiredFeatures.
export RADIUS_TEST_USE_LOCAL_CLOUD_CREDS=azure
export AZURE_SUBSCRIPTION_ID=${sub}
export AZURE_TENANT_ID=${tenant}
export AZURE_LOCAL_TEST_RG=${rg}
export AZURE_COSMOS_MONGODB_ACCOUNT_ID=${cosmos_id}
# INTEGRATION_TEST_RESOURCE_GROUP_NAME is used by Test_MongoDB_Recipe_Parameters.
export INTEGRATION_TEST_RESOURCE_GROUP_NAME=${rg}
EOF
  ok "State written to ${STATE_FILE}"
}

recover_state() {
  # Rebuild the state file from an existing RG. Used when the state file was
  # removed (e.g. after a `debug-stop` cleanup) but the RG itself is still
  # alive — common when re-running individual failing tests.
  require_cmd az jq
  require_az_login
  local sub tenant rg cosmos_id
  sub="$(resolve_subscription)"
  tenant="$(az account show --query tenantId -o tsv)"
  if [[ -n "${AZURE_LOCAL_PREPROVISIONED_RG:-}" ]]; then
    rg="${AZURE_LOCAL_PREPROVISIONED_RG}"
  elif [[ -n "${AZURE_LOCAL_TEST_RG:-}" ]]; then
    rg="${AZURE_LOCAL_TEST_RG}"
  else
    # Auto-discover a single radlocal-<user>-* RG owned by the current user.
    local user_slug
    user_slug="$(echo "${USER:-local}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '-' | sed 's/-\{2,\}/-/g;s/^-//;s/-$//')"
    local matches
    matches="$(az group list --subscription "${sub}" --query "[?starts_with(name, 'radlocal-${user_slug}-')].name" -o tsv)"
    local count
    count="$(printf '%s\n' "${matches}" | grep -c . || true)"
    if [[ "${count}" -eq 0 ]]; then
      err "no state file at ${STATE_FILE} and no radlocal-* RG to recover from. Run 'setup' first."
      exit 1
    fi
    # Pick the newest RG (epoch suffix). RG names are radlocal-<user>-<epoch>.
    rg="$(printf '%s\n' ${matches} | sort -t- -k3 -n | tail -1)"
    if [[ "${count}" -gt 1 ]]; then
      warn "multiple radlocal-${user_slug}-* RGs found; using newest: ${rg}"
      warn "clean up the rest with: $0 teardown --all-orphans"
    fi
  fi
  if ! az group show --subscription "${sub}" --name "${rg}" >/dev/null 2>&1; then
    err "resource group ${rg} not found in subscription ${sub}"
    exit 1
  fi
  cosmos_id="$(az resource list \
    --subscription "${sub}" \
    --resource-group "${rg}" \
    --resource-type Microsoft.DocumentDB/databaseAccounts \
    --query '[?kind==`MongoDB`] | [0].id' -o tsv 2>/dev/null || true)"
  mkdir -p "${STATE_DIR}"
  cat > "${STATE_FILE}" <<EOF
# Recovered by build/scripts/azure-local-testenv.sh on $(date -u +%FT%TZ)
export RADIUS_TEST_USE_LOCAL_CLOUD_CREDS=azure
export AZURE_SUBSCRIPTION_ID=${sub}
export AZURE_TENANT_ID=${tenant}
export AZURE_LOCAL_TEST_RG=${rg}
export AZURE_COSMOS_MONGODB_ACCOUNT_ID=${cosmos_id}
export INTEGRATION_TEST_RESOURCE_GROUP_NAME=${rg}
EOF
  ok "State recovered from RG ${rg} -> ${STATE_FILE}"
  ensure_tf_module_server
}

cmd_run() {
  if [[ ! -f "${STATE_FILE}" ]]; then
    warn "no state file at ${STATE_FILE}; attempting to recover from an existing RG..."
    recover_state
  fi
  # shellcheck disable=SC1090
  source "${STATE_FILE}"
  ensure_tf_module_server
  # Re-apply Azure scope on the rad env idempotently. `make debug-start`
  # recreates the Postgres DB, which wipes the env's Azure provider config
  # set during `setup`. Without this, bicep templates that use
  # `resourceGroup().id` for `providers.azure.scope` (e.g.
  # corerp-resources-terraform-azurerg.bicep) get an empty subscription id
  # because the deployment-engine substitutes `resourceGroup().id` from the
  # active env's Azure scope.
  log "Ensuring rad env '${RAD_ENV}' has Azure scope (sub=${AZURE_SUBSCRIPTION_ID}, rg=${AZURE_LOCAL_TEST_RG})"
  rad env update "${RAD_ENV}" \
    --azure-subscription-id "${AZURE_SUBSCRIPTION_ID}" \
    --azure-resource-group "${AZURE_LOCAL_TEST_RG}" >/dev/null
  log "Running corerp/cloud functional tests against RG ${AZURE_LOCAL_TEST_RG}"
  log "AWS-required tests will skip automatically (RADIUS_TEST_USE_LOCAL_CLOUD_CREDS=azure)"
  cd "${REPO_ROOT}"
  # Run the full corerp/cloud suite. CheckRequiredFeatures will skip tests that
  # need AWS or the CSI driver; only the Azure-required tests will execute.
  #
  # Stream per-test output: `go test -v` buffers each package's output until
  # the package finishes, which hides progress for long Azure deployments.
  # Prefer gotestsum (installed via `make test-get-envtools` in CI) for
  # per-test live output; fall back to `go test -json` piped through a small
  # awk filter that prints each PASS/FAIL/SKIP line as it completes.
  if command -v gotestsum >/dev/null 2>&1; then
    CGO_ENABLED=1 gotestsum --format testname -- \
      ./test/functional-portable/corerp/cloud/... \
      -timeout "${TEST_TIMEOUT:-1h}" \
      -parallel 5 \
      ${GOTEST_OPTS:-} \
      "$@"
  else
    log "gotestsum not found; using 'go test -json' for streaming output. Install with: go install gotest.tools/gotestsum@latest"
    CGO_ENABLED=1 go test \
      ./test/functional-portable/corerp/cloud/... \
      -json \
      -timeout "${TEST_TIMEOUT:-1h}" \
      -parallel 5 \
      ${GOTEST_OPTS:-} \
      "$@" | \
      awk -F'"' '
        /"Action":"run"/      { for (i=1;i<=NF;i++) if ($i=="Test") { print "RUN  " $(i+2); break } }
        /"Action":"pass"/     { for (i=1;i<=NF;i++) if ($i=="Test") { print "PASS " $(i+2); break } }
        /"Action":"fail"/     { for (i=1;i<=NF;i++) if ($i=="Test") { print "FAIL " $(i+2); break } }
        /"Action":"skip"/     { for (i=1;i<=NF;i++) if ($i=="Test") { print "SKIP " $(i+2); break } }
        /"Action":"output"/   { for (i=1;i<=NF;i++) if ($i=="Output") { gsub(/\\n/,"",$(i+2)); if ($(i+2) != "") print "    " $(i+2); break } }
      '
  fi
}

cmd_teardown() {
  require_cmd az
  # Optional: --all-orphans deletes every radlocal-<user>-* RG in the current
  # subscription, regardless of state file. Useful after a debug-stop wiped
  # state without tearing down RGs.
  if [[ "${1:-}" == "--all-orphans" ]]; then
    require_az_login
    local sub user_slug matches
    sub="$(resolve_subscription)"
    user_slug="$(echo "${USER:-local}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '-' | sed 's/-\{2,\}/-/g;s/^-//;s/-$//')"
    matches="$(az group list --subscription "${sub}" --query "[?starts_with(name, 'radlocal-${user_slug}-')].name" -o tsv)"
    if [[ -z "${matches}" ]]; then
      ok "no radlocal-${user_slug}-* RGs to delete."
    else
      log "Deleting orphan RGs (no-wait):"
      printf '  %s\n' ${matches}
      for orphan in ${matches}; do
        az group delete --subscription "${sub}" --name "${orphan}" --yes --no-wait \
          || warn "failed to start delete for ${orphan}"
      done
    fi
    stop_tf_module_server_port_forward
    rm -f "${STATE_FILE}"
    return 0
  fi
  if [[ ! -f "${STATE_FILE}" ]]; then
    warn "no state file at ${STATE_FILE}; nothing to tear down. Use '$0 teardown --all-orphans' to GC stale radlocal-* RGs."
    return 0
  fi
  # shellcheck disable=SC1090
  source "${STATE_FILE}"

  local sub="${AZURE_SUBSCRIPTION_ID}"
  local rg="${AZURE_LOCAL_TEST_RG}"

  if [[ -z "${AZURE_LOCAL_PREPROVISIONED_RG:-}" && "${rg}" == radlocal-* ]]; then
    log "Deleting resource group ${rg} (no-wait)"
    az group delete \
      --subscription "${sub}" \
      --name "${rg}" \
      --yes --no-wait || warn "az group delete returned non-zero"
  else
    warn "RG ${rg} was pre-provisioned (or not radlocal-* prefix); leaving it intact."
  fi

  log "Clearing Azure scope on rad env '${RAD_ENV}'"
  rad env update "${RAD_ENV}" --clear-azure 2>/dev/null || \
    warn "rad env update --clear-azure failed or unsupported; clear manually if needed."

  stop_tf_module_server_port_forward
  rm -f "${STATE_FILE}"
  ok "Teardown complete."
}

cmd_all() {
  cmd_setup
  local rc=0
  if [[ "${AZURE_LOCAL_KEEP_ON_FAILURE:-0}" == "1" || "${AZURE_LOCAL_KEEP_ON_FAILURE:-}" =~ ^[Tt]rue$ ]]; then
    log "AZURE_LOCAL_KEEP_ON_FAILURE is set; teardown will be SKIPPED if tests fail (post-mortem mode)."
    cmd_run "$@" || rc=$?
    if [[ "${rc}" -ne 0 ]]; then
      warn "Tests exited with rc=${rc}; preserving RG ${AZURE_LOCAL_TEST_RG:-?} and state file ${STATE_FILE} for inspection."
      warn "Run 'make test-functional-azure-local-teardown' when finished."
      return "${rc}"
    fi
    cmd_teardown
    return 0
  fi
  # Default: ensure teardown runs even if tests fail.
  trap cmd_teardown EXIT
  cmd_run "$@" || rc=$?
  trap - EXIT
  cmd_teardown
  return "${rc}"
}

usage() {
  cat >&2 <<EOF
Usage: $0 <setup|run|teardown|all> [-- extra go test args]

  setup      Create RG, deploy fixtures, write state file.
  run        Source state file (auto-recover from existing RG if missing) and
             run corerp/cloud functional tests. Extra args after the command
             are passed through to 'go test' (e.g. -run '^Test_X$' -v).
  teardown   Delete RG and remove state file.
  all        setup -> run -> teardown (teardown runs even on failure).

Examples:
  # Re-run only specific failing tests against the existing RG:
  $0 run -run '^(Test_TerraformRecipe_AzureResourceGroup|Test_Extender_RecipeAWS_LogGroup)\$' -v

Environment variables:
  AZURE_LOCAL_KEEP_ON_FAILURE=1   When set with 'all', skip teardown on test
                                  failure so the RG can be inspected. The
                                  state file is also preserved; clean up later
                                  with 'teardown'.
  AZURE_LOCAL_PREPROVISIONED_RG   Reuse an existing RG (skip Cosmos deploy and
                                  skip RG deletion).
  AZURE_LOCAL_TEST_RG             Force recovery from a specific RG when the
                                  state file is missing.
EOF
  exit 2
}

cmd="${1:-}"
shift || true
case "${cmd}" in
  setup)    cmd_setup "$@" ;;
  run)      cmd_run "$@" ;;
  teardown) cmd_teardown "$@" ;;
  all)      cmd_all "$@" ;;
  *)        usage ;;
esac
