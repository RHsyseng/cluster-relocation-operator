package util

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get

// Waits for the API server to update before returning
func WaitForAPIOperator(ctx context.Context, c client.Client, logger logr.Logger) error {
	for {
		co := &configv1.ClusterOperator{}
		if err := c.Get(ctx, types.NamespacedName{Name: "kube-apiserver"}, co); err != nil {
			return err
		}
		coProgressing := true
		for _, v := range co.Status.Conditions {
			if v.Type == configv1.OperatorProgressing && v.Status == configv1.ConditionTrue {
				logger.Info("Waiting for API server to stop progressing")
				time.Sleep(time.Minute)
			} else if v.Type == configv1.OperatorProgressing && v.Status == configv1.ConditionFalse {
				coProgressing = false
			}
		}
		if !coProgressing {
			break
		}
	}
	return nil
}
