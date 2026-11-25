# Radius CLI Component Threat Model: v0.62.0

- **Status**: Draft; security review pending.
- **Baseline**: Radius v0.62.0, commit `37a8cbd33e990886460f4fd1987d6a4fefd901c5`.
- **Related models**: [UCP](2026-11-ucp-component-threat-model-v0.62.0.md) and [recipe execution](2026-11-recipe-execution-threat-model-v0.62.0.md).

## Overview

`rad` runs on a developer or operator workstation and connects local inputs to privileged cluster and cloud operations. Workspace selection, local tools, kubeconfig files, registry credentials and output artifacts are security boundaries even though the CLI is not a network service.

Deployment, installation/upgrade, graph generation and state recovery use different parts of that authority. Arbitrary user applications and DE internals are outside this model. The scenarios below describe risks, not confirmed exploits.

## Terms and Definitions

| Term          | Definition                                                                                         |
|---------------|----------------------------------------------------------------------------------------------------|
| Workspace     | Local Radius configuration selecting a connection and resource scope.                              |
| Kubeconfig    | Kubernetes client configuration; it can contain credentials and executable authentication plugins. |
| Modeled graph | Graph computed from a compiled local application definition rather than only live resources.       |
| State archive | OCI artifact containing a directory snapshot used by state commands.                               |
| Restore       | Applying archived databases/state into a selected installation, not merely reading a file.         |

## System Description

The [CLI architecture][architecture] identifies workspace/config loading, SDK clients, local Bicep use, and direct Kubernetes/Helm operations. Deployment requests go through UCP. Install/upgrade commands operate directly with the user's cluster authority and chart inputs.

The [graph command][graph] can read deployed state, compile a local definition, write `app-graph.json`, or save a modeled graph to an OCI archive in GitHub Actions mode. The [state archive][archive] is also used by shutdown/startup commands to persist and restore selected durable state.

Assets include user credentials, kubeconfig contents, local source and parameters, binaries, selected target/scope, output files, registry credentials, database dumps and Terraform state. Actors include the workstation user, local file writers, repository contributors, tool/artifact publishers and registry administrators.

### Architecture

The local process is trusted with the invoking user's permissions. [Bicep execution][build] resolves a binary path and launches it with an argument list. A configured executable override or kubeconfig plugin is executable code, not just data to parse.

Installation gives selected charts, images and hooks cluster authority. The released chart includes readiness and optional pre-upgrade hooks and the enabled-by-default encryption-key rotation job. Their configuration belongs to the installation trust decision, not a separate unprivileged user path.

`rad shutdown` [backs up][shutdown] PostgreSQL control-plane databases and Terraform state Secrets to an OCI archive. It refuses to replace the archive when the inspected backup has no resource groups. `rad startup` [restores][startup] databases and Terraform state while scaling database-backed control-plane deployments down, then attempts to scale them back up.

These commands back up PostgreSQL data and Terraform state, not the whole cluster. The default chart uses API-server storage. Recovery of its records, application Secrets, cloud-backend state, encryption keys and cloud credentials needs a separate inventory.

### Implementation Details

**Is there any use of cryptography?**

Yes. Kubernetes/cloud client transports, HTTPS tool downloads, OCI content digests and credential stores are dependencies. The [published release lock][lock] records CLI OCI artifacts tied to the release source. Release checksums/digests are useful only when the receiving installation path verifies and trusts them.

The [Bicep download helper][download] pins `v0.46.1` and fetches from its HTTPS GitHub release URL. It copies the response into an executable file without an independent signature or checksum comparison. Verification of every other chart, image and tool download path remains outside this source review.

**Does the component store secrets?**

Yes. Local configuration/credential mechanisms, kubeconfigs, parameter files, dump files, Terraform state and archives can contain sensitive data. The OCI archive packs regular files into a compressed layer; compression is not encryption. Registry access control is therefore a confidentiality requirement.

**Does the component process untrusted data or does the component parse any custom formats?**

Yes. Commands and paths, YAML/JSON configuration, Bicep, ARM templates, custom Radius IDs, graph properties and OCI manifests/tar entries are processed. The archive implementation rejects non-regular extracted entries and checks that paths are local to the destination root. Those checks do not approve the contents of a valid database dump.

### Clients

People and automation invoke `rad`. Dependencies include local tools, Kubernetes/Helm, UCP, cloud authentication libraries, tool distribution endpoints, and OCI registries. Standard output/error and CI artifact consumers are data recipients too.

## Trust Boundaries

| Boundary                             | Security responsibility                                                  |
|--------------------------------------|--------------------------------------------------------------------------|
| Repository/files to local process    | Decide whether definitions, parameters and referenced tools are trusted. |
| Workspace/kubeconfig to target       | Confirm cluster, scope and identity before a mutating operation.         |
| Downloaded executable to workstation | Approve the executable source and protect local binary paths.            |
| Installer to cluster                 | Review chart/image/hook content and installation privileges.             |
| Live state to local/OCI archive      | Protect confidentiality, integrity and retention.                        |
| Archive to restore target            | Approve the snapshot and target before replacing live state.             |
| Resource metadata to graph consumers | Preserve appropriate redaction and restrict output readers.              |

## Assumptions

The invoking user is authorized to operate the selected target. The OS, configured credential helpers, cluster APIs and approved artifact services are trust dependencies.

Operators must check the selected workspace/target and archive access policy. Repository contributors and archive writers are separate trust roles; a content digest does not identify a trusted publisher. A malicious local administrator is outside the prevention boundary, but a less-privileged writer of an input repository remains a relevant actor.

## Data Flows

### Diagram

```mermaid
flowchart LR
    User["User / automation"]
    Files["Workspace / source / kubeconfig"]
    Tools["Local Bicep and other tools"]
    subgraph Local["Trusted workstation process"]
        Rad["rad"]
        Temp[("Local files / temporary state")]
    end
    U["UCP"]
    K["Kubernetes / Helm"]
    Registry["OCI registry"]
    Consumer["Output / CI artifact reader"]
    User -->|"F1: command / flags"| Rad
    Files -->|"F2: config / code / target"| Rad
    Rad -->|"F3: execute / compile"| Tools
    Rad -->|"F4: deployment / graph API"| U
    Rad -->|"F5: install / backup / restore"| K
    K -->|"F6: dump / state export"| Temp
    Temp -->|"F7: archive push through rad"| Registry
    Registry -->|"F8: archive pull / extract through rad"| Temp
    Temp -->|"F9: restore through rad"| K
    Rad -->|"F10: graph / logs / results"| Consumer
```

F1-F3 determine both executable input and target. F4-F5 use the user's selected connection. F6-F9 move sensitive state across local and registry trust boundaries. F10 can disclose data independently of whether a deployment succeeds.

## Threats

### CLI-01: Configuration selects the wrong target or identity

**Description:** A local file writer or automation configuration changes F2 so a mutating command reaches an unintended cluster, scope or endpoint.

**Impact:** Resource changes, credential disclosure to a wrong endpoint, or backup/restore of the wrong installation.

**Mitigation:** Protect workspace/kubeconfig files, verify target identity in automation, and scope credentials to intended operations.

**Status:** Workspace and connection selection are implemented. They cannot establish operator intent from a valid but unintended configuration.

### CLI-02: A local or downloaded tool executes hostile code

**Description:** An actor changes a configured binary, executable override, authentication plugin or downloaded tool used in F3.

**Impact:** Code runs with workstation/runner permissions and can access its files or credentials.

**Mitigation:** Use approved tool sources, protect binary/config directories, and verify authenticity through the applicable distribution mechanism.

**Status:** The Bicep helper pins a version and uses HTTPS, but its download function does not independently check a signature/checksum. Arguments are not evaluated as one shell command in the Bicep runner; this does not make the executable safe.

### CLI-03: Installation content without review obtains cluster authority

**Description:** An operator installs a changed chart, image, hook or values set through F5.

**Impact:** Elevated cluster access, exposed services, weakened policy, or replaced control-plane code.

**Mitigation:** Use an approved chart version and image source. Review overrides such as `global.imageRegistry` and `global.imageTag`, rendered service-account grants, and hook Jobs before installation. Give the installer only the cluster access needed for that installation.

**Status:** The chart exposes those image overrides and defines the hook Jobs and service-account grants. The Bicep downloader lacks its own signature/checksum check. This review did not trace verification in every chart/image download path. The UCP and recipe models describe the separate limits of RBAC mode, NetworkPolicy and BuildKit settings.

### CLI-04: Graphs, parameters or diagnostics disclose secrets

**Description:** A reader obtains F10 output, generated graph files, CI logs or temporary inputs that include author-provided sensitive values.

**Impact:** Credential, source-path or infrastructure metadata disclosure.

**Mitigation:** Keep credentials out of graph properties and ordinary parameters. Use the resource type's documented secret field or reference; for core secret stores, see [APP-05](2026-11-applications-rp-component-threat-model-v0.62.0.md#app-05-stored-or-rendered-values-expose-secrets). Restrict file, log and CI artifact readers, and set a retention policy.

**Status:** The [graph sanitizer][sanitizer] removes container environment maps for `Radius.Compute/containers`. Other properties, compiler messages, legacy API results and arbitrary parameter values were not all checked for secrets.

### CLI-05: Archive readers obtain sensitive infrastructure state

**Description:** A registry/local-file reader accesses database dumps or Terraform state at F6-F8.

**Impact:** Disclosure of deployment data and any secrets retained in those formats.

**Mitigation:** Use private, access-controlled repositories, protect credential helpers and local working files, and apply suitable storage encryption/retention.

**Status:** OCI authentication uses Docker credential-store integration and HTTPS by default; a plain-HTTP option exists for local registry use. The archive layer is compressed, not application-encrypted.

### CLI-06: An accepted archive contains malicious or stale state

**Description:** A party able to publish the selected archive supplies validly packaged but malicious/outdated data, or automation selects the wrong snapshot for F9.

**Impact:** Resource-state replacement, loss of deployment tracking, unintended later operations, or restore outage.

**Mitigation:** Restrict archive writers, bind approved snapshots to targets, retain known recovery points and validate restore procedures.

**Status:** Local-path/type checks reject unsafe paths and non-regular extracted entries. OCI digests identify content, not the publisher or the operator's approved recovery point. Publisher-signature and snapshot-freshness checks were not found in the cited archive implementation.

### CLI-07: Restore completeness or key mismatch breaks recovery

**Description:** F9 restores records without all required state, credentials or encryption key versions, or a restore step fails after state changes.

**Impact:** Pending data cannot be decrypted, reconciliation fails, recovery is partial, or infrastructure is no longer tracked correctly.

**Mitigation:** Maintain a recovery inventory for all providers/backends and keys, validate compatibility and rehearse recovery on an isolated target.

**Status:** Startup coordinates scaling and attempts to restore replica counts after failure. Shutdown rejects an empty resource-group backup. The commands do not make a whole-cluster transaction or inventory external secrets/state for the operator.

### CLI-08: Shared archive operations lose history or exhaust resources

**Description:** Separate writers update the same archive, or a large valid archive consumes workstation/runner capacity.

**Impact:** Lost snapshot updates, storage exhaustion, failed restore or reduced availability.

**Mitigation:** Serialize publishers across processes, bound retention/size and monitor recovery operations.

**Status:** The archive locks each repository/name within one process and checks the tag's digest before pushing. It rejects a tag change observed since opening the archive. That check and the later push are separate operations, not a distributed lock. The extraction loop sets no overall byte budget; limits in registry client dependencies were not assessed.

## Open Questions

- Which installer/tool paths verify signed provenance versus only using a version or checksum?
- Which records, Secrets, keys and external state backends must be restored together for each supported installation?
- What cross-runner serialization and retention policy should archive publishers use?

## Action Items

| Owner       | Follow-up and completion criteria                                                                                                                                                             | Tracking  |
|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------|
| Operators   | Record the cluster/scope used by each mutating job and the writers of its config and executables. Verify that credentials cannot change resources outside that scope.                         | Untracked |
| Operators   | Inventory the records, Secrets, key versions and backend state needed for recovery. Complete an isolated restore and verify a resource read and subsequent reconciliation.                    | Untracked |
| Maintainers | Review each tool/chart verification path and define archive publisher, freshness and size checks. Complete when rejected artifacts have tests and accepted recovery points remain restorable. | Untracked |

## Review Notes

Security review is pending. No install, upgrade, backup, restore or executable download was run for runtime testing.

The cited source defines the archive guards, command behavior and Bicep download path. [Published release metadata][manifest] records release asset/image/chart checks. It does not verify each tool downloaded later or an operator's live deployment.

## References

- [CLI architecture][architecture], [Bicep runner][build], and [tool download helper][download].
- [Graph command][graph], [archive implementation][archive], [shutdown][shutdown], and [startup][startup].
- [State backup client][backupclient], [restore client][restoreclient], and [released hooks/chart][chart].
- [Core release lock][lock] and [release manifest][manifest].

[architecture]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/docs/architecture/rad-cli.md
[build]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/bicep/build.go
[download]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/bicep/tools/download_tools.go
[graph]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/cmd/app/graph/graph.go
[sanitizer]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/graph/sanitization/properties.go
[archive]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/statearchive/oci/oci.go
[shutdown]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/cmd/shutdown/shutdown.go
[startup]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/cmd/startup/startup.go
[backupclient]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/cmd/shutdown/stateclient.go
[restoreclient]: https://github.com/radius-project/radius/blob/37a8cbd33e990886460f4fd1987d6a4fefd901c5/pkg/cli/cmd/startup/stateclient.go
[chart]: https://github.com/radius-project/radius/tree/37a8cbd33e990886460f4fd1987d6a4fefd901c5/deploy/Chart/templates
[lock]: https://github.com/radius-project/radius/releases/download/v0.62.0/core-release-lock.json
[manifest]: https://github.com/radius-project/radius/releases/download/v0.62.0/release-manifest.json
