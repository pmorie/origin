package generator

import (
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

type DeploymentConfigGenerator interface {
	Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error)
}

type deploymentConfigGenerator struct {
	osClient osclient.Interface
}

func NewDeploymentConfigGenerator(osClient osclient.Interface) DeploymentConfigGenerator {
	return &deploymentConfigGenerator{osClient: osClient}
}

func (g *deploymentConfigGenerator) Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	config, err := g.osClient.GetDeploymentConfig(deploymentConfigID)

	if err != nil {
		return nil, err
	}

	dirty := false
	for _, repoName := range deploy.ReferencedRepos(config).List() {
		params := deploy.ParamsForImageChangeTrigger(config, repoName)
		repo, repoErr := g.osClient.GetImageRepository(repoName)

		if repoErr != nil {
			return nil, repoErr
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
