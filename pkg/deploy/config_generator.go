package deploy

import (
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

type DeploymentGenerator struct {
	osClient osclient.Client
}

func (g *DeploymentGenerator) generateDeployment(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.Infof("Generating deploymentConfig %v", deploymentConfigID)

	config, err = g.osClient.GetDeploymentConfig(deploymentConfigID)

	if err != nil {
		return nil, err
	}

	for _, repoName := range referencedRepos(config).List() {
		params := paramsForImageChangeTrigger(config, repoName)
		repo := g.imageRepoCache.cachedRepo(repoName)

		for _, container := range config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
			if container.Image == params.ImageName {
				container.Image = repoName + ":" + repo.Tags[params.Tag]
			}
		}
	}

	return config, nil
}
