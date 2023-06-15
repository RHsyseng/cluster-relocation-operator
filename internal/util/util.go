package util

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get

// Waits for the operator to update before returning
func WaitForCO(ctx context.Context, c client.Client, logger logr.Logger, operator string) error {
	logger.Info(fmt.Sprintf("Waiting for %s Progressing to be %s", operator, configv1.ConditionTrue))
	if err := waitProgressing(ctx, c, logger, operator, configv1.ConditionTrue); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Waiting for %s Progressing to be %s", operator, configv1.ConditionFalse))
	if err := waitProgressing(ctx, c, logger, operator, configv1.ConditionFalse); err != nil {
		return err
	}
	return nil
}

func waitProgressing(ctx context.Context, c client.Client, logger logr.Logger, operator string, desired configv1.ConditionStatus) error {
	var current configv1.ConditionStatus
	if desired == configv1.ConditionTrue {
		current = configv1.ConditionFalse
	} else {
		current = configv1.ConditionTrue
	}
	startTime := time.Now()
	for {
		co := &configv1.ClusterOperator{}
		if err := c.Get(ctx, types.NamespacedName{Name: operator}, co); err != nil {
			return err
		}
		desiredStatus := false
		for _, v := range co.Status.Conditions {
			if v.Type == configv1.OperatorProgressing && v.Status == current {
				logger.Info(fmt.Sprintf("Still waiting for %s Progressing to be %s", operator, desired))
				time.Sleep(time.Second * 10)
			} else if v.Type == configv1.OperatorProgressing && v.Status == desired {
				desiredStatus = true
			}
		}
		if desiredStatus {
			break
		}
		// we set a 5 minute timeout in case the operator never gets to Progressing
		if time.Since(startTime) > time.Minute*5 {
			break
		}
	}
	return nil
}
