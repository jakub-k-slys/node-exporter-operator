package exporters

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	exporterv1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
	exportertypes "github.com/jakub-k-slys/node-exporter-operator/internal/controller/types"
)

// GetBlackboxExporterConfig returns configuration for Blackbox Exporter
func GetBlackboxExporterConfig(ne *exporterv1alpha1.NodeExporter) exportertypes.ExporterConfig {
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "blackbox-exporter-config",
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/config",
		},
	}

	return exportertypes.ExporterConfig{
		Name:  "blackbox-exporter",
		Image: "prom/blackbox-exporter:v0.24.0",
		Port:  exportertypes.BlackboxExporterPort,
		ContainerConfig: func(cfg *exportertypes.ExporterConfig) corev1.Container {
			return corev1.Container{
				Name:  cfg.Name,
				Image: cfg.Image,
				Args: []string{
					"--config.file=/config/blackbox.yml",
					"--web.listen-address=:9115",
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: cfg.Port,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("30Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("50Mi"),
					},
				},
				VolumeMounts: volumeMounts,
			}
		},
		Volumes: volumes,
	}
}
