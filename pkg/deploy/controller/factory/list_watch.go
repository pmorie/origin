package factory

import (
	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	osclient "github.com/openshift/origin/pkg/client"
)

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

type deploymentLW struct {
	client osclient.Interface
}

func (lw *deploymentLW) List() (runtime.Object, error) {
	return lw.client.ListDeployments(kapi.NewContext(), labels.Everything())
}

func (lw *deploymentLW) Watch(resourceVersion uint64) (watch.Interface, error) {
	return lw.client.WatchDeployments(kapi.NewContext(), labels.Everything(), labels.Everything(), 0)
}
