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

// Constants for exporter configuration
const (
	nodeExporterPort     = 9100
	blackboxExporterPort = 9115
	kubeStateMetricsPort = 8080
	metricsPath          = "/metrics"
	scrapeInterval       = "30s"
)

// createOrUpdateService creates or updates a Service for an exporter
func (r *NodeExporterReconciler) createOrUpdateService(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter, name string, port int32) error {
	labels := map[string]string{
		"app":        name,
		"controller": nodeExporter.Name,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-" + nodeExporter.Name,
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

	// Set controller reference
	if err := controllerutil.SetControllerReference(nodeExporter, svc, r.Scheme); err != nil {
		return err
	}

	// Check if Service exists
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, svc)
		}
		return err
	}

	// Update existing Service
	found.Spec = svc.Spec
	found.Labels = svc.Labels
	return r.Update(ctx, found)
}

// createOrUpdateServiceMonitor creates or updates a ServiceMonitor for an exporter
func (r *NodeExporterReconciler) createOrUpdateServiceMonitor(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter, name string) error {
	labels := map[string]string{
		"app":        name,
		"controller": nodeExporter.Name,
	}

	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-" + nodeExporter.Name,
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

	// Set controller reference
	if err := controllerutil.SetControllerReference(nodeExporter, sm, r.Scheme); err != nil {
		return err
	}

	// Check if ServiceMonitor exists
	found := &monitoringv1.ServiceMonitor{}
	err := r.Get(ctx, types.NamespacedName{Name: sm.Name, Namespace: sm.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, sm)
		}
		return err
	}

	// Update existing ServiceMonitor
	found.Spec = sm.Spec
	found.Labels = sm.Labels
	return r.Update(ctx, found)
}

// createServiceAccount creates a ServiceAccount if it doesn't exist
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

	// Set controller reference to enable garbage collection
	if err := controllerutil.SetControllerReference(nodeExporter, sa, r.Scheme); err != nil {
		return err
	}

	// Check if ServiceAccount exists
	found := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the ServiceAccount
			err = r.Create(ctx, sa)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// cleanupServiceAccount removes a ServiceAccount if it exists
func (r *NodeExporterReconciler) cleanupServiceAccount(ctx context.Context, name string, namespace string) error {
	sa := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sa)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.Delete(ctx, sa)
}

// createBlackboxConfigMap creates a ConfigMap for blackbox exporter if it doesn't exist
func (r *NodeExporterReconciler) createBlackboxConfigMap(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blackbox-exporter-config",
			Namespace: nodeExporter.Namespace,
			Labels: map[string]string{
				"app":        "blackbox-exporter",
				"controller": nodeExporter.Name,
			},
		},
		Data: map[string]string{
			"blackbox.yml": `modules:
  http_2xx:
    prober: http
    timeout: 5s
    http:
      preferred_ip_protocol: "ip4"
      valid_status_codes: [200]
  http_post_2xx:
    prober: http
    http:
      method: POST
  tcp_connect:
    prober: tcp
  icmp:
    prober: icmp
    timeout: 5s
    icmp:
      preferred_ip_protocol: "ip4"`,
		},
	}

	// Set controller reference to enable garbage collection
	if err := controllerutil.SetControllerReference(nodeExporter, cm, r.Scheme); err != nil {
		return err
	}

	// Check if ConfigMap exists
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the ConfigMap
			err = r.Create(ctx, cm)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// cleanupBlackboxConfigMap removes the blackbox exporter ConfigMap if it exists
func (r *NodeExporterReconciler) cleanupBlackboxConfigMap(ctx context.Context, namespace string) error {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: "blackbox-exporter-config", Namespace: namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.Delete(ctx, cm)
}

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
		// Node exporter is disabled, ensure the DaemonSet and ServiceAccount are removed
		if err := r.cleanupNodeExporter(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup NodeExporter")
			return err
		}
		if err := r.cleanupServiceAccount(ctx, "node-exporter", nodeExporter.Namespace); err != nil {
			logger.Error(err, "Failed to cleanup NodeExporter ServiceAccount")
			return err
		}
		return nil
	}

	// Create ServiceAccount first
	if err := r.createServiceAccount(ctx, "node-exporter", nodeExporter.Namespace, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ServiceAccount for NodeExporter")
		return err
	}

	// Create or update the Service
	if err := r.createOrUpdateService(ctx, nodeExporter, "node-exporter", nodeExporterPort); err != nil {
		logger.Error(err, "Failed to create/update Service for NodeExporter")
		return err
	}

	// Create or update the ServiceMonitor
	if err := r.createOrUpdateServiceMonitor(ctx, nodeExporter, "node-exporter"); err != nil {
		logger.Error(err, "Failed to create/update ServiceMonitor for NodeExporter")
		return err
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
		// Blackbox exporter is disabled, ensure the DaemonSet and ServiceAccount are removed
		if err := r.cleanupBlackboxExporter(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup BlackboxExporter")
			return err
		}
		if err := r.cleanupServiceAccount(ctx, "blackbox-exporter", nodeExporter.Namespace); err != nil {
			logger.Error(err, "Failed to cleanup BlackboxExporter ServiceAccount")
			return err
		}
		if err := r.cleanupBlackboxConfigMap(ctx, nodeExporter.Namespace); err != nil {
			logger.Error(err, "Failed to cleanup BlackboxExporter ConfigMap")
			return err
		}
		return nil
	}

	// Create ConfigMap first
	if err := r.createBlackboxConfigMap(ctx, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ConfigMap for BlackboxExporter")
		return err
	}

	// Create ServiceAccount
	if err := r.createServiceAccount(ctx, "blackbox-exporter", nodeExporter.Namespace, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ServiceAccount for BlackboxExporter")
		return err
	}

	// Create or update the Service
	if err := r.createOrUpdateService(ctx, nodeExporter, "blackbox-exporter", blackboxExporterPort); err != nil {
		logger.Error(err, "Failed to create/update Service for BlackboxExporter")
		return err
	}

	// Create or update the ServiceMonitor
	if err := r.createOrUpdateServiceMonitor(ctx, nodeExporter, "blackbox-exporter"); err != nil {
		logger.Error(err, "Failed to create/update ServiceMonitor for BlackboxExporter")
		return err
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

// cleanupNodeExporter removes the Node Exporter resources if they exist
func (r *NodeExporterReconciler) cleanupNodeExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	name := "node-exporter-" + nodeExporter.Name
	ns := nodeExporter.Namespace

	// Delete DaemonSet
	if err := r.cleanupResource(ctx, &appsv1.DaemonSet{}, name, ns); err != nil {
		return err
	}

	// Delete Service
	if err := r.cleanupResource(ctx, &corev1.Service{}, name, ns); err != nil {
		return err
	}

	// Delete ServiceMonitor
	if err := r.cleanupResource(ctx, &monitoringv1.ServiceMonitor{}, name, ns); err != nil {
		return err
	}

	return nil
}

// cleanupResource is a generic function to delete a Kubernetes resource
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

// cleanupBlackboxExporter removes the Blackbox Exporter resources if they exist
func (r *NodeExporterReconciler) cleanupBlackboxExporter(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	name := "blackbox-exporter-" + nodeExporter.Name
	ns := nodeExporter.Namespace

	// Delete DaemonSet
	if err := r.cleanupResource(ctx, &appsv1.DaemonSet{}, name, ns); err != nil {
		return err
	}

	// Delete Service
	if err := r.cleanupResource(ctx, &corev1.Service{}, name, ns); err != nil {
		return err
	}

	// Delete ServiceMonitor
	if err := r.cleanupResource(ctx, &monitoringv1.ServiceMonitor{}, name, ns); err != nil {
		return err
	}

	return nil
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
		// Kube State Metrics is disabled, ensure the DaemonSet and ServiceAccount are removed
		if err := r.cleanupKubeStateMetrics(ctx, nodeExporter); err != nil {
			logger.Error(err, "Failed to cleanup KubeStateMetrics")
			return err
		}
		if err := r.cleanupServiceAccount(ctx, "kube-state-metrics", nodeExporter.Namespace); err != nil {
			logger.Error(err, "Failed to cleanup KubeStateMetrics ServiceAccount")
			return err
		}
		return nil
	}

	// Create ServiceAccount first
	if err := r.createServiceAccount(ctx, "kube-state-metrics", nodeExporter.Namespace, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ServiceAccount for KubeStateMetrics")
		return err
	}

	// Create or update the Service
	if err := r.createOrUpdateService(ctx, nodeExporter, "kube-state-metrics", kubeStateMetricsPort); err != nil {
		logger.Error(err, "Failed to create/update Service for KubeStateMetrics")
		return err
	}

	// Create or update the ServiceMonitor
	if err := r.createOrUpdateServiceMonitor(ctx, nodeExporter, "kube-state-metrics"); err != nil {
		logger.Error(err, "Failed to create/update ServiceMonitor for KubeStateMetrics")
		return err
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

// cleanupKubeStateMetrics removes the Kube State Metrics resources if they exist
func (r *NodeExporterReconciler) cleanupKubeStateMetrics(ctx context.Context, nodeExporter *cachev1alpha1.NodeExporter) error {
	name := "kube-state-metrics-" + nodeExporter.Name
	ns := nodeExporter.Namespace

	// Delete DaemonSet
	if err := r.cleanupResource(ctx, &appsv1.DaemonSet{}, name, ns); err != nil {
		return err
	}

	// Delete Service
	if err := r.cleanupResource(ctx, &corev1.Service{}, name, ns); err != nil {
		return err
	}

	// Delete ServiceMonitor
	if err := r.cleanupResource(ctx, &monitoringv1.ServiceMonitor{}, name, ns); err != nil {
		return err
	}

	return nil
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
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Complete(r)
}
