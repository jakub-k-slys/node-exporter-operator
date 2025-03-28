package exporters

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	exporterv1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
	exportertypes "github.com/jakub-k-slys/node-exporter-operator/internal/controller/types"
)

// GetKubeStateMetricsConfig returns configuration for Kube State Metrics
func GetKubeStateMetricsConfig(ne *exporterv1alpha1.NodeExporter) exportertypes.ExporterConfig {
	return exportertypes.ExporterConfig{
		Name:  "kube-state-metrics",
		Image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1",
		Port:  exportertypes.KubeStateMetricsPort,
		ContainerConfig: func(cfg *exportertypes.ExporterConfig) corev1.Container {
			return corev1.Container{
				Name:  cfg.Name,
				Image: cfg.Image,
				Args: []string{
					"--port=8080",
					"--telemetry-port=8081",
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "metrics",
						ContainerPort: cfg.Port,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          "telemetry",
						ContainerPort: 8081,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &[]bool{false}[0],
					ReadOnlyRootFilesystem:   &[]bool{true}[0],
					RunAsUser:                &[]int64{65534}[0],
				},
			}
		},
	}
}
