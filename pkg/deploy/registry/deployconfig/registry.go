package deployconfig

import (
	api "github.com/openshift/origin/pkg/deploy/api"
)

// Registry is an interface for things that know how to store DeploymentConfigs
type Registry interface {
	ListDeploymentConfigs() (*api.DeploymentConfigList, error)
	GetDeploymentConfig(id string) (*api.DeploymentConfig, error)
	CreateDeploymentConfig(deploymentConfig *api.DeploymentConfig) error
	UpdateDeploymentConfig(deploymentConfig *api.DeploymentConfig) error
	DeleteDeploymentConfig(id string) error
}
