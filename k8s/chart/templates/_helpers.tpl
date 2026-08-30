{{- define "panel.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- end -}}

{{- define "panel.secretName" -}}
{{- if .Values.existingSecret -}}
{{ .Values.existingSecret }}
{{- else -}}
{{ .Release.Name }}-secrets
{{- end -}}
{{- end -}}

{{/*
Everything the panel reads from the environment. Secrets come from a Secret, the
rest from a ConfigMap, and the database DSN from wherever the operator keeps it -
templating a password into a ConfigMap would put it in plain sight.
*/}}
{{- define "panel.env" -}}
envFrom:
  - configMapRef:
      name: {{ .Release.Name }}-config
  - secretRef:
      name: {{ include "panel.secretName" . }}
{{- if and (ne .Values.database.driver "sqlite") .Values.database.dsnSecret.name }}
env:
  - name: DB_DSN
    valueFrom:
      secretKeyRef:
        name: {{ .Values.database.dsnSecret.name }}
        key: {{ .Values.database.dsnSecret.key }}
{{- end }}
{{- end -}}
