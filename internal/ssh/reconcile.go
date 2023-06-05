package ssh

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	machineconfigurationv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if len(relocation.Spec.SSHKeys) == 0 {
		// if they don't specify new ssh keys, nothing to do
		return nil
	}

	for _, v := range []string{"master", "worker"} {
		machineConfig := &machineconfigurationv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("core-ssh-key-%s", v)}}
		op, err := controllerutil.CreateOrUpdate(ctx, client, machineConfig, func() error {
			machineConfig.Labels["machineconfiguration.openshift.io/role"] = v
			// TODO: fill in spec
			// Set the controller as the owner so that the secret is deleted along with the CR
			err := controllerutil.SetControllerReference(relocation, machineConfig, scheme)
			return err
		})
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("Updated SSH keys for core user", "MachineConfigPool", v, "OperationResult", op)
		}
	}

	return nil
}
