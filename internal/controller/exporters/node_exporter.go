package exporters

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	exporterv1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
	exportertypes "github.com/jakub-k-slys/node-exporter-operator/internal/controller/types"
)

// GetNodeExporterConfig returns configuration for Node Exporter
func GetNodeExporterConfig(ne *exporterv1alpha1.NodeExporter) exportertypes.ExporterConfig {
	volumes := []corev1.Volume{
		{
			Name: "proc",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/proc"},
			},
		},
		{
			Name: "sys",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/sys"},
			},
		},
		{
			Name: "root",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/"},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "proc",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "sys",
			MountPath: "/host/sys",
			ReadOnly:  true,
		},
		{
			Name:      "root",
			MountPath: "/host/root",
			ReadOnly:  true,
		},
	}

	return exportertypes.ExporterConfig{
		Name:  "node-exporter",
		Image: "prom/node-exporter:v1.7.0",
		Port:  exportertypes.NodeExporterPort,
		ContainerConfig: func(cfg *exportertypes.ExporterConfig) corev1.Container {
			return corev1.Container{
				Name:  cfg.Name,
				Image: cfg.Image,
				Args: []string{
					"--web.listen-address=:9100",
					"--path.procfs=/host/proc",
					"--path.sysfs=/host/sys",
					"--path.rootfs=/host/root",
					"--collector.filesystem.mount-points-exclude=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/pods/.+)($|/)",
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "metrics",
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
		Volumes:     volumes,
		HostNetwork: true,
		HostPID:     true,
	}
}
