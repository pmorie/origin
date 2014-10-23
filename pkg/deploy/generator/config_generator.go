package generator

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"

	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployutil "github.com/openshift/origin/pkg/deploy/util"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// DeploymentConfigGenerator reconciles a DeploymentConfig with other pieces of deployment-related state
// and produces a DeploymentConfig which represents a potential future DeploymentConfig. If the generated
// state differs from the input state, the LatestVersion field of the output is incremented.
type DeploymentConfigGenerator struct {
	DeploymentInterface       deploymentInterface
	DeploymentConfigInterface deploymentConfigInterface
	ImageRepositoryInterface  imageRepositoryInterface
}

type deploymentInterface interface {
	GetDeployment(id string) (*deployapi.Deployment, error)
}

type deploymentConfigInterface interface {
	GetDeploymentConfig(id string) (*deployapi.DeploymentConfig, error)
}

type imageRepositoryInterface interface {
	ListImageRepositories(labels labels.Selector) (*imageapi.ImageRepositoryList, error)
}

// Generate returns a potential future DeploymentConfig based on the DeploymentConfig specified
// by deploymentConfigID.
func (g *DeploymentConfigGenerator) Generate(deploymentConfigID string) (*deployapi.DeploymentConfig, error) {
	glog.V(4).Infof("Generating new deployment config from deploymentConfig %v", deploymentConfigID)

	deploymentConfig, err := g.DeploymentConfigInterface.GetDeploymentConfig(deploymentConfigID)
	if err != nil {
		glog.V(4).Infof("Error getting deploymentConfig for id %v", deploymentConfigID)
		return nil, err
	}

	deploymentID := deployutil.LatestDeploymentIDForConfig(deploymentConfig)

	deployment, err := g.DeploymentInterface.GetDeployment(deploymentID)
	if err != nil && !errors.IsNotFound(err) {
		glog.V(2).Infof("Error getting deployment: %#v", err)
		return nil, err
	}

	configPodTemplate := deploymentConfig.Template.ControllerTemplate.PodTemplate

	referencedRepoNames := referencedRepoNames(deploymentConfig)
	referencedRepos := imageReposByDockerImageRepo(g.ImageRepositoryInterface, referencedRepoNames)

	for _, repoName := range referencedRepoNames.List() {
		params := deployutil.ParamsForImageChangeTrigger(deploymentConfig, repoName)
		repo, ok := referencedRepos[params.RepositoryName]
		if !ok {
			return nil, fmt.Errorf("Config references unknown ImageRepository '%s'", params.RepositoryName)
		}

		// TODO: If the tag is missing, what's the correct reaction?
		tag, tagExists := repo.Tags[params.Tag]
		if !tagExists {
			glog.V(4).Infof("No tag %s found for repository %s (potentially invalid DeploymentConfig status)", tag, repoName)
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
	} else if !deployutil.PodTemplatesEqual(configPodTemplate, deployment.ControllerTemplate.PodTemplate) {
		deploymentConfig.LatestVersion += 1
	}

	return deploymentConfig, nil
}

func imageReposByDockerImageRepo(imageRepoInterface imageRepositoryInterface, filter *util.StringSet) map[string]imageapi.ImageRepository {
	repos := make(map[string]imageapi.ImageRepository)

	imageRepos, err := imageRepoInterface.ListImageRepositories(labels.Everything())
	if err != nil {
		glog.V(2).Infof("Error listing imageRepositories: %#v", err)
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
