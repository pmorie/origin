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
  deploycontroller "github.com/openshift/origin/pkg/deploy/deploymentcontroller"
)

type ConfigFactory struct {
  OsClient    osclient.Interface
  KubeClient  kubeclient.Interface
  Environment []kapi.EnvVar
}

func (config *ConfigFactory) Create() *deploycontroller.Config {
  queue := cache.NewFIFO()
  cache.NewReflector(&deploymentLW{config.OsClient}, &deployapi.Deployment{}, queue).Run()

  return &deploycontroller.Config{
    OsClient:    config.OsClient,
    KubeClient:  config.KubeClient,
    Environment: config.Environment,
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
