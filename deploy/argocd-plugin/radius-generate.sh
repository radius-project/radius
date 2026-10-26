#!/bin/sh
# Argo CD runs this in the Application's source directory. stdout must be manifests only.
set -eu
: "${PARAM_BICEPFILE:?bicepFile is required}" "${PARAM_RESOURCEGROUP:?resourceGroup is required}"
out="$(mktemp -d)/out.yaml"
if [ -n "${PARAM_PARAMSFILE:-}" ]; then
  rad bicep generate-kubernetes-manifest "$PARAM_BICEPFILE" -g "$PARAM_RESOURCEGROUP" -p "@$PARAM_PARAMSFILE" -d "$out" >&2
else
  rad bicep generate-kubernetes-manifest "$PARAM_BICEPFILE" -g "$PARAM_RESOURCEGROUP" -d "$out" >&2
fi
# Stamp the Git revision; drop empty parameters (workaround for radius-project/radius#13436).
awk -v rev="${ARGOCD_APP_REVISION:-unknown}" '
  $0 == "  parameters: {}" { next }
  { print }
  $0 == "metadata:" { print "  annotations:"; print "    radapp.io/source-revision: \"" rev "\"" }
' "$out"
