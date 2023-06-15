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

//+kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get;list;watch

// Waits for the operator to update before returning
func WaitForCO(ctx context.Context, c client.Client, logger logr.Logger, operator string, waitProgressingTrue bool) error {
	if waitProgressingTrue {
		logger.Info(fmt.Sprintf("Waiting for %s Progressing to be %s", operator, configv1.ConditionTrue))
		if err := waitStatus(ctx, c, logger, operator, configv1.OperatorProgressing, configv1.ConditionTrue); err != nil {
			return err
		}
	}

	logger.Info(fmt.Sprintf("Waiting for %s Progressing to be %s", operator, configv1.ConditionFalse))
	if err := waitStatus(ctx, c, logger, operator, configv1.OperatorProgressing, configv1.ConditionFalse); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Waiting for %s Available to be %s", operator, configv1.ConditionTrue))
	if err := waitStatus(ctx, c, logger, operator, configv1.OperatorAvailable, configv1.ConditionTrue); err != nil {
		return err
	}
	return nil
}

func waitStatus(ctx context.Context, c client.Client, logger logr.Logger, operator string, statusType configv1.ClusterStatusConditionType, desired configv1.ConditionStatus) error {
	startTime := time.Now()
	for {
		co := &configv1.ClusterOperator{}
		if err := c.Get(ctx, types.NamespacedName{Name: operator}, co); err != nil {
			return err
		}
		desiredStatus := false
		for _, v := range co.Status.Conditions {
			if v.Type == statusType && v.Status != desired {
				logger.Info(fmt.Sprintf("Still waiting for %s %s to be %s", operator, statusType, desired))
				time.Sleep(time.Second * 10)
			} else if v.Type == statusType && v.Status == desired {
				desiredStatus = true
			}
		}
		if desiredStatus {
			break
		}
		// we set a 5 minute timeout in case the operator never gets to Progressing
		if time.Since(startTime) > time.Minute*5 && statusType == configv1.OperatorProgressing && desired == configv1.ConditionTrue {
			break
		}
	}
	return nil
}
