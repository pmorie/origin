package factory

import (
  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  kubeclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"

  "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  "github.com/openshift/origin/pkg/deploy/controller"
)

type DeploymentControllerConfigFactory struct {
  OsClient    osclient.Interface
  KubeClient  kubeclient.Interface
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
