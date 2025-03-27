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

	// Check if nodeExporter is enabled
	if !nodeExporter.Spec.NodeExporterSettings.Enabled {
		// Node exporter is disabled, ensure the DaemonSet is removed
		if err := r.cleanupNodeExporter(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup NodeExporter")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Create or update the DaemonSet
	daemonSet := r.nodeExporterDaemonSet(nodeExporter)

	// Check if the DaemonSet already exists
	found := &appsv1.DaemonSet{}
	err = r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// DaemonSet does not exist, create it
			logger.Info("Creating a new DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
			err = r.Create(ctx, daemonSet)
			if err != nil {
				logger.Error(err, "Failed to create new DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
				return ctrl.Result{}, err
			}
		} else {
			logger.Error(err, "Failed to get DaemonSet")
			return ctrl.Result{}, err
		}
	} else {
		// DaemonSet exists, update it
		logger.Info("Updating existing DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
		err = r.Update(ctx, daemonSet)
		if err != nil {
			logger.Error(err, "Failed to update DaemonSet", "DaemonSet.Namespace", daemonSet.Namespace, "DaemonSet.Name", daemonSet.Name)
			return ctrl.Result{}, err
		}
	}

	// Update status
	condition := metav1.Condition{
		Type:               "Available",
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

	return ctrl.Result{}, nil
}

// cleanupNodeExporter removes the DaemonSet if it exists
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

// SetupWithManager sets up the controller with the Manager.
func (r *NodeExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NodeExporter{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}
