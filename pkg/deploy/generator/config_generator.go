package generator

import (
	"fmt"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployreg "github.com/openshift/origin/pkg/deploy/registry/deploy"
	deployconfig "github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	imagerepo "github.com/openshift/origin/pkg/image/registry/imagerepository"
)

type DeploymentConfigGenerator interface {
	Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error)
}

type deploymentConfigGenerator struct {
	deploymentRegistry   deployreg.Registry
	deployConfigRegistry deployconfig.Registry
	imageRepoRegistry    imagerepo.Registry
}

func NewDeploymentConfigGenerator(deploymentRegistry deployreg.Registry,
	deployConfigRegistry deployconfig.Registry, imageRepoRegistry imagerepo.Registry) DeploymentConfigGenerator {
	return &deploymentConfigGenerator{
		deploymentRegistry:   deploymentRegistry,
		deployConfigRegistry: deployConfigRegistry,
		imageRepoRegistry:    imageRepoRegistry,
	}
}

func (g *deploymentConfigGenerator) Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	var (
		deploymentConfig *deployapi.DeploymentConfig
		deployment       *deployapi.Deployment
		err              error
	)

	if deploymentConfig, err = g.deployConfigRegistry.GetDeploymentConfig(deploymentConfigID); err != nil {
		return nil, err
	}

	if deploymentConfig == nil {
		return nil, fmt.Errorf("No deployment config returned with id %v", deploymentConfigID)
	}

	deploymentID := deploy.LatestDeploymentIDForConfig(deploymentConfig)
	if deployment, err = g.deploymentRegistry.GetDeployment(deploymentID); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	configPodTemplate := deploymentConfig.Template.ControllerTemplate.PodTemplate

	for _, repoName := range deploy.ReferencedRepos(deploymentConfig).List() {
		params := deploy.ParamsForImageChangeTrigger(deploymentConfig, repoName)
		repo, repoErr := g.imageRepoRegistry.GetImageRepository(repoName)

		if repoErr != nil {
			return nil, repoErr
		}

		// TODO: If the repo a config references has disappeared, what's the correct reaction?
		if repo == nil {
			glog.Errorf("Received a nil ImageRepository for repoName=%s (potentially invalid DeploymentConfig state)", repoName)
			continue
		}

		// TODO: If the tag is missing, what's the correct reaction?
		tag, tagExists := repo.Tags[params.Tag]
		if !tagExists {
			glog.Errorf("No tag %s found for repository %s (potentially invalid DeploymentConfig state)", tag, repoName)
			continue
		}
		newImage := repo.DockerImageRepository + ":" + tag

		containersToCheck := util.NewStringSet(params.ContainerNames...)
		for i, container := range configPodTemplate.DesiredState.Manifest.Containers {
			if !containersToCheck.Has(container.Name) {
				continue
			}

			// TODO: If we grow beyond this single mutation, diffing hashes of
			// a clone of the original config vs the mutation would be more generic.
			if newImage != container.Image {
				configPodTemplate.DesiredState.Manifest.Containers[i].Image = newImage
			}
		}
	}

	if deployment == nil {
		// TODO: Is this a safe assumption?
		deploymentConfig.LatestVersion = 1
	} else if !deploy.PodTemplatesEqual(configPodTemplate, deployment.ControllerTemplate.PodTemplate) {
		deploymentConfig.LatestVersion += 1
	}

	return deploymentConfig, nil
}
