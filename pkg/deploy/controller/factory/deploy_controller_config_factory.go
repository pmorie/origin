package factory

import (
  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  kubeclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"

  "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
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

type deploymentLW struct {
  client osclient.Interface
}

func (w *deploymentLW) List() (runtime.Object, error) {
  return w.client.ListDeployments(kapi.NewContext(), labels.Everything())
}

func (w *deploymentLW) Watch(resourceVersion uint64) (watch.Interface, error) {
  return w.client.WatchDeployments(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
