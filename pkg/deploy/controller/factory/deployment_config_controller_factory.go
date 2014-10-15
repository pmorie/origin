package factory

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	osclient "github.com/openshift/origin/pkg/client"
	api "github.com/openshift/origin/pkg/deploy/api"
	"github.com/openshift/origin/pkg/deploy/controller"
)

type DeploymentConfigControllerConfigFactory struct {
	Client osclient.Interface
}

func (factory *DeploymentConfigControllerConfigFactory) Create() *controller.DeploymentConfigControllerConfig {
	queue := cache.NewFIFO()
	cache.NewReflector(&deploymentConfigLW{factory.Client}, &api.DeploymentConfig{}, queue).Run()

	return &controller.DeploymentConfigControllerConfig{
		Client: factory.Client,
		NextDeploymentConfig: func() *api.DeploymentConfig {
			return queue.Pop().(*api.DeploymentConfig)
		},
	}
}
