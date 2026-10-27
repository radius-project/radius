---
name: user-rbac-poc
description: Build and exercise the opt-in user RBAC prototype on its dedicated kind cluster.
argument-hint: up | test | rad <identity> <arguments> | logs | down
---

# User RBAC POC

Follow [Testing the user RBAC prototype](../../../docs/contributing/contributing-code/contributing-code-tests/user-rbac-poc.md), including its security limits and expected HTTP results.

Requires Go, GNU Make, Git, kind, kubectl, Helm, jq, curl, Bicep, and a running Docker or Podman runtime. Run `bash hack/user-rbac/poc.sh --help` for parameters. `RBAC_POC_CLUSTER` must have the dedicated `radius-user-rbac-poc` prefix; named clusters keep credentials and evidence under ignored `dist/$RBAC_POC_CLUSTER/`, separately from the default `dist/user-rbac-poc/`.

```sh
bash hack/user-rbac/poc.sh up
bash hack/user-rbac/poc.sh test
bash hack/user-rbac/poc.sh rad poc-admin auth role list --scope /
bash hack/user-rbac/poc.sh logs
bash hack/user-rbac/poc.sh down
```

Use Bicep templates for manual and CLI deployment scenarios. The backing walkthrough explains initial administrator bootstrap, repeat runs, and isolated configuration; the `rad` wrapper selects a prepared test identity without rerunning setup.

Never target shared clusters, change the user's kube context, publish credentials, or claim that skipped opt-in scenarios passed. Diagnose setup failures using the backing doc; do not alter machine-wide runtime settings.
