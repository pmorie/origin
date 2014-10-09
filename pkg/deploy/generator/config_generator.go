package generator

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployregistry "github.com/openshift/origin/pkg/deploy/registry/deploy"
	deployconfig "github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagerepo "github.com/openshift/origin/pkg/image/registry/imagerepository"
)

type DeploymentConfigGenerator interface {
	Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error)
}

type deploymentConfigGenerator struct {
	deploymentRegistry   deployregistry.Registry
	deployConfigRegistry deployconfig.Registry
	imageRepoRegistry    imagerepo.Registry
}

func NewDeploymentConfigGenerator(deploymentRegistry deployregistry.Registry,
	deployConfigRegistry deployconfig.Registry, imageRepoRegistry imagerepo.Registry) DeploymentConfigGenerator {
	return &deploymentConfigGenerator{
		deploymentRegistry:   deploymentRegistry,
		deployConfigRegistry: deployConfigRegistry,
		imageRepoRegistry:    imageRepoRegistry,
	}
}

func (g *deploymentConfigGenerator) Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	deploymentConfig, err := g.deployConfigRegistry.GetDeploymentConfig(deploymentConfigID)
	if err != nil {
		glog.Errorf("Error getting deploymentConfig for id %v", deploymentConfigID)
		return nil, err
	}

	deploymentID := deploy.LatestDeploymentIDForConfig(deploymentConfig)

	deployment, err := g.deploymentRegistry.GetDeployment(deploymentID)
	if err != nil && !errors.IsNotFound(err) {
		glog.Errorf("Error getting deployment: %#v", err)
		return nil, err
	}

	configPodTemplate := deploymentConfig.Template.ControllerTemplate.PodTemplate

	referencedRepoNames := referencedRepoNames(deploymentConfig)
	referencedRepos := imageReposByDockerImageRepo(g.imageRepoRegistry, referencedRepoNames)

	for _, repoName := range referencedRepoNames.List() {
		params := deploy.ParamsForImageChangeTrigger(deploymentConfig, repoName)
		repo, ok := referencedRepos[params.RepositoryName]
		if !ok {
			return nil, fmt.Errorf("Config references unknown ImageRepository '%s'", params.RepositoryName)
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
		if deploymentConfig.LatestVersion == 0 {
			// TODO: Is this a safe assumption? -- NO
			deploymentConfig.LatestVersion = 1
		}
	} else if !deploy.PodTemplatesEqual(configPodTemplate, deployment.ControllerTemplate.PodTemplate) {
		deploymentConfig.LatestVersion += 1
	}

	return deploymentConfig, nil
}

func imageReposByDockerImageRepo(registry imagerepo.Registry, filter *util.StringSet) map[string]imageapi.ImageRepository {
	repos := make(map[string]imageapi.ImageRepository)

	imageRepos, err := registry.ListImageRepositories(labels.Everything())
	if err != nil {
		glog.Errorf("Error listing imageRepositories: %#v", err)
		return repos
	}

	for _, repo := range imageRepos.Items {
		if filter.Has(repo.DockerImageRepository) {
			repos[repo.DockerImageRepository] = repo
		}
	}

	return repos
}

// Returns the image repositories names a config has triggers registered for
func referencedRepoNames(config *deployapi.DeploymentConfig) *util.StringSet {
	repoIDs := &util.StringSet{}

	if config == nil || config.Triggers == nil {
		return repoIDs
	}

	for _, trigger := range config.Triggers {
		if trigger.Type == deployapi.DeploymentTriggerOnImageChange {
			repoIDs.Insert(trigger.ImageChangeParams.RepositoryName)
		}
	}

	return repoIDs
}
