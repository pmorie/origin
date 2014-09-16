package trigger

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployutil "github.com/openshift/origin/pkg/deploy/util"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// Image repo triggers
type ImageRepoTriggers struct {
	reposToConfigs map[string]util.StringSet
	configsToRepos map[string]util.StringSet
}

func NewImageRepoTriggers() ImageRepoTriggers {
	return ImageRepoTriggers{
		make(map[string]util.StringSet),
		make(map[string]util.StringSet),
	}
}

func (t *ImageRepoTriggers) Insert(configID string, repoIDs util.StringSet) {
	for _, repoID := range repoIDs.List() {
		configs, ok := t.reposToConfigs[repoID]
		if !ok {
			configs = util.StringSet{}
		}
		configs.Insert(configID)
		t.reposToConfigs[repoID] = configs

		repos, ok := t.configsToRepos[configID]
		if !ok {
			repos = util.StringSet{}
		}
		repos.Insert(repoID)
		t.configsToRepos[configID] = repos
	}
}

func (t *ImageRepoTriggers) ConfigsForRepo(id string) util.StringSet {
	return t.reposToConfigs[id]
}

func (t *ImageRepoTriggers) ReposForConfig(id string) util.StringSet {
	return t.configsToRepos[id]
}

func (t *ImageRepoTriggers) Remove(configID string, repoIDs util.StringSet) {
	referencedRepos := t.configsToRepos[configID]

	for _, repoID := range repoIDs.List() {
		configs := t.reposToConfigs[repoID]

		configs.Delete(configID)
		if len(configs) == 0 {
			delete(t.reposToConfigs, repoID)
		}

		referencedRepos.Delete(repoID)
		if len(referencedRepos) == 0 {
			delete(t.configsToRepos, configID)
		}
	}
}

func (t *ImageRepoTriggers) HasRegisteredTriggers(repo *imageapi.ImageRepository) bool {
	id := repo.DockerImageRepository

	if _, ok := t.reposToConfigs[id]; ok {
		return true
	}

	return false
}

func (t *ImageRepoTriggers) Fire(
	repo *imageapi.ImageRepository,
	config *deployapi.DeploymentConfig,
	deployment *deployapi.Deployment) bool {

	var (
		repoName               = repo.DockerImageRepository
		referencedImageVersion = deployutil.ReferencedImages(deployment)[repo.DockerImageRepository]
		params                 = deployutil.ParamsForImageChangeTrigger(config, repoName)
		latestTagVersion       = repo.Tags[params.Tag]
	)

	return referencedImageVersion != latestTagVersion
}
