{{- $newer := semverCompare (printf ">%s" .current) .latest -}}
{{- if and .enabled $newer -}}
{{- .latest -}}
{{- else -}}
{{- .current -}}
{{- end -}}
