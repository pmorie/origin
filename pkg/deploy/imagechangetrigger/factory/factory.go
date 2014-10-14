package factory

import (
	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	controller "github.com/openshift/origin/pkg/deploy/imagechangetrigger"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

type ConfigFactory struct {
	Client osclient.Interface
}

func (factory *ConfigFactory) Create() *controller.Config {
	queue := cache.NewFIFO()
	cache.NewReflector(&imageRepositoryLW{factory.Client}, &imageapi.ImageRepository{}, queue).Run()

	store := cache.NewStore()
	cache.NewReflector(&deploymentConfigLW{factory.Client}, &deployapi.DeploymentConfig{}, store).Run()

	return &controller.Config{
		Client:                factory.Client,
		DeploymentConfigStore: store,
		NextImageRepository: func() *imageapi.ImageRepository {
			return queue.Pop().(*imageapi.ImageRepository)
		},
	}
}

type imageRepositoryLW struct {
	client osclient.Interface
}

func (lw *imageRepositoryLW) List() (runtime.Object, error) {
	return lw.client.ListImageRepositories(kapi.NewContext(), labels.Everything())
}

func (lw *imageRepositoryLW) Watch(resourceVersion uint64) (watch.Interface, error) {
	return lw.client.WatchImageRepositories(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}

type deploymentConfigLW struct {
	client osclient.Interface
}

func (lw *deploymentConfigLW) List() (runtime.Object, error) {
	return lw.client.ListDeploymentConfigs(kapi.NewContext(), labels.Everything())
}

func (lw *deploymentConfigLW) Watch(resourceVersion uint64) (watch.Interface, error) {
	return lw.client.WatchDeploymentConfigs(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
