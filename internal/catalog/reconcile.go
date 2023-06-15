package catalog

import (
	"context"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	operatorhubv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups=operators.coreos.com,resources=catalogsources,verbs=create;update;get;list;delete;watch

const marketplaceNamespace = "openshift-marketplace"

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if err := Cleanup(ctx, c, relocation, logger); err != nil {
		return err
	}

	for _, v := range relocation.Spec.CatalogSources {
		catalogSource := &operatorhubv1alpha1.CatalogSource{ObjectMeta: metav1.ObjectMeta{Name: v.Name, Namespace: marketplaceNamespace}}
		op, err := controllerutil.CreateOrUpdate(ctx, c, catalogSource, func() error {
			catalogSource.Spec.Image = v.Image
			catalogSource.Spec.SourceType = operatorhubv1alpha1.SourceTypeGrpc
			// Set the controller as the owner so that the CatalogSource is deleted along with the CR
			return controllerutil.SetControllerReference(relocation, catalogSource, scheme)
		})
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("Updated Catalog Source", "CatalogSource", v.Name, "OperationResult", op)
		}
	}
	return nil
}

func Cleanup(ctx context.Context, c client.Client, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	// if they remove something from relocation.Spec.CatalogSources, we need to clean it up
	catalogSources := &operatorhubv1alpha1.CatalogSourceList{}
	if err := c.List(ctx, catalogSources, client.InNamespace(marketplaceNamespace)); err != nil {
		return err
	}
	for _, v := range catalogSources.Items { // loop through all existing CatalogSources
		if len(v.ObjectMeta.OwnerReferences) > 0 &&
			v.ObjectMeta.OwnerReferences[0].APIVersion == relocation.APIVersion &&
			v.ObjectMeta.OwnerReferences[0].Kind == relocation.Kind { // check if we own this CatalogSource
			var existsInSpec bool

			for _, w := range relocation.Spec.CatalogSources { // check if the current Spec wants this CatalogSource
				if v.Name == w.Name {
					existsInSpec = true
				}
			}

			if !existsInSpec {
				// if we own this CatalogSource, but it's not in the Spec, then it is old and needs to be removed
				if err := c.Delete(ctx, &v); err != nil {
					return err
				}
				logger.Info("Deleted old Catalog Source", "CatalogSource", v.Name)
			}
		}
	}
	return nil
}
