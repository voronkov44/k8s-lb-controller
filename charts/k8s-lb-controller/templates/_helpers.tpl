{{- define "k8s-lb-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8s-lb-controller.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "k8s-lb-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8s-lb-controller.labels" -}}
helm.sh/chart: {{ include "k8s-lb-controller.chart" . }}
{{ include "k8s-lb-controller.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Chart.AppVersion }}
app.kubernetes.io/version: {{ . | quote }}
{{- end }}
{{- end -}}

{{- define "k8s-lb-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-lb-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "k8s-lb-controller.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "k8s-lb-controller.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- required "serviceAccount.name is required when serviceAccount.create=false" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "k8s-lb-controller.ipPool" -}}
{{- join "," .Values.controller.ipPool -}}
{{- end -}}

{{- define "k8s-lb-controller.metricsAddr" -}}
{{- printf ":%d" (int .Values.metrics.port) -}}
{{- end -}}

{{- define "k8s-lb-controller.healthAddr" -}}
{{- printf ":%d" (int .Values.health.port) -}}
{{- end -}}

{{- define "k8s-lb-controller.durationSeconds" -}}
{{- $duration := . | toString | trim -}}
{{- $pattern := "^[0-9]+(?:\\.[0-9]+)?(?:ns|us|ms|s|m|h)(?:[0-9]+(?:\\.[0-9]+)?(?:ns|us|ms|s|m|h))*$" -}}
{{- if regexMatch $pattern $duration -}}
{{- $seconds := 0.0 -}}
{{- range regexFindAll "[0-9]+(?:\\.[0-9]+)?(?:ns|us|ms|s|m|h)" $duration -1 }}
  {{- $segment := . -}}
  {{- $value := regexFind "^[0-9]+(?:\\.[0-9]+)?" $segment | float64 -}}
  {{- $unit := regexFind "(?:ns|us|ms|s|m|h)$" $segment -}}
  {{- if eq $unit "h" -}}
    {{- $seconds = addf $seconds (mulf $value 3600) -}}
  {{- else if eq $unit "m" -}}
    {{- $seconds = addf $seconds (mulf $value 60) -}}
  {{- else if eq $unit "s" -}}
    {{- $seconds = addf $seconds $value -}}
  {{- else if eq $unit "ms" -}}
    {{- $seconds = addf $seconds (divf $value 1000) -}}
  {{- else if eq $unit "us" -}}
    {{- $seconds = addf $seconds (divf $value 1000000) -}}
  {{- else if eq $unit "ns" -}}
    {{- $seconds = addf $seconds (divf $value 1000000000) -}}
  {{- end -}}
{{- end -}}
{{- printf "%f" $seconds -}}
{{- end -}}
{{- end -}}

{{- define "k8s-lb-controller.validateValues" -}}
{{- $timeoutSeconds := include "k8s-lb-controller.durationSeconds" .Values.controller.gracefulShutdownTimeout | trim -}}
{{- if $timeoutSeconds -}}
  {{- $terminationSeconds := float64 .Values.terminationGracePeriodSeconds -}}
  {{- if lt $terminationSeconds ($timeoutSeconds | float64) -}}
    {{- fail (printf "terminationGracePeriodSeconds (%d) must be greater than or equal to controller.gracefulShutdownTimeout (%s)" (int .Values.terminationGracePeriodSeconds) .Values.controller.gracefulShutdownTimeout) -}}
  {{- end -}}
{{- end -}}
{{- end -}}
