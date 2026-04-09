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

{{- define "k8s-lb-controller.dataplaneFullname" -}}
{{- printf "%s-dataplane" (include "k8s-lb-controller.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8s-lb-controller.dataplaneSelectorLabels" -}}
{{ include "k8s-lb-controller.selectorLabels" . }}
app.kubernetes.io/component: dataplane
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

{{- define "k8s-lb-controller.dataplaneHTTPAddr" -}}
{{- if .Values.dataplane.http.addr -}}
{{- .Values.dataplane.http.addr -}}
{{- else -}}
{{- printf ":%d" (int .Values.dataplane.http.port) -}}
{{- end -}}
{{- end -}}

{{- define "k8s-lb-controller.dataplaneImage" -}}
{{- printf "%s:%s" .Values.dataplane.image.repository (default .Chart.AppVersion .Values.dataplane.image.tag) -}}
{{- end -}}

{{- define "k8s-lb-controller.dataplaneHAProxyImage" -}}
{{- printf "%s:%s" .Values.dataplane.haproxy.image.repository (default .Chart.AppVersion .Values.dataplane.haproxy.image.tag) -}}
{{- end -}}

{{- define "k8s-lb-controller.dataplaneServiceURL" -}}
{{- printf "http://%s.%s.svc:%d" (include "k8s-lb-controller.dataplaneFullname" .) .Release.Namespace (int .Values.dataplane.http.port) -}}
{{- end -}}

{{- define "k8s-lb-controller.controllerDataplaneAPIURL" -}}
{{- if .Values.controller.dataplane.apiURL -}}
{{- .Values.controller.dataplane.apiURL -}}
{{- else if .Values.dataplane.enabled -}}
{{- include "k8s-lb-controller.dataplaneServiceURL" . -}}
{{- end -}}
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
{{- if .Values.dataplane.enabled -}}
  {{- $dataplaneTimeoutSeconds := include "k8s-lb-controller.durationSeconds" .Values.dataplane.gracefulShutdownTimeout | trim -}}
  {{- if and $dataplaneTimeoutSeconds (lt (float64 .Values.dataplane.terminationGracePeriodSeconds) ($dataplaneTimeoutSeconds | float64)) -}}
    {{- fail (printf "dataplane.terminationGracePeriodSeconds (%d) must be greater than or equal to dataplane.gracefulShutdownTimeout (%s)" (int .Values.dataplane.terminationGracePeriodSeconds) .Values.dataplane.gracefulShutdownTimeout) -}}
  {{- end -}}
  {{- if .Values.dataplane.ipAttach.enabled -}}
    {{- if not (.Values.dataplane.interface | trim) -}}
      {{- fail "dataplane.interface must be set when dataplane.ipAttach.enabled=true" -}}
    {{- end -}}
  {{- end -}}
  {{- if ne (dir .Values.dataplane.haproxy.configPath) (dir .Values.dataplane.haproxy.pidFile) -}}
    {{- fail "dataplane.haproxy.configPath and dataplane.haproxy.pidFile must use the same runtime directory" -}}
  {{- end -}}
{{- end -}}
{{- if eq .Values.controller.providerMode "dataplane-api" -}}
  {{- $apiURL := include "k8s-lb-controller.controllerDataplaneAPIURL" . | trim -}}
  {{- if not $apiURL -}}
    {{- fail "controller.providerMode=dataplane-api requires dataplane.enabled=true or controller.dataplane.apiURL to be set" -}}
  {{- end -}}
{{- end -}}
{{- end -}}
