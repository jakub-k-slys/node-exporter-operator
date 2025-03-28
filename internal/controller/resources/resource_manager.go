package resources

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	exporterv1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
	exportertypes "github.com/jakub-k-slys/node-exporter-operator/internal/controller/types"
)

// ResourceManager handles Kubernetes resource operations
type ResourceManager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewResourceManager creates a new ResourceManager instance
func NewResourceManager(client client.Client, scheme *runtime.Scheme) *ResourceManager {
	return &ResourceManager{
		client: client,
		scheme: scheme,
	}
}

// CreateOrUpdateDaemonSet creates or updates a DaemonSet for an exporter
func (r *ResourceManager) CreateOrUpdateDaemonSet(ctx context.Context, cfg exportertypes.ExporterConfig, nodeExporter *exporterv1alpha1.NodeExporter) error {
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

	if err := controllerutil.SetControllerReference(nodeExporter, daemonSet, r.scheme); err != nil {
		return err
	}

	found := &appsv1.DaemonSet{}
	err := r.client.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.client.Create(ctx, daemonSet)
		}
		return err
	}

	found.Spec = daemonSet.Spec
	found.Labels = daemonSet.Labels
	return r.client.Update(ctx, found)
}

// CreateServiceAccount creates a ServiceAccount if it doesn't exist
func (r *ResourceManager) CreateServiceAccount(ctx context.Context, name string, namespace string, nodeExporter *exporterv1alpha1.NodeExporter) error {
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

	if err := controllerutil.SetControllerReference(nodeExporter, sa, r.scheme); err != nil {
		return err
	}

	found := &corev1.ServiceAccount{}
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.client.Create(ctx, sa)
		}
		return err
	}

	return nil
}

// CreateOrUpdateService creates or updates a Service for an exporter
func (r *ResourceManager) CreateOrUpdateService(ctx context.Context, nodeExporter *exporterv1alpha1.NodeExporter, name string, port int32) error {
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

	if err := controllerutil.SetControllerReference(nodeExporter, svc, r.scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.client.Create(ctx, svc)
		}
		return err
	}

	found.Spec = svc.Spec
	found.Labels = svc.Labels
	return r.client.Update(ctx, found)
}

// CreateOrUpdateServiceMonitor creates or updates a ServiceMonitor for an exporter
func (r *ResourceManager) CreateOrUpdateServiceMonitor(ctx context.Context, nodeExporter *exporterv1alpha1.NodeExporter, name string) error {
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
				Path:     exportertypes.MetricsPath,
				Interval: exportertypes.ScrapeInterval,
			}},
		},
	}

	if err := controllerutil.SetControllerReference(nodeExporter, sm, r.scheme); err != nil {
		return err
	}

	found := &monitoringv1.ServiceMonitor{}
	err := r.client.Get(ctx, types.NamespacedName{Name: sm.Name, Namespace: sm.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.client.Create(ctx, sm)
		}
		return err
	}

	found.Spec = sm.Spec
	found.Labels = sm.Labels
	return r.client.Update(ctx, found)
}

// CleanupResource removes a Kubernetes resource if it exists
func (r *ResourceManager) CleanupResource(ctx context.Context, obj client.Object, name, namespace string) error {
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.client.Delete(ctx, obj)
}
