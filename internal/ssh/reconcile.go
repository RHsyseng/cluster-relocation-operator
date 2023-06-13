package ssh

import (
	"context"
	"encoding/json"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	machineconfigurationv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type MachineConfigUsersData struct {
	Name              string   `json:"name"`
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys"`
}

type MachineConfigPasswdData struct {
	Users []MachineConfigUsersData `json:"users"`
}

type MachineConfigData struct {
	Ignition map[string]string       `json:"ignition"`
	Passwd   MachineConfigPasswdData `json:"passwd"`
}

//+kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=create;update;get;delete

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if relocation.Spec.SSHKeys == nil {
		return Cleanup(ctx, c, logger)
	}

	for _, v := range []string{"master", "worker"} {
		machineConfig := &machineconfigurationv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("core-ssh-key-%s", v)}}
		op, err := controllerutil.CreateOrUpdate(ctx, c, machineConfig, func() error {
			machineConfig.Labels = map[string]string{"machineconfiguration.openshift.io/role": v}
			configData := MachineConfigData{
				Ignition: map[string]string{"version": "3.2.0"},
				Passwd: MachineConfigPasswdData{
					Users: []MachineConfigUsersData{
						{
							Name:              "core",
							SSHAuthorizedKeys: *relocation.Spec.SSHKeys,
						},
					},
				},
			}
			bytes, err := json.Marshal(configData)
			if err != nil {
				return err
			}
			machineConfig.Spec.Config.Raw = bytes
			// Set the controller as the owner so that the MachineConfig is deleted along with the CR
			return controllerutil.SetControllerReference(relocation, machineConfig, scheme)
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

func Cleanup(ctx context.Context, c client.Client, logger logr.Logger) error {
	// if they move from relocation.Spec.SSHKeys=<something> to relocation.Spec.SSHKeys=<empty>, we need to delete the MachineConfigs
	for _, v := range []string{"master", "worker"} {
		machineConfig := &machineconfigurationv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("core-ssh-key-%s", v)}}
		if err := c.Delete(ctx, machineConfig); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			logger.Info("SSH key MachineConfig deleted", "MachineConfig", machineConfig.ObjectMeta.Name)
		}
	}
	return nil
}
