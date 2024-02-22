/*
Copyright 2024 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package controller

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/deckhouse/deckhouse/api/v1alpha1"
	"github.com/deckhouse/deckhouse/internal/scopes"
)

// ZvirtClusterReconciler reconciles a ZvirtCluster object
type ZvirtClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *rest.Config
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ZvirtClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling StaticCluster")

	zvirtCluster := &infrastructurev1alpha1.ZvirtCluster{}
	err := r.Get(ctx, req.NamespacedName, zvirtCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, zvirtCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")

		return ctrl.Result{}, nil
	}

	newScope, err := scopes.NewScope(r.Client, r.Config, ctrl.LoggerFrom(ctx))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to create a scope: %w", err)
	}

	clusterScope, err := scopes.NewClusterScope(newScope, cluster, zvirtCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to create a cluster scope: %w", err)
	}

	// Handle deleted cluster
	if !zvirtCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, clusterScope)
}

func (r *ZvirtClusterReconciler) reconcile(
	ctx context.Context,
	clusterScope *scopes.ClusterScope,
) (ctrl.Result, error) {
	controlPlaneEndpointURL, err := url.Parse(r.Config.Host)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to parse api server host: %w", err)
	}

	port, err := strconv.Atoi(controlPlaneEndpointURL.Port())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to parse api server port: %w", err)
	}

	clusterScope.ZvirtCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: controlPlaneEndpointURL.Hostname(),
		Port: int32(port),
	}

	clusterScope.ZvirtCluster.Status.Ready = true

	err = clusterScope.Patch(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to patch StaticCluster: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZvirtClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ZvirtCluster{}).
		Complete(r)
}
