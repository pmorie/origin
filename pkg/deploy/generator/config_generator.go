package generator

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployreg "github.com/openshift/origin/pkg/deploy/registry/deploy"
	deployconfig "github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	imageapi "github.com/openshift/origin/pkg/image/api"
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
		glog.Errorf("Error getting deploymentConfig for id %v", deploymentConfigID)
		return nil, err
	}

	if deploymentConfig == nil {
		return nil, fmt.Errorf("No deployment config returned with id %v", deploymentConfigID)
	}

	deploymentID := deploy.LatestDeploymentIDForConfig(deploymentConfig)
	if deployment, err = g.deploymentRegistry.GetDeployment(deploymentID); err != nil {
		if !errors.IsNotFound(err) {
			glog.Errorf("Error getting deployment: %#v", err)
			return nil, err
		}
	}

	configPodTemplate := deploymentConfig.Template.ControllerTemplate.PodTemplate
	imageRepos := g.imageRepos()

	for _, repoName := range deploy.ReferencedRepos(deploymentConfig).List() {
		params := deploy.ParamsForImageChangeTrigger(deploymentConfig, repoName)
		repo, ok := imageRepos[params.RepositoryName]
		if !ok {
			return nil, fmt.Errorf("Referenced an imageRepo '%s' without a record in OpenShift (known repos: %v)", params.RepositoryName, imageRepos)
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
				glog.Infof("Updating container %v to %v", container.Name, newImage)
				configPodTemplate.DesiredState.Manifest.Containers[i].Image = newImage
			}
		}
	}

	if deployment == nil {
		if deploymentConfig.LatestVersion == 0 {
			// TODO: Is this a safe assumption? -- NO
			glog.Infof("Setting LatestVersion=1 due to absent deployment for config %s", deploymentConfigID)
			deploymentConfig.LatestVersion = 1
		} else {
			glog.Infof("Config %v: no latest deployment and LatestVersion != 0", deploymentConfigID)
		}
	} else if !deploy.PodTemplatesEqual(configPodTemplate, deployment.ControllerTemplate.PodTemplate) {
		deploymentConfig.LatestVersion += 1
		glog.Infof("Incrementing deploymentConfig %v LatestVersion: %v", deploymentConfigID, deploymentConfig.LatestVersion)
	}

	return deploymentConfig, nil
}

func (g *deploymentConfigGenerator) imageRepos() map[string]imageapi.ImageRepository {
	repos := make(map[string]imageapi.ImageRepository)

	imageRepos, err := g.imageRepoRegistry.ListImageRepositories(labels.Everything())
	if err != nil {
		glog.Errorf("Error listing imageRepositories: %#v", err)
		return repos
	}

	for _, repo := range imageRepos.Items {
		repos[repo.DockerImageRepository] = repo
	}

	return repos
}
