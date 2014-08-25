package deployconfig

import (
	"fmt"

	"code.google.com/p/go-uuid/uuid"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// Storage is an implementation of RESTStorage for the api server.
type Storage struct {
	registry Registry
}

func NewStorage(registry Registry) apiserver.RESTStorage {
	return &Storage{
		registry: registry,
	}
}

// List obtains a list of DeploymentConfigs that match selector.
func (storage *Storage) List(selector labels.Selector) (interface{}, error) {
	result := deployapi.DeploymentConfigList{}
	deploymentConfigs, err := storage.registry.ListDeploymentConfigs()
	if err == nil {
		for _, deploymentConfig := range deploymentConfigs.Items {
			if selector.Matches(labels.Set(deploymentConfig.Labels)) {
				result.Items = append(result.Items, deploymentConfig)
			}
		}
	}
	return result, err
}

// Get obtains the DeploymentConfig specified by its id.
func (storage *Storage) Get(id string) (interface{}, error) {
	deploymentConfig, err := storage.registry.GetDeploymentConfig(id)
	if err != nil {
		return nil, err
	}
	return deploymentConfig, err
}

// Delete asynchronously deletes the DeploymentConfig specified by its id.
func (storage *Storage) Delete(id string) (<-chan interface{}, error) {
	return apiserver.MakeAsync(func() (interface{}, error) {
		return api.Status{Status: api.StatusSuccess}, storage.registry.DeleteDeploymentConfig(id)
	}), nil
}

// New creates a new DeploymentConfig for use with Create and Update
func (storage *Storage) New() interface{} {
	return &deployapi.DeploymentConfig{}
}

// Create registers a given new DeploymentConfig instance to storage.registry.
func (storage *Storage) Create(obj interface{}) (<-chan interface{}, error) {
	deploymentConfig, ok := obj.(*deployapi.DeploymentConfig)
	if !ok {
		return nil, fmt.Errorf("not a deploymentConfig: %#v", obj)
	}
	if len(deploymentConfig.ID) == 0 {
		deploymentConfig.ID = uuid.NewUUID().String()
	}

	//TODO: Add validation

	return apiserver.MakeAsync(func() (interface{}, error) {
		err := storage.registry.CreateDeploymentConfig(deploymentConfig)
		if err != nil {
			return nil, err
		}
		return *deploymentConfig, nil
	}), nil
}

// Update replaces a given DeploymentConfig instance with an existing instance in storage.registry.
func (storage *Storage) Update(obj interface{}) (<-chan interface{}, error) {
	deploymentConfig, ok := obj.(*deployapi.DeploymentConfig)
	if !ok {
		return nil, fmt.Errorf("not a deploymentConfig: %#v", obj)
	}
	if len(deploymentConfig.ID) == 0 {
		return nil, fmt.Errorf("ID should not be empty: %#v", deploymentConfig)
	}
	return apiserver.MakeAsync(func() (interface{}, error) {
		err := storage.registry.UpdateDeploymentConfig(deploymentConfig)
		if err != nil {
			return nil, err
		}
		return *deploymentConfig, nil
	}), nil
}
