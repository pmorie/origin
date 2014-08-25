package deploy

import (
	"fmt"

	"code.google.com/p/go-uuid/uuid"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kubeerrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/golang/glog"
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

// List obtains a list of Deployments that match selector.
func (storage *Storage) List(selector labels.Selector) (interface{}, error) {
	result := deployapi.DeploymentList{}
	deployments, err := storage.registry.ListDeployments()
	if err == nil {
		for _, deployment := range deployments.Items {
			if selector.Matches(labels.Set(deployment.Labels)) {
				result.Items = append(result.Items, deployment)
			}
		}
	}
	return result, err
}

// New creates a new Deployment for use with Create and Update
func (storage *Storage) New() interface{} {
	return &deployapi.Deployment{}
}

// Get obtains the Deployment specified by its id.
func (storage *Storage) Get(id string) (interface{}, error) {
	deployment, err := storage.registry.GetDeployment(id)
	if err != nil {
		return nil, err
	}
	return deployment, err
}

// Delete asynchronously deletes the Deployment specified by its id.
func (storage *Storage) Delete(id string) (<-chan interface{}, error) {
	return apiserver.MakeAsync(func() (interface{}, error) {
		return api.Status{Status: api.StatusSuccess}, storage.registry.DeleteDeployment(id)
	}), nil
}

// Create registers a given new Deployment instance to storage.registry.
func (storage *Storage) Create(obj interface{}) (<-chan interface{}, error) {
	deployment, ok := obj.(*deployapi.Deployment)
	if !ok {
		return nil, fmt.Errorf("not a deployment: %#v", obj)
	}

	glog.Infof("Creating deployment with ID: %v", deployment.ID)

	if len(deployment.ID) == 0 {
		deployment.ID = uuid.NewUUID().String()
	}
	deployment.Status = deployapi.DeploymentNew

	if errs := deployapi.ValidateDeployment(deployment); len(errs) > 0 {
		return nil, kubeerrors.NewInvalid("deployment", deployment.ID, errs)
	}

	return apiserver.MakeAsync(func() (interface{}, error) {
		err := storage.registry.CreateDeployment(deployment)
		if err != nil {
			return nil, err
		}
		return *deployment, nil
	}), nil
}

// Update replaces a given Deployment instance with an existing instance in storage.registry.
func (storage *Storage) Update(obj interface{}) (<-chan interface{}, error) {
	deployment, ok := obj.(*deployapi.Deployment)
	if !ok {
		return nil, fmt.Errorf("not a deployment: %#v", obj)
	}
	if len(deployment.ID) == 0 {
		return nil, fmt.Errorf("ID should not be empty: %#v", deployment)
	}
	return apiserver.MakeAsync(func() (interface{}, error) {
		err := storage.registry.UpdateDeployment(deployment)
		if err != nil {
			return nil, err
		}
		return *deployment, nil
	}), nil
}
