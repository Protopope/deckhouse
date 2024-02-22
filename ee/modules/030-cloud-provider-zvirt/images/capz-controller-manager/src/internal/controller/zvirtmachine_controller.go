/*
Copyright 2024 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package controller

import (
	"context"
	"fmt"
	"slices"
	"time"

	ovirt "github.com/ovirt/go-ovirt-client"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zv1alpha1 "github.com/deckhouse/deckhouse/api/v1alpha1"
	"github.com/deckhouse/deckhouse/internal/controller/utils"
)

const ProviderIDPrefix = "zvirt://"

const (
	FailureReasonSpecValidation = "InvalidSpec"
	FailureReasonTemplateError  = "TemplateError"
	FailureReasonVMError        = "VirtualMachineError"
)

// ZvirtMachineReconciler reconciles a ZvirtMachine's
type ZvirtMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Zvirt  ovirt.Client
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=zvirtmachines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ZvirtMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	zvMachine := &zv1alpha1.ZvirtMachine{}
	err := r.Client.Get(ctx, req.NamespacedName, zvMachine)
	switch {
	case apierrors.IsNotFound(err):
		return ctrl.Result{}, nil
	case err != nil:
		return ctrl.Result{}, fmt.Errorf("Error getting ZvirtMachine: %w", err)
	}

	if zvMachine.Status.FailureReason != nil || zvMachine.Status.FailureMessage != nil {
		return ctrl.Result{}, nil // Skip failed instances.
	}

	machine, err := capiutil.GetOwnerMachine(ctx, r.Client, zvMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("capiutil.GetOwnerMachine: %w", err)
	}
	if machine == nil {
		logger.Info("Waiting for Machine Controller to set owner reference on ZvirtMachine")
		return ctrl.Result{}, nil
	}
	if machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Waiting for bootstrap data script to be provided to Machine")
		return ctrl.Result{}, nil
	}

	cluster, err := capiutil.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("ZvirtMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, fmt.Errorf("capiutil.GetClusterFromMetadata: %w", err)
	}
	if cluster == nil {
		logger.Info("Please associate this machine with a cluster using the label", "label", clusterv1.ClusterNameLabel)
		return ctrl.Result{}, nil
	}

	machineIsDeleted := !zvMachine.DeletionTimestamp.IsZero()
	if !cluster.Status.InfrastructureReady && !machineIsDeleted {
		return ctrl.Result{}, nil
	}

	switch {
	case machineIsDeleted:
		return r.reconcileDeletedMachine(ctx, zvMachine)
	case zvMachine.Spec.ProviderID == "":
		return r.reconcileNewMachine(ctx, zvMachine, machine)
	default:
		return r.reconcileNormalOperation(ctx, zvMachine)
	}
}

func (r *ZvirtMachineReconciler) reconcileNormalOperation(
	ctx context.Context,
	zvMachine *zv1alpha1.ZvirtMachine,
) (ctrl.Result, error) {
	// TODO(mvasl) consider node lookup by machine owner reference?
	node := &corev1.Node{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: zvMachine.Name}, node)
	switch {
	case apierrors.IsNotFound(err):
		return ctrl.Result{}, nil // Node was deleted
	case err != nil:
		return ctrl.Result{}, fmt.Errorf("Error getting ZvirtMachine: %w", err)
	}

	logger := log.FromContext(ctx)
	vm, err := r.Zvirt.GetVMByName(node.Name)
	if err != nil {
		if ovirt.HasErrorCode(err, ovirt.ENotFound) {
			logger.Info("Deleting Node %q from cluster, it's VM was removed from the zVirt engine", node.Name)
			if err := r.Client.Delete(ctx, node); err != nil {
				return ctrl.Result{}, fmt.Errorf("Got error during node %q deletion: %w", node.Name, err)
			}
		}
		return ctrl.Result{}, fmt.Errorf("Failed to get %q VM from zVirt: %w", node.Name, err)
	} else if vm.Status() == ovirt.VMStatusDown {
		logger.Info("Node %q VM status is Down, will try reconciling in 60 seconds", node.Name)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

const (
	vNICName        = "nic1"
	expectedNICName = "eth0"
)

func (r *ZvirtMachineReconciler) reconcileNewMachine(
	ctx context.Context,
	zvirtMachine *zv1alpha1.ZvirtMachine,
	machine *clusterv1.Machine,
) (ctrl.Result, error) {
	if err := r.validateZvirtMachineSpec(&zvirtMachine.Spec); err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonSpecValidation, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	timeout := ovirt.ContextStrategy(timeoutCtx)

	tpl, err := r.Zvirt.GetTemplateByName(zvirtMachine.Spec.TemplateName, timeout)
	if err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonTemplateError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Cannot get VM template %q: %w", zvirtMachine.Spec.TemplateName, err)
	}

	dataSecretName := *machine.Spec.Bootstrap.DataSecretName
	bootstrapDataSecret := &corev1.Secret{}
	if err = r.Client.Get(
		ctx,
		client.ObjectKey{
			Namespace: machine.GetNamespace(),
			Name:      dataSecretName,
		},
		bootstrapDataSecret,
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("Cannot get cloud-init data secret: %w", err)
	}

	cloudInitScript, hasBootstrapScript := bootstrapDataSecret.Data["value"]
	if !hasBootstrapScript {
		return ctrl.Result{}, fmt.Errorf("Expected to find a cloud-init script in secret %s/%s", bootstrapDataSecret.Namespace, bootstrapDataSecret.Name)
	}

	vmConfig, err := r.vmConfigFromZvirtMachineSpec(zvirtMachine.Name, &zvirtMachine.Spec, cloudInitScript)
	if err != nil {
		return ctrl.Result{}, err
	}

	vm, err := r.Zvirt.CreateVM(ovirt.ClusterID(zvirtMachine.Spec.ClusterID), tpl.ID(), zvirtMachine.Name, vmConfig, timeout)
	if err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonVMError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Cannot create VM: %w", err)
	}

	if vm, err = r.Zvirt.WaitForVMStatus(vm.ID(), ovirt.VMStatusDown, timeout); err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonVMError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Tired of waiting for VM to be created: %w", err)
	}

	_, err = r.Zvirt.CreateNIC(vm.ID(), ovirt.VNICProfileID(zvirtMachine.Spec.VNICProfileID), vNICName, nil, timeout)
	if err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonVMError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Attach vNIC to the VM: %w", err)
	}

	if err = vm.Start(timeout); err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonVMError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Cannot start VM: %w", err)
	}

	addrs, err := vm.WaitForNonLocalIPAddress(timeout)
	if err != nil {
		if err := r.tryPatchingFailureReasonIntoZvirtMachineStatus(ctx, zvirtMachine, FailureReasonVMError, err); err != nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Tired of waiting for VM to get IP address: %w", err)
	}

	machineAddress := ""
	nicAddrs, hasAddr := addrs[expectedNICName]
	if !hasAddr {
		return ctrl.Result{}, fmt.Errorf("Expected vNIC %q to be attached to VM %q and configured non-loopback IP address", vNICName, zvirtMachine.Name)
	}

	for _, ip := range nicAddrs {
		if !ip.IsLoopback() {
			machineAddress = ip.String()
		}
	}

	if machineAddress == "" {
		return ctrl.Result{}, fmt.Errorf("Expected vNIC %q to be attached to VM %q and configured non-loopback IP address", vNICName, zvirtMachine.Name)
	}

	patched := *zvirtMachine
	patched.Spec.ProviderID = string(ProviderIDPrefix + vm.ID())
	patched.Status.Ready = true
	patched.Status.Addresses = []zv1alpha1.VMAddress{
		{
			Type:    clusterv1.MachineHostName,
			Address: zvirtMachine.Name,
		},
		{
			Type:    clusterv1.MachineInternalIP,
			Address: machineAddress,
		},
		// {
		// 	Type:    clusterv1.MachineExternalIP,
		// 	Address: machineAddress,
		// },
	}
	if err = r.Client.Patch(ctx, &patched, client.MergeFrom(zvirtMachine)); err != nil {
		return ctrl.Result{}, fmt.Errorf("Cannot patch ZvirtMachine object with provider ID: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ZvirtMachineReconciler) reconcileDeletedMachine(
	ctx context.Context,
	zvirtMachine *zv1alpha1.ZvirtMachine,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vm, err := r.Zvirt.GetVMByName(zvirtMachine.Name)
	if err != nil {
		if ovirt.HasErrorCode(err, ovirt.ENotFound) {
			logger.Info("No VM with name %q was not found during deletion of corresponding ZvirtMachine, nothing to do", zvirtMachine.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Failed to get %q VM from zVirt: %w", zvirtMachine.Name, err)
	}

	vmID := vm.ID()
	if err = r.Zvirt.ShutdownVM(vmID, false); err != nil {
		return ctrl.Result{}, fmt.Errorf("Cannot shutdown VM: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	timeout := ovirt.ContextStrategy(timeoutCtx)
	vm, err = r.Zvirt.WaitForVMStatus(vmID, ovirt.VMStatusDown, timeout)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Waiting for VM to shutdown: %w", err)
	}

	if err = r.Zvirt.RemoveVM(vmID, timeout); err != nil {
		return ctrl.Result{}, fmt.Errorf("Delete stopped VM: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ZvirtMachineReconciler) vmConfigFromZvirtMachineSpec(
	hostname string,
	machineSpec *zv1alpha1.ZvirtMachineSpec,
	cloudInitScript []byte,
) (ovirt.BuildableVMParameters, error) {
	ramBytes := int64(machineSpec.Memory) * 1024 * 1024
	if machineSpec.Memory == 0 {
		ramBytes = 8589934592 // 8 GB
	}

	cpuSpec := machineSpec.CPU
	if cpuSpec == nil {
		cpuSpec = &zv1alpha1.CPU{Sockets: 4, Cores: 1, Threads: 1}
	}

	vmType := ovirt.VMTypeHighPerformance
	if machineSpec.VMType != "" {
		vmType = ovirt.VMType(machineSpec.VMType)
	}

	vmConfig := ovirt.NewCreateVMParams()
	vmConfig = vmConfig.MustWithClone(true).
		MustWithCPU(
			ovirt.NewVMCPUParams().
				MustWithTopo(
					ovirt.MustNewVMCPUTopo(uint(cpuSpec.Cores), uint(cpuSpec.Threads), uint(cpuSpec.Sockets)),
				),
		)
	vmConfig = vmConfig.MustWithMemory(ramBytes)
	vmConfig = vmConfig.MustWithVMType(vmType)
	vmConfig = vmConfig.WithMemoryPolicy(
		ovirt.NewMemoryPolicyParameters().
			MustWithBallooning(false).
			MustWithMax(ramBytes).
			MustWithGuaranteed(ramBytes),
	)

	encodedCloudInit, err := utils.XMLEncode(cloudInitScript)
	if err != nil {
		return nil, fmt.Errorf("Cannot prepare cloud-init script for VM: %w", err)
	}

	vmConfig, err = vmConfig.WithInitialization(ovirt.NewInitialization(encodedCloudInit, hostname))
	if err != nil {
		return nil, fmt.Errorf("Cannot prepare cloud-init script for VM: %w", err)
	}

	return vmConfig, nil
}

func (r *ZvirtMachineReconciler) validateZvirtMachineSpec(spec *zv1alpha1.ZvirtMachineSpec) error {
	validations := map[string]bool{
		"no .spec.template_name provided":   spec.TemplateName == "",
		"no .spec.cluster_id provided":      spec.ClusterID == "",
		"no .spec.vnic_profile_id provided": spec.VNICProfileID == "",

		".spec.vm_type, is not one of 'high_performance', 'server' or 'desktop'": !slices.Contains(
			[]string{"", string(ovirt.VMTypeHighPerformance), string(ovirt.VMTypeServer), string(ovirt.VMTypeDesktop)},
			string(spec.VMType),
		),
	}

	for message, failed := range validations {
		if failed {
			return fmt.Errorf("ZvirtMachine spec validation failed: %s", message)
		}
	}

	return nil
}

func (r *ZvirtMachineReconciler) tryPatchingFailureReasonIntoZvirtMachineStatus(
	ctx context.Context,
	zm *zv1alpha1.ZvirtMachine,
	errorReason string,
	errorDescription error,
) error {
	if errorDescription == nil || errorReason == "" {
		return nil
	}

	errorMessage := errorDescription.Error()
	patched := *zm
	patched.Status.Ready = false
	patched.Status.FailureReason = &errorReason
	patched.Status.FailureMessage = &errorMessage

	if err := r.Patch(ctx, &patched, client.MergeFrom(zm)); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZvirtMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zv1alpha1.ZvirtMachine{}).
		Complete(r)
}
