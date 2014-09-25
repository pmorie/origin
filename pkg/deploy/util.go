package deploy

import (
	"strings"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// Returns the image repositories names a config has triggers registered for
func ReferencedRepos(config *deployapi.DeploymentConfig) util.StringSet {
	repoIDs := util.StringSet{}

	for _, trigger := range config.Triggers {
		if trigger.Type == deployapi.DeploymentTriggerOnImageChange {
			repoIDs.Insert(trigger.ImageChangeParams.RepositoryName)
		}
	}

	return repoIDs
}

func ParamsForImageChangeTrigger(config *deployapi.DeploymentConfig, repoName string) *deployapi.DeploymentTriggerImageChangeParams {
	for _, trigger := range config.Triggers {
		if trigger.Type == deployapi.DeploymentTriggerOnImageChange && trigger.ImageChangeParams.RepositoryName == repoName {
			return trigger.ImageChangeParams
		}
	}

	return nil
}

// Set a-b
func Difference(a, b util.StringSet) util.StringSet {
	diff := util.StringSet{}

	for _, s := range a.List() {
		if !b.Has(s) {
			diff.Insert(s)
		}
	}

	return diff
}

// Returns a map of referenced image name to image version
func ReferencedImages(deployment *deployapi.Deployment) map[string]string {
	result := make(map[string]string)

	for _, container := range deployment.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
		name, version := ParseContainerImage(container.Image)
		result[name] = version
	}

	return result
}

func ParseContainerImage(image string) (string, string) {
	tokens := strings.Split(image, ":")
	return tokens[0], tokens[1]
}
