package factory

import (
  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
  osclient "github.com/openshift/origin/pkg/client"
  api "github.com/openshift/origin/pkg/deploy/api"
  controller "github.com/openshift/origin/pkg/deploy/configchangetrigger"
)

type ConfigFactory struct {
  OsClient osclient.Interface
}

func (factory *ConfigFactory) Create() *controller.Config {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentConfigLW{factory.OsClient}, &api.DeploymentConfig{}, queue).Run()

  store := cache.NewStore()
  cache.NewReflector(&deploymentLW{factory.OsClient}, &api.Deployment{}, store).Run()

  return &controller.Config{
    OsClient: factory.OsClient,
    NextDeploymentConfig: func() *api.DeploymentConfig {
      return queue.Pop().(*api.DeploymentConfig)
    },
    DeploymentStore: store,
  }
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

type deploymentLW struct {
  client osclient.Interface
}

func (lw *deploymentLW) List() (runtime.Object, error) {
  return lw.client.ListDeployments(kapi.NewContext(), labels.Everything())
}

func (lw *deploymentLW) Watch(resourceVersion uint64) (watch.Interface, error) {
  return lw.client.WatchDeployments(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
