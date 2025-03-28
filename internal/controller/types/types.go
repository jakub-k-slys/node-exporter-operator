package exportertypes

import (
	corev1 "k8s.io/api/core/v1"
)

// ExporterConfig holds configuration for an exporter
type ExporterConfig struct {
	Name            string
	Image           string
	Port            int32
	ContainerConfig func(cfg *ExporterConfig) corev1.Container
	Volumes         []corev1.Volume
	VolumeMounts    []corev1.VolumeMount
	ExtraLabels     map[string]string
	HostNetwork     bool
	HostPID         bool
}

// Constants for configuration
const (
	NodeExporterPort     int32 = 9100
	BlackboxExporterPort int32 = 9115
	KubeStateMetricsPort int32 = 8080
	MetricsPath                = "/metrics"
	ScrapeInterval             = "30s"
)
