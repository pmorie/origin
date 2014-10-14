package factory

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	"github.com/openshift/origin/pkg/deploy/controller"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

type ImageChangeControllerConfigFactory struct {
	Client osclient.Interface
}

func (factory *ImageChangeControllerConfigFactory) Create() *controller.ImageChangeControllerConfig {
	queue := cache.NewFIFO()
	cache.NewReflector(&imageRepositoryLW{factory.Client}, &imageapi.ImageRepository{}, queue).Run()

	store := cache.NewStore()
	cache.NewReflector(&deploymentConfigLW{factory.Client}, &deployapi.DeploymentConfig{}, store).Run()

	return &controller.ImageChangeControllerConfig{
		Client:                factory.Client,
		DeploymentConfigStore: store,
		NextImageRepository: func() *imageapi.ImageRepository {
			return queue.Pop().(*imageapi.ImageRepository)
		},
	}
}
