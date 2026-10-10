{{- define "leetforce-runner.labels" -}}
app.kubernetes.io/name: runner
app.kubernetes.io/part-of: leetforce
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "leetforce-runner.selectorLabels" -}}
app.kubernetes.io/name: runner
{{- end }}
