package mirror

import (
	"context"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups=operator.openshift.io,resources=imagecontentsourcepolicies,verbs=create;update;get;delete
//+kubebuilder:rbac:groups=config.openshift.io,resources=imagedigestmirrorsets,verbs=create;update;get;delete

const ImageSetName = "mirror-ocp"

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger, clusterVersion string) error {
	if relocation.Spec.ImageDigestMirrors == nil {
		return Cleanup(ctx, c, logger, clusterVersion)
	}

	if semver.Compare(clusterVersion, "v4.13.0") == -1 {
		return createICSP(ctx, c, scheme, relocation, logger)
	}

	// In case we are upgrading from 4.12 to 4.13+, remove any old ImageContentSourcePolicy
	if err := cleanupICSP(ctx, c, logger); err != nil {
		return err
	}
	return createIDMS(ctx, c, scheme, relocation, logger)
}

func Cleanup(ctx context.Context, c client.Client, logger logr.Logger, clusterVersion string) error {
	// if they move from relocation.Spec.ImageDigestMirrors=<something> to relocation.Spec.ImageDigestMirrors=<empty>, we need to delete the ICSP
	if semver.Compare(clusterVersion, "v4.13.0") == -1 {
		return cleanupICSP(ctx, c, logger)
	}
	return cleanupIDMS(ctx, c, logger)
}

// ImageContentSourcePolicy is deprecated since OCP 4.13
// This function converts the values in Spec.RepositoryDigestMirrors into an ImageContentSourcePolicy
// Used for OCP < 4.13
func createICSP(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	op, err := controllerutil.CreateOrUpdate(ctx, c, icsp, func() error {
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

func createIDMS(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	idms := &configv1.ImageDigestMirrorSet{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	op, err := controllerutil.CreateOrUpdate(ctx, c, idms, func() error {
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

func cleanupICSP(ctx context.Context, c client.Client, logger logr.Logger) error {
	icsp := &operatorv1alpha1.ImageContentSourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	if err := c.Delete(ctx, icsp); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logger.Info("ICSP deleted", "ImageContentSourcePolicy", ImageSetName)
	}
	return nil
}

func cleanupIDMS(ctx context.Context, c client.Client, logger logr.Logger) error {
	idms := &configv1.ImageDigestMirrorSet{ObjectMeta: metav1.ObjectMeta{Name: ImageSetName}}
	if err := c.Delete(ctx, idms); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logger.Info("IDMS deleted", "ImageDigestMirrorSet", ImageSetName)
	}
	return nil
}
