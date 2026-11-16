{{- define "proxmox-host-labeler.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "proxmox-host-labeler.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "proxmox-host-labeler.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
