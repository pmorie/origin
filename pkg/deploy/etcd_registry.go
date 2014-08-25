package deploy

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/tools"
	"github.com/openshift/origin/pkg/deploy/api"
)

// EtcdRegistry implements deployment.Registry and deploymentconfig.Registry interfaces.
type EtcdRegistry struct {
	helper tools.EtcdHelper
}

// MakeEtcdRegistry creates an etcd registry using the given etcd client.
func NewEtcdRegistry(client tools.EtcdClient) *EtcdRegistry {
	registry := &EtcdRegistry{
		helper: tools.EtcdHelper{client, runtime.Codec, runtime.ResourceVersioner},
	}
	return registry
}

// ListDeployments obtains a list of Deployments.
func (registry *EtcdRegistry) ListDeployments() (*api.DeploymentList, error) {
	deployments := api.DeploymentList{}
	err := registry.helper.ExtractList("/registry/deployments", &deployments.Items, &deployments.ResourceVersion)
	return &deployments, err
}

func makeDeploymentKey(id string) string {
	return "/registry/deployments/" + id
}

// GetDeployment gets a specific Deployment specified by its ID.
func (registry *EtcdRegistry) GetDeployment(id string) (*api.Deployment, error) {
	var deployment api.Deployment
	key := makeDeploymentKey(id)
	err := registry.helper.ExtractObj(key, &deployment, false)
	if tools.IsEtcdNotFound(err) {
		return nil, errors.NewNotFound("deployment", id)
	}
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// CreateDeployment creates a new Deployment.
func (registry *EtcdRegistry) CreateDeployment(deployment *api.Deployment) error {
	err := registry.helper.CreateObj(makeDeploymentKey(deployment.ID), deployment)
	if tools.IsEtcdNodeExist(err) {
		return errors.NewAlreadyExists("deployment", deployment.ID)
	}
	return err
}

// UpdateDeployment replaces an existing Deployment.
func (registry *EtcdRegistry) UpdateDeployment(deployment *api.Deployment) error {
	return registry.helper.SetObj(makeDeploymentKey(deployment.ID), deployment)
}

// DeleteDeployment deletes a Deployment specified by its ID.
func (registry *EtcdRegistry) DeleteDeployment(id string) error {
	key := makeDeploymentKey(id)
	err := registry.helper.Delete(key, false)
	if tools.IsEtcdNotFound(err) {
		return errors.NewNotFound("deployment", id)
	}
	return err
}

// ListDeploymentConfigs obtains a list of DeploymentConfigs.
func (registry *EtcdRegistry) ListDeploymentConfigs() (*api.DeploymentConfigList, error) {
	deploymentConfigs := api.DeploymentConfigList{}
	err := registry.helper.ExtractList("/registry/deploymentConfigs", &deploymentConfigs.Items, &deploymentConfigs.ResourceVersion)
	return &deploymentConfigs, err
}

func makeDeploymentConfigKey(id string) string {
	return "/registry/deploymentConfigs/" + id
}

// GetDeploymentConfig gets a specific DeploymentConfig specified by its ID.
func (registry *EtcdRegistry) GetDeploymentConfig(id string) (*api.DeploymentConfig, error) {
	var deploymentConfig api.DeploymentConfig
	key := makeDeploymentConfigKey(id)
	err := registry.helper.ExtractObj(key, &deploymentConfig, false)
	if tools.IsEtcdNotFound(err) {
		return nil, errors.NewNotFound("deploymentConfig", id)
	}
	if err != nil {
		return nil, err
	}
	return &deploymentConfig, nil
}

// CreateDeploymentConfig creates a new DeploymentConfig.
func (registry *EtcdRegistry) CreateDeploymentConfig(deploymentConfig *api.DeploymentConfig) error {
	err := registry.helper.CreateObj(makeDeploymentConfigKey(deploymentConfig.ID), deploymentConfig)
	if tools.IsEtcdNodeExist(err) {
		return errors.NewAlreadyExists("deploymentConfig", deploymentConfig.ID)
	}
	return err
}

// UpdateDeploymentConfig replaces an existing DeploymentConfig.
func (registry *EtcdRegistry) UpdateDeploymentConfig(deploymentConfig *api.DeploymentConfig) error {
	return registry.helper.SetObj(makeDeploymentConfigKey(deploymentConfig.ID), deploymentConfig)
}

// DeleteDeploymentConfig deletes a DeploymentConfig specified by its ID.
func (registry *EtcdRegistry) DeleteDeploymentConfig(id string) error {
	key := makeDeploymentConfigKey(id)
	err := registry.helper.Delete(key, false)
	if tools.IsEtcdNotFound(err) {
		return errors.NewNotFound("deploymentConfig", id)
	}
	return err
}
