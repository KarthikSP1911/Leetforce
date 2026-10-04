{{- define "leetforce-api.labels" -}}
app.kubernetes.io/name: api
app.kubernetes.io/part-of: leetforce
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "leetforce-api.selectorLabels" -}}
app.kubernetes.io/name: api
{{- end }}
