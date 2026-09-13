---
name: Publish pinned command-line tool updates
pipelineid: update-tools

scms:
  tools:
    kind: github
    spec:
      owner: radius-project
      repository: radius
      branch: main
      workingbranchprefix: automation
      workingbranchseparator: /
      token: '{{ requiredEnv "UPDATECLI_GITHUB_TOKEN" }}'
      commitusingapi: true
      commitmessage:
        type: chore
        scope: tools
        footers: 'Signed-off-by: {{ requiredEnv "UPDATECLI_COMMITTER" }}'

targets:
  refresh:
    name: Refresh pinned command-line tools
    kind: shell
    scmid: tools
    disablesourceinput: true
    spec:
      shell: /bin/sh
      # One target commits the entire successful update, not individual YAML fields.
      command: |
        set -eu
        action=apply
        if [ "$DRY_RUN" = "true" ]; then
          action=diff
        fi
        updatecli --disable-version-check --unique-tmp-dir pipeline "$action" \
          --config .updatecli/update-tools.yaml.tpl --values build/tools.yaml \
          --disable-changelog --disable-udash-report
      environments:
        - name: PATH
        - name: HOME
        - name: UPDATECLI_GITHUB_TOKEN
      changedif:
        kind: file/checksum
        spec:
          files:
            - build/tools.yaml
            - build/tools.generated.mk
{{- range .tools }}
{{- range .versionFiles }}
            - {{ .path | quote }}
{{- end }}
{{- end }}

actions:
  pull-request:
    kind: github/pullrequest
    scmid: tools
    title: "chore(tools): bump versions"
    spec:
      draft: true
      merge:
        strategy: manual
      description: |
        Refreshes the versions, platform SHA-256 checksums, generated Make metadata,
        and version consumers declared in `build/tools.yaml` using Updatecli.

        This PoC has no release-age filtering. Tools with `update: false` remain pinned.
        GitHub-hosted binaries use release-asset digests; vendor checksum files are
        used for Helm, kubectl, and Terraform.

        Upstream releases:
{{- range .tools }}
        - [{{ .name }}](https://github.com/{{ .source.repository }}/releases)
{{- end }}
