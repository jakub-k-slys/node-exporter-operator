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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
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
func (r *NodeExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NodeExporter instance
	nodeExporter := &cachev1alpha1.NodeExporter{}
	err := r.Get(ctx, req.NamespacedName, nodeExporter)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			logger.Info("NodeExporter resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get NodeExporter")
		return ctrl.Result{}, err
	}

	// Handle Node Exporter
	if err := r.reconcileNodeExporter(ctx, nodeExporter); err != nil {
		return ctrl.Result{}, err
	}

	// Handle Blackbox Exporter
	if err := r.reconcileBlackboxExporter(ctx, nodeExporter); err != nil {
		return ctrl.Result{}, err
	}

	// Handle Kube State Metrics
	if err := r.reconcileKubeStateMetrics(ctx, nodeExporter); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileNodeExporter handles the Node Exporter DaemonSet lifecycle
func (r *NodeExporterReconciler) reconcileNodeExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	logger := log.FromContext(ctx)

	if !nodeExporter.Spec.NodeExporterSettings.Enabled {
		// Node exporter is disabled, ensure the DaemonSet is removed
		if err := r.cleanupNodeExporter(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup NodeExporter")
			return err
		}
		return nil
	}

	// Create or update the DaemonSet
	daemonSet := r.nodeExporterDaemonSet(nodeExporter)

	// Check if the DaemonSet already exists
	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet does not exist, create it
			logger.Info("Creating a new NodeExporter DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
			err = r.Create(ctx, daemonSet)
			if err != nil {
				logger.Error(err, "Failed to create new NodeExporter DaemonSet")
				return err
			}
		} else {
			logger.Error(err, "Failed to get NodeExporter DaemonSet")
			return err
		}
	} else {
		// DaemonSet exists, update it
		logger.Info("Updating existing NodeExporter DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
		err = r.Update(ctx, daemonSet)
		if err != nil {
			logger.Error(err, "Failed to update NodeExporter DaemonSet")
			return err
		}
	}

	// Update status
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
		return err
	}

	return nil
}

// reconcileBlackboxExporter handles the Blackbox Exporter DaemonSet lifecycle
func (r *NodeExporterReconciler) reconcileBlackboxExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	logger := log.FromContext(ctx)

	if !nodeExporter.Spec.BlackboxExporterSettings.Enabled {
		// Blackbox exporter is disabled, ensure the DaemonSet is removed
		if err := r.cleanupBlackboxExporter(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup BlackboxExporter")
			return err
		}
		return nil
	}

	// Create or update the DaemonSet
	daemonSet := r.blackboxExporterDaemonSet(nodeExporter)

	// Check if the DaemonSet already exists
	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet does not exist, create it
			logger.Info("Creating a new BlackboxExporter DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
			err = r.Create(ctx, daemonSet)
			if err != nil {
				logger.Error(err, "Failed to create new BlackboxExporter DaemonSet")
				return err
			}
		} else {
			logger.Error(err, "Failed to get BlackboxExporter DaemonSet")
			return err
		}
	} else {
		// DaemonSet exists, update it
		logger.Info("Updating existing BlackboxExporter DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
		err = r.Update(ctx, daemonSet)
		if err != nil {
			logger.Error(err, "Failed to update BlackboxExporter DaemonSet")
			return err
		}
	}

	return nil
}

// cleanupNodeExporter removes the Node Exporter DaemonSet if it exists
func (r *NodeExporterReconciler) cleanupNodeExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	// Check if DaemonSet exists
	daemonSet := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "node-exporter-" + nodeExporter.Name,
		Namespace: nodeExporter.Namespace,
	}, daemonSet)

	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet doesn't exist, nothing to do
			return nil
		}
		return err
	}

	// DaemonSet exists, delete it
	return r.Delete(ctx, daemonSet)
}

// cleanupBlackboxExporter removes the Blackbox Exporter DaemonSet if it exists
func (r *NodeExporterReconciler) cleanupBlackboxExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	// Check if DaemonSet exists
	daemonSet := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "blackbox-exporter-" + nodeExporter.Name,
		Namespace: nodeExporter.Namespace,
	}, daemonSet)

	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet doesn't exist, nothing to do
			return nil
		}
		return err
	}

	// DaemonSet exists, delete it
	return r.Delete(ctx, daemonSet)
}

// blackboxExporterDaemonSet returns a blackbox-exporter DaemonSet object
func (r *NodeExporterReconciler) blackboxExporterDaemonSet(nodeExporter *cachev1alpha1.NodeExporter) *appsv1.DaemonSet {
	labels := map[string]string{
		"app":        "blackbox-exporter",
		"controller": nodeExporter.Name,
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blackbox-exporter-" + nodeExporter.Name,
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
					Containers: []corev1.Container{
						{
							Name:  "blackbox-exporter",
							Image: "prom/blackbox-exporter:v0.24.0",
							Args: []string{
								"--config.file=/config/blackbox.yml",
								"--web.listen-address=:9115",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9115,
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/config",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
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
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0],
					},
					ServiceAccountName: "blackbox-exporter",
				},
			},
		},
	}
}

// nodeExporterDaemonSet returns a node-exporter DaemonSet object
func (r *NodeExporterReconciler) nodeExporterDaemonSet(nodeExporter *cachev1alpha1.NodeExporter) *appsv1.DaemonSet {
	labels := map[string]string{
		"app":        "node-exporter",
		"controller": nodeExporter.Name,
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-exporter-" + nodeExporter.Name,
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
					Containers: []corev1.Container{
						{
							Name:  "node-exporter",
							Image: "prom/node-exporter:v1.7.0",
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
									ContainerPort: 9100,
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
							VolumeMounts: []corev1.VolumeMount{
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
							},
						},
					},
					HostNetwork: true,
					HostPID:     true,
					Volumes: []corev1.Volume{
						{
							Name: "proc",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/proc",
								},
							},
						},
						{
							Name: "sys",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/sys",
								},
							},
						},
						{
							Name: "root",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/",
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0],
					},
					ServiceAccountName: "node-exporter",
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
				},
			},
		},
	}
}

// reconcileKubeStateMetrics handles the Kube State Metrics DaemonSet lifecycle
func (r *NodeExporterReconciler) reconcileKubeStateMetrics(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	logger := log.FromContext(ctx)

	if !nodeExporter.Spec.KubeStateMetricsSettings.Enabled {
		// Kube State Metrics is disabled, ensure the DaemonSet is removed
		if err := r.cleanupKubeStateMetrics(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup KubeStateMetrics")
			return err
		}
		return nil
	}

	// Create or update the DaemonSet
	daemonSet := r.kubeStateMetricsDaemonSet(nodeExporter)

	// Check if the DaemonSet already exists
	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet does not exist, create it
			logger.Info("Creating a new KubeStateMetrics DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
			err = r.Create(ctx, daemonSet)
			if err != nil {
				logger.Error(err, "Failed to create new KubeStateMetrics DaemonSet")
				return err
			}
		} else {
			logger.Error(err, "Failed to get KubeStateMetrics DaemonSet")
			return err
		}
	} else {
		// DaemonSet exists, update it
		logger.Info("Updating existing KubeStateMetrics DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
		err = r.Update(ctx, daemonSet)
		if err != nil {
			logger.Error(err, "Failed to update KubeStateMetrics DaemonSet")
			return err
		}
	}

	return nil
}

// cleanupKubeStateMetrics removes the Kube State Metrics DaemonSet if it exists
func (r *NodeExporterReconciler) cleanupKubeStateMetrics(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	// Check if DaemonSet exists
	daemonSet := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "kube-state-metrics-" + nodeExporter.Name,
		Namespace: nodeExporter.Namespace,
	}, daemonSet)

	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet doesn't exist, nothing to do
			return nil
		}
		return err
	}

	// DaemonSet exists, delete it
	return r.Delete(ctx, daemonSet)
}

// kubeStateMetricsDaemonSet returns a kube-state-metrics DaemonSet object
func (r *NodeExporterReconciler) kubeStateMetricsDaemonSet(nodeExporter *cachev1alpha1.NodeExporter) *appsv1.DaemonSet {
	labels := map[string]string{
		"app":        "kube-state-metrics",
		"controller": nodeExporter.Name,
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-state-metrics-" + nodeExporter.Name,
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
					Containers: []corev1.Container{
						{
							Name:  "kube-state-metrics",
							Image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1",
							Args: []string{
								"--port=8080",
								"--telemetry-port=8081",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 8080,
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
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0],
					},
					ServiceAccountName: "kube-state-metrics",
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NodeExporter{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}
