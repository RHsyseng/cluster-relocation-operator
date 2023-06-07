package mirror

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ImageSetName = "mirror-ocp"

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if len(relocation.Spec.ImageDigestMirrors) == 0 {
		return Cleanup(client, ctx, logger)
	}

	clusterVersion := &configv1.ClusterVersion{}
	if err := client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return err
	}
	if semver.Compare(fmt.Sprintf("v%s", clusterVersion.Status.Desired.Version), "v4.13.0") == -1 {
		return createICSP(client, scheme, ctx, relocation, logger)
	} else {
		return createIDMS(client, scheme, ctx, relocation, logger)
	}
}

func Cleanup(client client.Client, ctx context.Context, logger logr.Logger) error {
	// if they move from relocation.Spec.ImageDigestMirrors=<something> to relocation.Spec.ImageDigestMirrors=<empty>, we need to delete the ICSP
	clusterVersion := &configv1.ClusterVersion{}
	if err := client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return err
	}
	if semver.Compare(fmt.Sprintf("v%s", clusterVersion.Status.Desired.Version), "v4.13.0") == -1 {
		return cleanupICSP(client, ctx, logger)
	} else {
		return cleanupIDMS(client, ctx, logger)
	}
}

// ImageContentSourcePolicy is deprecated since OCP 4.13
// This function converts the values in Spec.RepositoryDigestMirrors into an ImageContentSourcePolicy
// Used for OCP < 4.13
func createICSP(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	op, err := controllerutil.CreateOrUpdate(ctx, client, icsp, func() error {
		icsp.Spec.RepositoryDigestMirrors = []operatorv1alpha1.RepositoryDigestMirrors{}
		for _, v := range relocation.Spec.ImageDigestMirrors {
			mirrors := []string{}
			for _, w := range v.Mirrors {
				mirrors = append(mirrors, string(w))
			}
			item := operatorv1alpha1.RepositoryDigestMirrors{
				Source:  v.Source,
				Mirrors: mirrors,
			}
			icsp.Spec.RepositoryDigestMirrors = append(icsp.Spec.RepositoryDigestMirrors, item)
		}
		// Set the controller as the owner so that the ICSP is deleted along with the CR
		return controllerutil.SetControllerReference(relocation, icsp, scheme)
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Updated Image Content Sources", "ImageContentSourcePolicy", ImageSetName, "OperationResult", op)
	}
	return nil
}

func createIDMS(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	idms := &configv1.ImageDigestMirrorSet{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	op, err := controllerutil.CreateOrUpdate(ctx, client, idms, func() error {
		idms.Spec.ImageDigestMirrors = relocation.Spec.ImageDigestMirrors

		// Set the controller as the owner so that the IDMS is deleted along with the CR
		return controllerutil.SetControllerReference(relocation, idms, scheme)
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Updated Image Content Sources", "ImageDigestMirrorSet", ImageSetName, "OperationResult", op)
	}
	return nil
}

func cleanupICSP(client client.Client, ctx context.Context, logger logr.Logger) error {
	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	if err := client.Delete(ctx, icsp); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logger.Info("ICSP deleted", "ImageContentSourcePolicy", ImageSetName)
	}
	return nil
}

func cleanupIDMS(client client.Client, ctx context.Context, logger logr.Logger) error {
	idms := &configv1.ImageDigestMirrorSet{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	if err := client.Delete(ctx, idms); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logger.Info("IDMS deleted", "ImageDigestMirrorSet", ImageSetName)
	}
	return nil
}
