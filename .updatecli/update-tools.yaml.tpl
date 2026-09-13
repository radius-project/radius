{{- $manifest := default "build/tools.yaml" .manifestPath -}}
{{- $makefile := default "build/tools.generated.mk" .makefilePath -}}
{{- $githubAPI := default "https://api.github.com" .githubAPI | trimSuffix "/" -}}
{{- $token := coalesce (env "UPDATECLI_GITHUB_TOKEN") (env "GITHUB_TOKEN") (env "GH_TOKEN") -}}
{{- $refresh := true -}}
{{- if hasKey . "refresh" -}}{{- $refresh = .refresh -}}{{- end -}}
{{- if or (ne (int .schemaVersion) 1) (not .tools) (not .platforms) -}}
{{- fail "tools.yaml requires schemaVersion 1 and non-empty tools and platforms" -}}
{{- end -}}
{{- $allSources := list -}}
---
name: Refresh pinned command-line tools
pipelineid: radius-tools

sources:
{{- range $index, $tool := .tools }}
{{- $enabled := true }}
{{- if hasKey $tool "update" }}{{ $enabled = $tool.update }}{{ end }}
{{- $latestID := printf "%s-latest" $tool.name }}
{{- $pinID := printf "%s-pin" $tool.name }}
{{- $versionID := printf "%s-version" $tool.name }}
{{- $version := printf "{{ source %q }}" $versionID }}
{{- $versionNoV := printf "{{ source %q }}" (printf "%s-version-no-v" $tool.name) }}
{{- $versionRegexp := printf "{{ source %q }}" (printf "%s-version-regexp" $tool.name) }}
{{- $versionNoVRegexp := printf "{{ source %q }}" (printf "%s-version-no-v-regexp" $tool.name) }}
{{- $tagPrefix := default "" $tool.source.tagPrefix }}
{{- $tag := printf "%s%s" $tagPrefix $version }}
{{- $repository := default "" $tool.source.repository }}
{{- $allSources = append $allSources $versionID }}
  {{ $pinID }}:
    name: Read the current {{ $tool.name }} pin
    kind: yaml
    spec:
      file: {{ $manifest | quote }}
      key: {{ printf "$.tools[%d].version" $index | quote }}
{{- if $refresh }}
{{- $allSources = append $allSources $latestID }}
  {{ $latestID }}:
    name: Discover the latest {{ $tool.name }} release
    kind: http
    spec:
      url: {{ $tool.source.latestURL | quote }}
{{- if and $token (hasPrefix "https://api.github.com/" $tool.source.latestURL) }}
      request:
        headers:
          Authorization: {{ printf "Bearer %s" $token | quote }}
          Accept: application/vnd.github+json
{{- end }}
    transformers:
{{- if eq $tool.source.type "github-release" }}
      - jsonmatch:
          key: tag_name
      - trimprefix: {{ $tagPrefix | quote }}
{{- else if eq $tool.source.type "hashicorp-checkpoint" }}
      - jsonmatch:
          key: current_version
{{- else if ne $tool.source.type "stable-text" }}
{{- fail (printf "unsupported release source %q for %s" $tool.source.type $tool.name) }}
{{- end }}
      - findsubmatch:
          pattern: '^\s*(\S+)\s*$'
          captureindex: 1
      - trimprefix: " "
{{- end }}
{{- range $variant := list "version" "version-no-v" "version-regexp" "version-no-v-regexp" }}
  {{ $tool.name }}-{{ $variant }}:
    name: Resolve {{ $tool.name }} {{ $variant }}
    kind: yaml
    dependson:
      - {{ printf "target#%s-select-version" $tool.name | quote }}
    spec:
      file: {{ $manifest | quote }}
      key: {{ printf "$.tools[%d].version" $index | quote }}
    transformers:
      - replacer:
          from: {{ printf "{{ source %q }}" $pinID | quote }}
          to: {{ printf "{{ pipeline %q }}" (printf "Targets.%s-select-version.Result.NewInformation" $tool.name) | quote }}
{{- if contains "no-v" $variant }}
      - trimprefix: v
{{- end }}
{{- if contains "regexp" $variant }}
      - replacers:
          - from: "."
            to: '\.'
          - from: "+"
            to: '\+'
{{- end }}
{{- end }}
{{- if ne $tool.checksumSource.type "none" }}
{{- range $platform := $.platforms }}
{{- $entry := index $tool.platforms $platform }}
{{- if not $entry.asset }}{{ fail (printf "%s has no asset for %s" $tool.name $platform) }}{{ end }}
{{- $checksumID := printf "%s-%s-checksum" $tool.name $platform }}
{{- $allSources = append $allSources $checksumID }}
{{- $parts := splitList "_" $platform }}
{{- $os := default (index $parts 0) $entry.os }}
{{- $arch := default (index $parts 1) $entry.arch }}
{{- $asset := $entry.asset | replace "{version_no_v}" $versionNoV | replace "{version}" $version | replace "{tag}" $tag | replace "{repository}" $repository | replace "{os}" $os | replace "{arch}" $arch }}
  {{ $checksumID }}:
    name: Resolve {{ $tool.name }} {{ $platform }} SHA-256
{{- if not $refresh }}
    kind: yaml
    spec:
      file: {{ $manifest | quote }}
      key: {{ printf "$.tools[%d].platforms.%s.checksum" $index $platform | quote }}
    transformers:
      - findsubmatch:
          pattern: '^[a-f0-9]{64}$'
{{- else if eq $tool.checksumSource.type "url-file" }}
{{- $url := $tool.checksumSource.urlTemplate | replace "{asset}" $asset | replace "{version_no_v}" $versionNoV | replace "{version}" $version | replace "{tag}" $tag | replace "{repository}" $repository | replace "{os}" $os | replace "{arch}" $arch }}
    kind: http
    spec:
      url: {{ $url | quote }}
    transformers:
      - findsubmatch:
{{- if eq $tool.checksumSource.format "first" }}
          pattern: '^\s*([a-f0-9]{64})(?:\s|$)'
{{- else if eq $tool.checksumSource.format "standard" }}
{{- $assetRegexp := $entry.asset | regexQuoteMeta | replace "\\{version_no_v\\}" $versionNoVRegexp | replace "\\{version\\}" $versionRegexp | replace "\\{os\\}" ($os | regexQuoteMeta) | replace "\\{arch\\}" ($arch | regexQuoteMeta) }}
          pattern: {{ printf "(?m)^([a-f0-9]{64})[ \\t]+\\*?%s\\r?$" $assetRegexp | quote }}
{{- else }}
{{- fail (printf "unsupported checksum format %q for %s" $tool.checksumSource.format $tool.name) }}
{{- end }}
          captureindex: 1
{{- else if eq $tool.checksumSource.type "github-release-asset" }}
{{- if not $repository }}{{ fail (printf "%s needs a GitHub repository for asset digests" $tool.name) }}{{ end }}
    kind: http
    spec:
      url: {{ printf "%s/repos/%s/releases/tags/%s" $githubAPI $repository $tag | quote }}
{{- if and $token (eq $githubAPI "https://api.github.com") }}
      request:
        headers:
          Authorization: {{ printf "Bearer %s" $token | quote }}
          Accept: application/vnd.github+json
{{- end }}
    transformers:
      - jsonmatch:
          key: {{ printf "assets.all().filter(equal(name,%s)).digest" $asset | quote }}
      - findsubmatch:
          pattern: '^sha256:[a-f0-9]{64}$'
{{- else }}
{{- fail (printf "unsupported checksum source %q for %s" $tool.checksumSource.type $tool.name) }}
{{- end }}
      # A missing regex match is empty; this next transformer rejects empty input.
      - trimprefix: "sha256:"
{{- end }}
{{- end }}
{{- range $consumerIndex, $consumer := $tool.versionFiles }}
{{- $consumerID := printf "%s-consumer-%d" $tool.name $consumerIndex }}
{{- $allSources = append $allSources $consumerID }}
  {{ $consumerID }}:
    name: Read {{ $consumer.path }} before changing any files
{{- if eq $consumer.format "yaml" }}
    kind: yaml
    spec:
      file: {{ $consumer.path | quote }}
      key: {{ $consumer.key | quote }}
{{- else if eq $consumer.format "plain" }}
    kind: file
    spec:
      file: {{ $consumer.path | quote }}
      line: 1
{{- else if eq $consumer.format "replace" }}
    kind: file
    spec:
      file: {{ $consumer.path | quote }}
      matchpattern: {{ printf "(?m)^%s[^\\r\\n]*?%s" ($consumer.prefix | regexQuoteMeta) ($consumer.suffix | regexQuoteMeta) | quote }}
    transformers:
      - findsubmatch:
          pattern: {{ printf "^%s(v?[0-9]+(?:\\.[0-9]+){1,2}(?:-[0-9A-Za-z.-]+)?(?:\\+[0-9A-Za-z.-]+)?)%s$" ($consumer.prefix | regexQuoteMeta) ($consumer.suffix | regexQuoteMeta) | quote }}
          captureindex: 1
      - replacers:
          - from: "."
            to: '\.'
          - from: "+"
            to: '\+'
{{- else }}
{{- fail (printf "unsupported version file format %q" $consumer.format) }}
{{- end }}
{{- end }}
{{- end }}

conditions:
  all-metadata-resolved:
    name: Resolve every version, platform checksum, and consumer before writing
    kind: yaml
    disablesourceinput: true
    dependson:
{{- range $sourceID := $allSources }}
      - {{ printf "source#%s" $sourceID | quote }}
{{- end }}
    spec:
      file: {{ $manifest | quote }}
      key: $.schemaVersion
      value: "1"
{{- range $tool := .tools }}
{{- range $consumerIndex, $consumer := $tool.versionFiles }}
  {{ $tool.name }}-consumer-{{ $consumerIndex }}:
    name: Confirm {{ $consumer.path }} still matches its version snapshot
    disablesourceinput: true
{{- if eq $consumer.format "yaml" }}
    kind: yaml
    spec:
      file: {{ $consumer.path | quote }}
      key: {{ $consumer.key | quote }}
      value: {{ printf "{{ source %q }}" (printf "%s-consumer-%d" $tool.name $consumerIndex) | quote }}
{{- else if eq $consumer.format "plain" }}
    kind: file
    spec:
      file: {{ $consumer.path | quote }}
      line: 1
      content: {{ printf "{{ source %q }}" (printf "%s-consumer-%d" $tool.name $consumerIndex) | quote }}
{{- else }}
    kind: file
    spec:
      file: {{ $consumer.path | quote }}
      matchpattern: {{ printf "(?m)^%s{{ source %q }}%s\\r?$" ($consumer.prefix | regexQuoteMeta) (printf "%s-consumer-%d" $tool.name $consumerIndex) ($consumer.suffix | regexQuoteMeta) | quote }}
{{- end }}
{{- end }}
{{- end }}

targets:
{{- range $index, $tool := .tools }}
{{- $versionID := printf "%s-version" $tool.name }}
{{- $version := printf "{{ source %q }}" $versionID }}
{{- $enabled := true }}
{{- if hasKey $tool "update" }}{{ $enabled = $tool.update }}{{ end }}
  {{ $tool.name }}-select-version:
    name: Select {{ $tool.name }} without downgrading
    kind: file
    disablesourceinput: true
    disableconditions: true
    spec:
      file: {{ printf "bin/updatecli/%s.version" $tool.name | quote }}
      forcecreate: true
      template: .updatecli/version.tpl
      templatedata:
        enabled: {{ $enabled }}
        current: {{ printf "{{ source %q }}" (printf "%s-pin" $tool.name) | quote }}
{{- if $refresh }}
        latest: {{ printf "{{ source %q }}" (printf "%s-latest" $tool.name) | quote }}
{{- else }}
        latest: {{ printf "{{ source %q }}" (printf "%s-pin" $tool.name) | quote }}
{{- end }}
  {{ $tool.name }}-version:
    name: Update {{ $tool.name }} version
    kind: yaml
    sourceid: {{ $versionID }}
    spec:
      file: {{ $manifest | quote }}
      key: {{ printf "$.tools[%d].version" $index | quote }}
{{- if ne $tool.checksumSource.type "none" }}
{{- range $platform := $.platforms }}
  {{ $tool.name }}-{{ $platform }}-checksum:
    name: Update {{ $tool.name }} {{ $platform }} checksum
    kind: yaml
    sourceid: {{ printf "%s-%s-checksum" $tool.name $platform }}
    spec:
      file: {{ $manifest | quote }}
      key: {{ printf "$.tools[%d].platforms.%s.checksum" $index $platform | quote }}
{{- end }}
{{- end }}
{{- range $consumerIndex, $consumer := $tool.versionFiles }}
  {{ $tool.name }}-consumer-{{ $consumerIndex }}:
    name: Synchronize {{ $consumer.path }}
    sourceid: {{ $versionID }}
{{- if eq $consumer.format "yaml" }}
    kind: yaml
    spec:
      file: {{ $consumer.path | quote }}
      key: {{ $consumer.key | quote }}
{{- else }}
    kind: file
    spec:
      file: {{ $consumer.path | quote }}
{{- if eq $consumer.format "plain" }}
      content: {{ printf "%s\n" $version | quote }}
{{- else }}
      matchpattern: {{ printf "(?m)^%s[^\\r\\n]*?%s" ($consumer.prefix | regexQuoteMeta) ($consumer.suffix | regexQuoteMeta) | quote }}
      replacepattern: {{ printf "%s%s%s" $consumer.prefix $version $consumer.suffix | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
  make-metadata:
    name: Regenerate pinned Make metadata
    kind: file
    disablesourceinput: true
    spec:
      file: {{ $makefile | quote }}
      template: .updatecli/tools.mk.tpl
      templatedata:
        platforms:
{{- range .platforms }}
          - {{ . }}
{{- end }}
        tools:
{{- range $tool := .tools }}
          - makePrefix: {{ $tool.makePrefix }}
            version: {{ printf "{{ source %q }}" (printf "%s-version" $tool.name) | quote }}
            checksums:
{{- if ne $tool.checksumSource.type "none" }}
{{- range $platform := $.platforms }}
              {{ $platform }}: {{ printf "{{ source %q }}" (printf "%s-%s-checksum" $tool.name $platform) | quote }}
{{- end }}
{{- else }}
              {}
{{- end }}
{{- end }}
