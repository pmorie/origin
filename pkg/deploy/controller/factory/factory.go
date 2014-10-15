package factory

import (
  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"

  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  controller "github.com/openshift/origin/pkg/deploy/controller"
  imageapi "github.com/openshift/origin/pkg/image/api"
)

type DeploymentControllerConfigFactory struct {
  OsClient    osclient.Interface
  KubeClient  kclient.Interface
  Environment []kapi.EnvVar
}

func (factory *DeploymentControllerConfigFactory) Create() *controller.DeploymentControllerConfig {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentLW{factory.OsClient}, &deployapi.Deployment{}, queue).Run()

  return &controller.DeploymentControllerConfig{
    OsClient:    factory.OsClient,
    KubeClient:  factory.KubeClient,
    Environment: factory.Environment,
    NextDeployment: func() *deployapi.Deployment {
      return queue.Pop().(*deployapi.Deployment)
    },
  }
}

type DeploymentConfigControllerConfigFactory struct {
  Client osclient.Interface
}

func (factory *DeploymentConfigControllerConfigFactory) Create() *controller.DeploymentConfigControllerConfig {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentConfigLW{factory.Client}, &deployapi.DeploymentConfig{}, queue).Run()

  return &controller.DeploymentConfigControllerConfig{
    Client: factory.Client,
    NextDeploymentConfig: func() *deployapi.DeploymentConfig {
      return queue.Pop().(*deployapi.DeploymentConfig)
    },
  }
}

type ConfigChangeTriggerControllerConfigFactory struct {
  OsClient osclient.Interface
}

func (factory *ConfigChangeTriggerControllerConfigFactory) Create() *controller.ConfigChangeTriggerControllerConfig {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentConfigLW{factory.OsClient}, &deployapi.DeploymentConfig{}, queue).Run()

  store := cache.NewStore()
  cache.NewReflector(&deploymentLW{factory.OsClient}, &deployapi.Deployment{}, store).Run()

  return &controller.ConfigChangeTriggerControllerConfig{
    OsClient: factory.OsClient,
    NextDeploymentConfig: func() *deployapi.DeploymentConfig {
      return queue.Pop().(*deployapi.DeploymentConfig)
    },
    DeploymentStore: store,
  }
}

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

type deploymentLW struct {
  client osclient.Interface
}

func (lw *deploymentLW) List() (runtime.Object, error) {
  return lw.client.ListDeployments(kapi.NewContext(), labels.Everything())
}

func (lw *deploymentLW) Watch(resourceVersion uint64) (watch.Interface, error) {
  return lw.client.WatchDeployments(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
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

type imageRepositoryLW struct {
  client osclient.Interface
}

func (lw *imageRepositoryLW) List() (runtime.Object, error) {
  return lw.client.ListImageRepositories(kapi.NewContext(), labels.Everything())
}

func (lw *imageRepositoryLW) Watch(resourceVersion uint64) (watch.Interface, error) {
  return lw.client.WatchImageRepositories(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
