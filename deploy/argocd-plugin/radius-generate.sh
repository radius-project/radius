#!/bin/sh
# Argo CD runs this in the Application's source path. stdout must be manifests only.
# Each file in bicepFiles becomes one DeploymentTemplate. Its position in the list is its
# Argo CD sync wave, so Argo CD applies a file only after the previous ones are Healthy.
# A file's parameters come from <name>.parameters.json next to it, if present.
set -eu
: "${PARAM_BICEPFILES:?bicepFiles is required}" "${PARAM_RESOURCEGROUP:?resourceGroup is required}"
tmp="$(mktemp -d)"
wave=0
for f in $PARAM_BICEPFILES; do
  out="$tmp/$wave.yaml"
  p="${f%.bicep}.parameters.json"
  if [ -f "$p" ]; then
    rad bicep generate-kubernetes-manifest "$f" -g "$PARAM_RESOURCEGROUP" -p "@$p" -d "$out" >&2
  else
    rad bicep generate-kubernetes-manifest "$f" -g "$PARAM_RESOURCEGROUP" -d "$out" >&2
  fi
  [ "$wave" -gt 0 ] && echo "---"
  # Stamp the Git revision and sync wave; drop empty parameters (radius-project/radius#13436).
  awk -v rev="${ARGOCD_APP_REVISION:-unknown}" -v wave="$wave" '
    $0 == "  parameters: {}" { next }
    { print }
    $0 == "metadata:" {
      print "  annotations:"
      print "    radapp.io/source-revision: \"" rev "\""
      print "    argocd.argoproj.io/sync-wave: \"" wave "\""
    }
  ' "$out"
  wave=$((wave + 1))
done
