{{/*
memcached StatefulSet
*/}}
{{- define "mimir.memcached.statefulSet" -}}
{{ with (index $.ctx.Values $.component) }}
{{- if .enabled -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "mimir.resourceName" (dict "ctx" $.ctx "component" $.component) }}
  labels:
    {{- include "mimir.labels" (dict "ctx" $.ctx "component" "memcached") | nindent 4 }}
  annotations:
    {{- toYaml .annotations | nindent 4 }}
spec:
  podManagementPolicy: {{ .podManagementPolicy }}
  replicas: {{ .replicas }}
  selector:
    matchLabels:
      {{- include "mimir.selectorLabels" (dict "ctx" $.ctx "component" $.component) | nindent 6 }}
  updateStrategy:
    {{- toYaml .statefulStrategy | nindent 4 }}
  serviceName: {{ template "mimir.fullname" $.ctx }}-{{ $.component }}

  template:
    metadata:
      labels:
        {{- include "mimir.podLabels" (dict "ctx" $.ctx "component" $.component) | nindent 8 }}
        {{- with .podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      annotations:
        {{- with .podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}

    spec:
      serviceAccountName: {{ template "mimir.serviceAccountName" $.ctx }}
      {{- if .priorityClassName }}
      priorityClassName: {{ .priorityClassName }}
      {{- end }}
      securityContext:
        {{- toYaml .securityContext | nindent 8 }}
      initContainers:
        {{- toYaml .initContainers | nindent 8 }}
      {{- if .image.pullSecrets }}
      imagePullSecrets:
      {{- range .image.pullSecrets }}
        - name: {{ . }}
      {{- end }}
      {{- end }}
      nodeSelector:
        {{- toYaml .nodeSelector | nindent 8 }}
      affinity:
        {{- toYaml .affinity | nindent 8 }}
      tolerations:
        {{- toYaml .tolerations | nindent 8 }}
      terminationGracePeriodSeconds: {{ .terminationGracePeriodSeconds }}
      containers:
        {{- if .extraContainers }}
        {{ toYaml .extraContainers | nindent 8 }}
        {{- end }}
        - name: memcached
          image: {{ .image.repository}}:{{ .image.tag }}
          imagePullPolicy: {{ .image.pullPolicy }}
          resources:
            {{- toYaml .resources | nindent 12 }}
          ports:
            - containerPort: {{ .port }}
              name: client
          args:
            - -m {{ .allocatedMemory }}
            - -o
            - modern
            - -I {{ .maxItemMemory }}m
            - -c 16384
            - -v
            - -u {{ .port }}
            {{- range $key, $value := .extraArgs }}
            - "-{{ $key }} {{ $value }}"
            {{- end }}

      {{- if .metrics.enabled }}
        - name: exporter
          image: {{ .metrics.image.repository}}:{{ .metrics.image.tag }}
          imagePullPolicy: {{ .metrics.image.pullPolicy }}
          ports:
            - containerPort: {{ .metrics.containerPort }}
              name: http-metrics
          args:
            {{- range $value := .metrics.args }}
            - {{ $value }}
            {{- end }}
      {{- end }}
{{- end -}}
{{- end -}}
{{- end -}}
