package deploy

import (
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

type DeploymentGenerator struct {
	osClient osclient.Client
}

func (g *DeploymentGenerator) generateDeployment(config *deployapi.DeploymentConfig) (*deployapi.Deployment, error) {
	glog.Infof("Generating deployment for deploymentConfig %v", config.ID)

	labels := make(map[string]string)
	for label, value := range config.Labels {
		labels[label] = value
	}
	labels["configID"] = config.ID

	deployment := &deployapi.Deployment{
		JSONBase:           api.JSONBase{},
		Labels:             labels,
		Strategy:           config.Template.Strategy,
		ControllerTemplate: config.Template.ControllerTemplate,
	}

	for _, repoName := range referencedRepos(config).List() {
		params := paramsForImageChangeTrigger(config, repoName)
		repo := g.imageRepoCache.cachedRepo(repoName)

		for _, container := range deployment.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
			if container.Image == params.ImageName {
				container.Image = repoName + ":" + repo.Tags[params.Tag]
			}
		}
	}

	return deployment, nil
}
