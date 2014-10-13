package factory

import (
	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	osclient "github.com/openshift/origin/pkg/client"
	api "github.com/openshift/origin/pkg/deploy/api"
	controller "github.com/openshift/origin/pkg/deploy/configcontroller"
)

type ConfigFactory struct {
	Client osclient.Interface
}

func (factory *ConfigFactory) Create() *controller.Config {
	queue := cache.NewFIFO()
	cache.NewReflector(&listWatch{factory.Client}, &api.DeploymentConfig{}, queue).Run()

	return &controller.Config{
		Client: factory.Client,
		NextDeploymentConfig: func() *api.DeploymentConfig {
			return queue.Pop().(*api.DeploymentConfig)
		},
	}
}

type listWatch struct {
	client osclient.Interface
}

func (lw *listWatch) List() (runtime.Object, error) {
	return lw.client.ListDeploymentConfigs(kapi.NewContext(), labels.Everything())
}

func (lw *listWatch) Watch(resourceVersion uint64) (watch.Interface, error) {
	return lw.client.WatchDeploymentConfigs(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
