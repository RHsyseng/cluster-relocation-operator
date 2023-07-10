package acm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete;get;list;watch
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=klusterlets,verbs=get;list;watch

// these resources are created by the 'crds.yaml' file that is provided by ACM
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create

// these resources are created by the 'import.yaml' file that is provided by ACM
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=create
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=create
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create
//+kubebuilder:rbac:groups="",resources=secrets,verbs=create
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=klusterlets,verbs=create

// these permissions are granted by the ClusterRoles created by import.yaml
// since an object cannot grant permissions that it doesn't have, the operator needs these as well
// ClusterRole/klusterlet
//+kubebuilder:rbac:groups="",resources=secrets;configmaps;serviceaccounts,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update;watch;patch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=create;get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;patch;update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=create;get;list;update;watch;patch;delete;escalate;bind
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=klusterlets,verbs=get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=klusterlets/status,verbs=update;patch
//+kubebuilder:rbac:groups=work.open-cluster-management.io,resources=appliedmanifestworks,verbs=list;update;patch

// ClusterRole/klusterlet-bootstrap-kubeconfig
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;update

// ClusterRole/open-cluster-management:klusterlet-admin-aggregate-clusterrole
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=klusterlets,verbs=get;list;watch;create;update;patch;delete

// returns nil if the Klusterlet is Available, error otherwise
func checkKlusterlet(ctx context.Context, c client.Client, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	klusterlet := &operatorapiv1.Klusterlet{}
	err := c.Get(ctx, types.NamespacedName{Name: "klusterlet"}, klusterlet)
	if err == nil {
		klusterletCondition := apimeta.FindStatusCondition(klusterlet.Status.Conditions, "Available")
		if klusterletCondition != nil && klusterletCondition.Status == metav1.ConditionTrue {
			logger.Info("cluster registered to ACM")

			acmSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: relocation.Spec.ACMRegistration.ACMSecret.Name, Namespace: relocation.Spec.ACMRegistration.ACMSecret.Namespace}}
			if err := c.Delete(ctx, acmSecret); err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
			} else {
				logger.Info("acmSecret deleted")
			}
		} else {
			return fmt.Errorf("cluster not registered to ACM")
		}
	}
	return err
}

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if relocation.Spec.ACMRegistration == nil {
		return nil
	}

	// skip these steps if the cluster is already registered to ACM
	if checkKlusterlet(ctx, c, relocation, logger) == nil {
		return nil
	}

	// the acmSecret holds the credentials for the ACM cluster
	// these credentials should allow the following:
	// Creating ManagedClusters (these are cluster scoped resources)
	// Creating KlusterletAddonConfigs (these are namespace scoped resources)
	// Getting Secrets (these are namespace scoped resources)
	//
	// the secret is deleted once registration succeeds
	acmSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: relocation.Spec.ACMRegistration.ACMSecret.Name, Namespace: relocation.Spec.ACMRegistration.ACMSecret.Namespace}, acmSecret); err != nil {
		return err
	}

	config := rest.Config{
		Host:            relocation.Spec.ACMRegistration.URL,
		BearerToken:     string(acmSecret.Data["token"]),
		TLSClientConfig: rest.TLSClientConfig{CAData: acmSecret.Data["ca.crt"]},
	}

	acmClient, err := client.New(&config, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	managedClusterSet := "default"
	if relocation.Spec.ACMRegistration.ManagedClusterSet != nil {
		managedClusterSet = *relocation.Spec.ACMRegistration.ManagedClusterSet
	}

	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: relocation.Spec.ACMRegistration.ClusterName,
			Labels: map[string]string{
				"cloud":  "auto-detect",
				"vendor": "auto-detect",
				"cluster.open-cluster-management.io/clusterset": managedClusterSet,
			},
		},
		Spec: clusterv1.ManagedClusterSpec{
			HubAcceptsClient: true,
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{URL: fmt.Sprintf("https://api.%s:6443", relocation.Spec.Domain)},
			},
		},
	}
	if err := acmClient.Create(ctx, managedCluster); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}

	logger.Info("getting ACM import secret")
	importSecret := &corev1.Secret{}
	for {
		if err := acmClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-import", relocation.Spec.ACMRegistration.ClusterName), Namespace: relocation.Spec.ACMRegistration.ClusterName}, importSecret); err != nil {
			// after the ManagedCluster is created, it can take some time for this secret and the RBAC roles to be created
			if errors.IsNotFound(err) || errors.IsForbidden(err) {
				time.Sleep(time.Second * 10)
				continue
			}
			return err
		}
		break
	}

	if relocation.Spec.ACMRegistration.KlusterletAddonConfig != nil {
		klusterletAddonConfig := &agentv1.KlusterletAddonConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      relocation.Spec.ACMRegistration.ClusterName,
				Namespace: relocation.Spec.ACMRegistration.ClusterName,
			},
			Spec: *relocation.Spec.ACMRegistration.KlusterletAddonConfig,
		}
		if err := acmClient.Create(ctx, klusterletAddonConfig); err != nil {
			if !errors.IsAlreadyExists(err) {
				return err
			}
		}
	}

	logger.Info("applying ACM CRDs")
	// the import secret that we obtained from the ACM cluster contains YAML manifests that need to be applied here
	d := yaml.NewYAMLToJSONDecoder(bytes.NewReader(importSecret.Data["crds.yaml"]))
	for {
		klusterletCRDObj := &unstructured.Unstructured{}
		if err := d.Decode(klusterletCRDObj); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if klusterletCRDObj.Object == nil {
			continue
		}

		if err := c.Create(ctx, klusterletCRDObj); err != nil {
			if !errors.IsAlreadyExists(err) {
				return err
			}
		}
	}

	logger.Info("applying ACM import manifests")
	// the import secret that we obtained from the ACM cluster contains YAML manifests that need to be applied here
	d = yaml.NewYAMLToJSONDecoder(bytes.NewReader(importSecret.Data["import.yaml"]))
	for {
		importObj := &unstructured.Unstructured{}
		if err := d.Decode(importObj); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if importObj.Object == nil {
			continue
		}

		if err := c.Create(ctx, importObj); err != nil {
			if !errors.IsAlreadyExists(err) {
				return err
			}
		}
	}

	logger.Info("waiting for Klusterlet to become Available")
	// wait for the Klusterlet to become Available
	startTime := time.Now()
	for {
		if checkKlusterlet(ctx, c, relocation, logger) == nil {
			return nil
		}
		time.Sleep(time.Second * 10)

		// we set a 5 minute timeout in case the Klusterlet never gets to Available
		if time.Since(startTime) > time.Minute*5 {
			return fmt.Errorf("klusterlet error")
		}
	}
}
