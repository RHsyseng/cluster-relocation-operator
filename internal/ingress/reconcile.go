package ingress

import (
	"context"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {

	// Define the IngressController's namespace and name
	namespace := "openshift-ingress-operator"
	name := "default"
	certName := "ingress-cert"

	err := operatorv1.Install(scheme) // Add operator.openshift.io/v1 to the scheme
	if err != nil {
		logger.Error(err, "Failed to install operator.openshift.io/v1")
		return err
	}

	ingress := &operatorv1.IngressController{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       operatorv1.IngressControllerSpec{},
		Status:     operatorv1.IngressControllerStatus{},
	}
	op, err := controllerutil.CreateOrPatch(ctx, client, ingress, func() error {
		ingress.Spec.DefaultCertificate = &corev1.LocalObjectReference{Name: certName}
		return nil
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		logger.Info("APIServer modified", "OperationResult", op)
	}

	return nil
}
