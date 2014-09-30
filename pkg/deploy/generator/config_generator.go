package generator

import (
	"fmt"
	"github.com/golang/glog"
	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployconfig "github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	imagerepo "github.com/openshift/origin/pkg/image/registry/imagerepository"
)

type DeploymentConfigGenerator interface {
	Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error)
}

type deploymentConfigGenerator struct {
	deployConfigRegistry deployconfig.Registry
	imageRepoRegistry    imagerepo.Registry
}

func NewDeploymentConfigGenerator(deployConfigRegistry deployconfig.Registry, imageRepoRegistry imagerepo.Registry) DeploymentConfigGenerator {
	return &deploymentConfigGenerator{
		deployConfigRegistry: deployConfigRegistry,
		imageRepoRegistry:    imageRepoRegistry,
	}
}

func (g *deploymentConfigGenerator) Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	config, err := g.deployConfigRegistry.GetDeploymentConfig(deploymentConfigID)
	if err != nil {
		return nil, err
	}

	if config == nil {
		return nil, fmt.Errorf("No deployment config returned with id %v", deploymentConfigID)
	}

	dirty := false
	for _, repoName := range deploy.ReferencedRepos(config).List() {
		params := deploy.ParamsForImageChangeTrigger(config, repoName)
		repo, repoErr := g.imageRepoRegistry.GetImageRepository(repoName)

		if repoErr != nil {
			return nil, repoErr
		}

		// TODO: If the repo a config references has disappeared, what's the correct reaction?
		if repo == nil {
			continue
		}

		for _, container := range config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
			if container.Image != params.ImageName {
				continue
			}

			// TODO: If we grow beyond this single mutation, diffing hashes of
			// a clone of the original config vs the mutation would be more generic.
			// TODO: If the referenced tag doesn't exist, what's the correct reaction?
			if tag, exists := repo.Tags[params.Tag]; exists {
				newImage := repoName + ":" + tag
				if newImage != container.Image {
					container.Image = newImage
					dirty = true
				}
			}
		}
	}

	if dirty {
		config.LatestVersion += 1
	}

	return config, nil
}
