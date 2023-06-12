package dns

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	machineconfigurationv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type MachineConfigFilesData struct {
	Contents  map[string]string `json:"contents"`
	Mode      int               `json:"mode"`
	Overwrite bool              `json:"overwrite"`
	Path      string            `json:"path"`
	User      map[string]string `json:"user"`
}

type MachineConfigStorageData struct {
	Files []MachineConfigFilesData `json:"files"`
}

type MachineConfigData struct {
	Ignition map[string]string        `json:"ignition"`
	Storage  MachineConfigStorageData `json:"storage"`
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=list
//+kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=create;update;get

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		return err
	}
	if len(nodes.Items) > 1 {
		// DNS reconfiguration is only supported on SNO
		// Relocation will still work on MNO, but external DNS records need to be in place for the new domain
		logger.Info("DNS reconfiguration not supported on multi-node clusters. Ensure that external DNS records exist for the new domain")
		return nil
	}
	internalIP, err := getInternalIP(nodes.Items[0])
	if err != nil {
		return err
	}

	machineConfig := &machineconfigurationv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "relocation-dns-master"}}
	op, err := controllerutil.CreateOrUpdate(ctx, c, machineConfig, func() error {
		snoDNSContents := fmt.Sprintf("address=/apps.%s/%s\n"+
			"address=/api-int.%s/%s\n"+
			"address=/api.%s/%s\n",
			relocation.Spec.Domain, internalIP, relocation.Spec.Domain, internalIP, relocation.Spec.Domain, internalIP)

		machineConfig.Labels = map[string]string{"machineconfiguration.openshift.io/role": "master"}
		configData := MachineConfigData{
			Ignition: map[string]string{"version": "3.2.0"},
			Storage: MachineConfigStorageData{
				Files: []MachineConfigFilesData{
					{
						Contents: map[string]string{
							"source": fmt.Sprintf("data:text/plain;charset=utf-8;base64,%s", base64.StdEncoding.EncodeToString([]byte(snoDNSContents))),
						},
						Mode:      0o644,
						Overwrite: true,
						Path:      "/etc/dnsmasq.d/relocation-domain.conf",
						User: map[string]string{
							"name": "root",
						},
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
		logger.Info("Updated DNS settings", "OperationResult", op)
	}
	return nil
}

func getInternalIP(node corev1.Node) (string, error) {
	for _, v := range node.Status.Addresses {
		if v.Type == corev1.NodeInternalIP {
			return v.Address, nil
		}
	}
	return "", fmt.Errorf("could not find node IP address")
}
