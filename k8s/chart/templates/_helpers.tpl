{{/*
Names are fixed, not derived from the release name. Service names end up in a
DSN, in an IngressRoute and in whatever else points at the panel from outside
the chart, and a name that moves when someone renames the release turns those
into silent misroutes.
*/}}
{{- define "panel.name" -}}
{{ .Values.nameOverride | default "panel" }}
{{- end -}}

{{/*
The chart installs into wingsvpn whether or not -n was passed. Helm still keeps
its release metadata in the namespace of -n, so pass "-n wingsvpn" as well or
"helm list -n wingsvpn" will not find the release it just installed.
*/}}
{{- define "panel.namespace" -}}
{{ .Values.namespace | default .Release.Namespace }}
{{- end -}}

{{- define "panel.pgName" -}}
{{ include "panel.name" . }}-pg
{{- end -}}

{{- define "panel.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- end -}}

{{- define "panel.secretName" -}}
{{- if .Values.existingSecret -}}
{{ .Values.existingSecret }}
{{- else -}}
{{ include "panel.name" . }}-secrets
{{- end -}}
{{- end -}}

{{- define "panel.tlsSecret" -}}
{{- if .Values.ingress.tls.secretName -}}
{{ .Values.ingress.tls.secretName }}
{{- else -}}
{{ include "panel.name" . }}-tls
{{- end -}}
{{- end -}}

{{/*
Where the DSN comes from. With the bundled CNPG cluster the chart renders it
itself from the same values that seed the database, so the credentials are
written down once. Pointing at a database somebody else manages means handing
over a secret instead.
*/}}
{{- define "panel.dsnSecret" -}}
{{- if .Values.postgres.enabled -}}
{{ include "panel.name" . }}-dsn
{{- else -}}
{{ .Values.database.dsnSecret.name }}
{{- end -}}
{{- end -}}

{{- define "panel.dsnKey" -}}
{{- if .Values.postgres.enabled -}}dsn{{- else -}}{{ .Values.database.dsnSecret.key }}{{- end -}}
{{- end -}}

{{- define "panel.env" -}}
envFrom:
  - configMapRef:
      name: {{ include "panel.name" . }}-config
  - secretRef:
      name: {{ include "panel.secretName" . }}
{{- if and (ne .Values.database.driver "sqlite") (include "panel.dsnSecret" .) }}
env:
  - name: DB_DSN
    valueFrom:
      secretKeyRef:
        name: {{ include "panel.dsnSecret" . }}
        key: {{ include "panel.dsnKey" . }}
{{- end }}
{{- end -}}
