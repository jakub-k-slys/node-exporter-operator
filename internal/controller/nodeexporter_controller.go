/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
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
	nodeExporterPort     int32 = 9100
	blackboxExporterPort int32 = 9115
	kubeStateMetricsPort int32 = 8080
	metricsPath                = "/metrics"
	scrapeInterval             = "30s"
)

// NodeExporterReconciler reconciles a NodeExporter object
type NodeExporterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.slys.dev,resources=nodeexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.slys.dev,resources=nodeexporters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.slys.dev,resources=nodeexporters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

func (r *NodeExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NodeExporter")

	// Fetch the NodeExporter instance
	nodeExporter := &cachev1alpha1.NodeExporter{}
	if err := r.Get(ctx, req.NamespacedName, nodeExporter); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NodeExporter resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NodeExporter")
		return ctrl.Result{}, err
	}

	// Reconcile each exporter
	if err := r.reconcileExporter(ctx, r.getNodeExporterConfig(nodeExporter), nodeExporter.Spec.NodeExporterSettings.Enabled); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileExporter(ctx, r.getBlackboxExporterConfig(nodeExporter), nodeExporter.Spec.BlackboxExporterSettings.Enabled); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileExporter(ctx, r.getKubeStateMetricsConfig(nodeExporter), nodeExporter.Spec.KubeStateMetricsSettings.Enabled); err != nil {
		return ctrl.Result{}, err
	}

	// Update status for NodeExporter
	if nodeExporter.Spec.NodeExporterSettings.Enabled {
		condition := metav1.Condition{
			Type:               "NodeExporterAvailable",
			Status:             metav1.ConditionTrue,
			Reason:             "NodeExporterDeployed",
			Message:            "Node Exporter DaemonSet is deployed",
			LastTransitionTime: metav1.Now(),
		}
		nodeExporter.Status.Conditions = []metav1.Condition{condition}
		if err := r.Status().Update(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to update NodeExporter status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileExporter handles the lifecycle of an exporter
func (r *NodeExporterReconciler) reconcileExporter(ctx context.Context, cfg ExporterConfig, enabled bool) error {
	logger := log.FromContext(ctx)

	if !enabled {
		return r.cleanupExporter(ctx, cfg)
	}

	// Create or update resources
	if err := r.reconcileResources(ctx, cfg); err != nil {
		logger.Error(err, "Failed to reconcile resources", "exporter", cfg.Name)
		return err
	}

	return nil
}

// reconcileResources creates or updates all resources for an exporter
func (r *NodeExporterReconciler) reconcileResources(ctx context.Context, cfg ExporterConfig) error {
	logger := log.FromContext(ctx)

	// Get NodeExporter instance for owner reference
	nodeExporter := &cachev1alpha1.NodeExporter{}
	if err := r.Get(ctx, types.NamespacedName{Name: cfg.Name, Namespace: "default"}, nodeExporter); err != nil {
		return err
	}

	// Create ServiceAccount
	if err := r.createServiceAccount(ctx, cfg.Name, nodeExporter.Namespace, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ServiceAccount", "exporter", cfg.Name)
		return err
	}

	// Create or update Service
	if err := r.createOrUpdateService(ctx, nodeExporter, cfg.Name, cfg.Port); err != nil {
		logger.Error(err, "Failed to create/update Service", "exporter", cfg.Name)
		return err
	}

	// Create or update ServiceMonitor
	if err := r.createOrUpdateServiceMonitor(ctx, nodeExporter, cfg.Name); err != nil {
		logger.Error(err, "Failed to create/update ServiceMonitor", "exporter", cfg.Name)
		return err
	}

	// Create or update DaemonSet
	if err := r.createOrUpdateDaemonSet(ctx, cfg, nodeExporter); err != nil {
		logger.Error(err, "Failed to create/update DaemonSet", "exporter", cfg.Name)
		return err
	}

	return nil
}

// createOrUpdateDaemonSet creates or updates a DaemonSet for an exporter
func (r *NodeExporterReconciler) createOrUpdateDaemonSet(ctx context.Context, cfg ExporterConfig, nodeExporter *cachev1alpha1.NodeExporter) error {
	labels := map[string]string{
		"app":        cfg.Name,
		"controller": nodeExporter.Name,
	}
	for k, v := range cfg.ExtraLabels {
		labels[k] = v
	}

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", cfg.Name, nodeExporter.Name),
			Namespace: nodeExporter.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers:  []corev1.Container{cfg.ContainerConfig(&cfg)},
					Volumes:     cfg.Volumes,
					HostNetwork: cfg.HostNetwork,
					HostPID:     cfg.HostPID,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0],
					},
					ServiceAccountName: cfg.Name,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(nodeExporter, daemonSet, r.Scheme); err != nil {
		return err
	}

	// Check if DaemonSet exists
	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, daemonSet)
		}
		return err
	}

	// Update existing DaemonSet
	found.Spec = daemonSet.Spec
	found.Labels = daemonSet.Labels
	return r.Update(ctx, found)
}

// cleanupExporter removes all resources for an exporter
func (r *NodeExporterReconciler) cleanupExporter(ctx context.Context, cfg ExporterConfig) error {
	nodeExporter := &cachev1alpha1.NodeExporter{}
	if err := r.Get(ctx, types.NamespacedName{Name: cfg.Name, Namespace: "default"}, nodeExporter); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	name := fmt.Sprintf("%s-%s", cfg.Name, nodeExporter.Name)
	ns := nodeExporter.Namespace

	resources := []client.Object{
		&appsv1.DaemonSet{},
		&corev1.Service{},
		&monitoringv1.ServiceMonitor{},
		&corev1.ServiceAccount{},
	}

	for _, obj := range resources {
		if err := r.cleanupResource(ctx, obj, name, ns); err != nil {
			return err
		}
	}

	return nil
}

// getNodeExporterConfig returns configuration for Node Exporter
func (r *NodeExporterReconciler) getNodeExporterConfig(ne *cachev1alpha1.NodeExporter) ExporterConfig {
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

	return ExporterConfig{
		Name:  "node-exporter",
		Image: "prom/node-exporter:v1.7.0",
		Port:  nodeExporterPort,
		ContainerConfig: func(cfg *ExporterConfig) corev1.Container {
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

// getBlackboxExporterConfig returns configuration for Blackbox Exporter
func (r *NodeExporterReconciler) getBlackboxExporterConfig(ne *cachev1alpha1.NodeExporter) ExporterConfig {
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

	return ExporterConfig{
		Name:  "blackbox-exporter",
		Image: "prom/blackbox-exporter:v0.24.0",
		Port:  blackboxExporterPort,
		ContainerConfig: func(cfg *ExporterConfig) corev1.Container {
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

// getKubeStateMetricsConfig returns configuration for Kube State Metrics
func (r *NodeExporterReconciler) getKubeStateMetricsConfig(ne *cachev1alpha1.NodeExporter) ExporterConfig {
	return ExporterConfig{
		Name:  "kube-state-metrics",
		Image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1",
		Port:  kubeStateMetricsPort,
		ContainerConfig: func(cfg *ExporterConfig) corev1.Container {
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

// Helper functions from the original implementation remain unchanged
func (r *NodeExporterReconciler) createServiceAccount(ctx context.Context, name string, namespace string, nodeExporter *cachev1alpha1.NodeExporter) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        name,
				"controller": nodeExporter.Name,
			},
		},
	}

	if err := controllerutil.SetControllerReference(nodeExporter, sa, r.Scheme); err != nil {
		return err
	}

	found := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, sa)
		}
		return err
	}

	return nil
}

func (r *NodeExporterReconciler) createOrUpdateService(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter, name string, port int32) error {
	labels := map[string]string{
		"app":        name,
		"controller": nodeExporter.Name,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", name, nodeExporter.Name),
			Namespace: nodeExporter.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "metrics",
				Port:       port,
				TargetPort: intstr.FromInt(int(port)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	if err := controllerutil.SetControllerReference(nodeExporter, svc, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, svc)
		}
		return err
	}

	found.Spec = svc.Spec
	found.Labels = svc.Labels
	return r.Update(ctx, found)
}

func (r *NodeExporterReconciler) createOrUpdateServiceMonitor(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter, name string) error {
	labels := map[string]string{
		"app":        name,
		"controller": nodeExporter.Name,
	}

	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", name, nodeExporter.Name),
			Namespace: nodeExporter.Namespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Endpoints: []monitoringv1.Endpoint{{
				Port:     "metrics",
				Path:     metricsPath,
				Interval: scrapeInterval,
			}},
		},
	}

	if err := controllerutil.SetControllerReference(nodeExporter, sm, r.Scheme); err != nil {
		return err
	}

	found := &monitoringv1.ServiceMonitor{}
	err := r.Get(ctx, types.NamespacedName{Name: sm.Name, Namespace: sm.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, sm)
		}
		return err
	}

	found.Spec = sm.Spec
	found.Labels = sm.Labels
	return r.Update(ctx, found)
}

func (r *NodeExporterReconciler) cleanupResource(ctx context.Context, obj client.Object, name, namespace string) error {
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Delete(ctx, obj)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NodeExporter{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Complete(r)
}
