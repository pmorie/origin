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
	glog.Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	config, err := g.osClient.GetDeploymentConfig(deploymentConfigID)

	if err != nil {
		return nil, err
	}

	dirty := false
	for _, repoName := range referencedRepos(config).List() {
		params := paramsForImageChangeTrigger(config, repoName)
		repo, err := g.osClient.GetImageRepository(repoName)

		if err != nil {
			return nil, err
		}

		for _, container := range config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
			if container.Image != params.ImageName {
				continue
			}

			// TODO: If we grow beyond this single mutation, diffing hashes of
			// a clone of the original config vs the mutation would be more generic.
			newImage := repoName + ":" + repo.Tags[params.Tag]
			if newImage != container.Image {
				container.Image = newImage
				dirty = true
			}
		}
	}

	if dirty {
		config.LatestVersion += 1
	}

	return config, nil
}
