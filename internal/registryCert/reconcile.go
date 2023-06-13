package registrycert

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups="",resources=configmaps,verbs=create;update;get
//+kubebuilder:rbac:groups=config.openshift.io,resources=images,verbs=patch;get

const ConfigMapName = "generated-registry-cert"

func Reconcile(c client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if relocation.Spec.RegistryCert == nil {
		return Cleanup(c, ctx, logger)
	}

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: rhsysenggithubiov1beta1.ConfigNamespace}}
	op, err := controllerutil.CreateOrUpdate(ctx, c, configMap, func() error {
		var port string
		if relocation.Spec.RegistryCert.RegistryPort != nil {
			port = fmt.Sprintf("..%s", *relocation.Spec.RegistryCert.RegistryPort)
		}
		configMap.Data = map[string]string{
			fmt.Sprintf("%s%s", relocation.Spec.RegistryCert.RegistryHostname, port): relocation.Spec.RegistryCert.Certificate,
		}
		// Set the controller as the owner so that the ConfigMap is deleted along with the CR
		return controllerutil.SetControllerReference(relocation, configMap, scheme)
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Registry certificate modified", "OperationResult", op)
	}

	imageConfig := &configv1.Image{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	op, err = controllerutil.CreateOrPatch(ctx, c, imageConfig, func() error {
		imageConfig.Spec.AdditionalTrustedCA = configv1.ConfigMapNameReference{Name: ConfigMapName}
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("AdditionalTrustedCA modified", "OperationResult", op)
	}
	return nil
}

func Cleanup(c client.Client, ctx context.Context, logger logr.Logger) error {
	// if they move from relocation.Spec.RegistryCert.Certificate=<something> to relocation.Spec.RegistryCert.Certificate=<empty>
	// we need to clear out the AdditionalTrustedCA
	imageConfig := &configv1.Image{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	op, err := controllerutil.CreateOrPatch(ctx, c, imageConfig, func() error {
		imageConfig.Spec.AdditionalTrustedCA = configv1.ConfigMapNameReference{}
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("AdditionalTrustedCA reverted to original state", "OperationResult", op)
	}
	return nil
}
