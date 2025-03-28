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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	exporterv1alpha1 "github.com/jakub-k-slys/node-exporter-operator/api/v1alpha1"
	"github.com/jakub-k-slys/node-exporter-operator/internal/controller/exporters"
	"github.com/jakub-k-slys/node-exporter-operator/internal/controller/resources"
	exportertypes "github.com/jakub-k-slys/node-exporter-operator/internal/controller/types"
)

// NodeExporterReconciler reconciles a NodeExporter object
type NodeExporterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=exporter.slys.dev,resources=nodeexporters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=exporter.slys.dev,resources=nodeexporters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=exporter.slys.dev,resources=nodeexporters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the reconciliation loop for NodeExporter resources
func (r *NodeExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NodeExporter")

	// Initialize resource manager
	resourceManager := resources.NewResourceManager(r.Client, r.Scheme)

	// Fetch the NodeExporter instance
	nodeExporter := &exporterv1alpha1.NodeExporter{}
	if err := r.Get(ctx, req.NamespacedName, nodeExporter); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NodeExporter resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NodeExporter")
		return ctrl.Result{}, err
	}

	// Reconcile each exporter
	if err := r.reconcileExporter(ctx, resourceManager, exporters.GetNodeExporterConfig(nodeExporter), nodeExporter.Spec.NodeExporterSettings.Enabled); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileExporter(ctx, resourceManager, exporters.GetBlackboxExporterConfig(nodeExporter), nodeExporter.Spec.BlackboxExporterSettings.Enabled); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileExporter(ctx, resourceManager, exporters.GetKubeStateMetricsConfig(nodeExporter), nodeExporter.Spec.KubeStateMetricsSettings.Enabled); err != nil {
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
func (r *NodeExporterReconciler) reconcileExporter(ctx context.Context, rm *resources.ResourceManager, cfg exportertypes.ExporterConfig, enabled bool) error {
	logger := log.FromContext(ctx)

	if !enabled {
		return r.cleanupExporter(ctx, rm, cfg)
	}

	// Create or update resources
	if err := r.reconcileResources(ctx, rm, cfg); err != nil {
		logger.Error(err, "Failed to reconcile resources", "exporter", cfg.Name)
		return err
	}

	return nil
}

// reconcileResources creates or updates all resources for an exporter
func (r *NodeExporterReconciler) reconcileResources(ctx context.Context, rm *resources.ResourceManager, cfg exportertypes.ExporterConfig) error {
	logger := log.FromContext(ctx)

	// Get NodeExporter instance for owner reference
	nodeExporter := &exporterv1alpha1.NodeExporter{}
	if err := r.Get(ctx, types.NamespacedName{Name: cfg.Name, Namespace: "default"}, nodeExporter); err != nil {
		return err
	}

	// Create ServiceAccount
	if err := rm.CreateServiceAccount(ctx, cfg.Name, nodeExporter.Namespace, nodeExporter); err != nil {
		logger.Error(err, "Failed to create ServiceAccount", "exporter", cfg.Name)
		return err
	}

	// Create or update Service
	if err := rm.CreateOrUpdateService(ctx, nodeExporter, cfg.Name, cfg.Port); err != nil {
		logger.Error(err, "Failed to create/update Service", "exporter", cfg.Name)
		return err
	}

	// Create or update ServiceMonitor
	if err := rm.CreateOrUpdateServiceMonitor(ctx, nodeExporter, cfg.Name); err != nil {
		logger.Error(err, "Failed to create/update ServiceMonitor", "exporter", cfg.Name)
		return err
	}

	// Create or update DaemonSet
	if err := rm.CreateOrUpdateDaemonSet(ctx, cfg, nodeExporter); err != nil {
		logger.Error(err, "Failed to create/update DaemonSet", "exporter", cfg.Name)
		return err
	}

	return nil
}

// cleanupExporter removes all resources for an exporter
func (r *NodeExporterReconciler) cleanupExporter(ctx context.Context, rm *resources.ResourceManager, cfg exportertypes.ExporterConfig) error {
	nodeExporter := &exporterv1alpha1.NodeExporter{}
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
		if err := rm.CleanupResource(ctx, obj, name, ns); err != nil {
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&exporterv1alpha1.NodeExporter{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Complete(r)
}
