package factory

import (
  "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  osclient "github.com/openshift/origin/pkg/client"
  api "github.com/openshift/origin/pkg/deploy/api"
  "github.com/openshift/origin/pkg/deploy/controller"
)

type ConfigChangeTriggerControllerConfigFactory struct {
  OsClient osclient.Interface
}

func (factory *ConfigChangeTriggerControllerConfigFactory) Create() *controller.ConfigChangeTriggerControllerConfig {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentConfigLW{factory.OsClient}, &api.DeploymentConfig{}, queue).Run()

  store := cache.NewStore()
  cache.NewReflector(&deploymentLW{factory.OsClient}, &api.Deployment{}, store).Run()

  return &controller.ConfigChangeTriggerControllerConfig{
    OsClient: factory.OsClient,
    NextDeploymentConfig: func() *api.DeploymentConfig {
      return queue.Pop().(*api.DeploymentConfig)
    },
    DeploymentStore: store,
  }
}
