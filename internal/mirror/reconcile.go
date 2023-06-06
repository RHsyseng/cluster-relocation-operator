package mirror

import (
	"context"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const IcspName = "mirror-ocp"

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if len(relocation.Spec.ImageDigestSources) == 0 {
		return Cleanup(client, ctx, logger)
	}

	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: IcspName}}
	op, err := controllerutil.CreateOrUpdate(ctx, client, icsp, func() error {
		icsp.Spec.RepositoryDigestMirrors = relocation.Spec.ImageDigestSources
		// Set the controller as the owner so that the ICSP is deleted along with the CR
		return controllerutil.SetControllerReference(relocation, icsp, scheme)
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Updated Image Content Sources", "ImageContentSourcePolicy", IcspName, "OperationResult", op)
	}
	return nil
}

func Cleanup(client client.Client, ctx context.Context, logger logr.Logger) error {
	// if they move from relocation.Spec.ImageDigestSources=<something> to relocation.Spec.ImageDigestSources=<empty>, we need to delete the ICSP
	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: IcspName}}
	if err := client.Delete(ctx, icsp); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logger.Info("ICSP deleted", "ImageContentSourcePolicy", IcspName)
	}
	return nil
}
