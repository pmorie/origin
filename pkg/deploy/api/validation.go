package api

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/golang/glog"
)

func ValidateDeployment(deployment *Deployment) errors.ErrorList {
	result := errors.ErrorList{}

	glog.Infof("Validating deployment %+v", deployment)

	if len(deployment.Strategy.Type) == 0 {
		result = append(result, errors.NewFieldRequired("Strategy.Type", deployment.Strategy.Type))
	}

	if len(deployment.ConfigID) == 0 {
		result = append(result, errors.NewFieldRequired("ConfigID", deployment.ConfigID))
	}

	return result
}

func ValidateDeploymentConfig(config *DeploymentConfig) errors.ErrorList {
	return errors.ErrorList{}
}
