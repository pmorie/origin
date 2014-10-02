package deploy

import (
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// Cache of config ID -> deploymentConfig
type deploymentConfigCache struct {
	store map[string]deployapi.DeploymentConfig
}

func newDeploymentConfigCache() deploymentConfigCache {
	return deploymentConfigCache{
		make(map[string]deployapi.DeploymentConfig),
	}
}

func (c *deploymentConfigCache) refreshList(configs *deployapi.DeploymentConfigList) {
	for _, config := range configs.Items {
		c.refresh(&config)
	}
}

func (c *deploymentConfigCache) refresh(config *deployapi.DeploymentConfig) {
	c.store[config.ID] = *config
}

func (c *deploymentConfigCache) delete(config *deployapi.DeploymentConfig) {
	delete(c.store, config.ID)
}

func (c *deploymentConfigCache) cachedConfig(id string) deployapi.DeploymentConfig {
	return c.store[id]
}

// A filter for deployment config IDs
type deploymentConfigTriggers struct {
	util.StringSet
}

func newDeploymentConfigTriggers() deploymentConfigTriggers {
	return deploymentConfigTriggers{util.StringSet{}}
}

func (t *deploymentConfigTriggers) fire(config *deployapi.DeploymentConfig) bool {
	return t.Has(config.ID)
}

// Cache of image repo DockerImageRepository -> latest sha1
type imageRepoCache struct {
	store map[string]imageapi.ImageRepository
}

func newImageRepoCache() imageRepoCache {
	return imageRepoCache{
		make(map[string]imageapi.ImageRepository),
	}
}

func (c *imageRepoCache) refreshList(repos *imageapi.ImageRepositoryList) {
	for _, repo := range repos.Items {
		c.refresh(&repo)
	}
}

func (c *imageRepoCache) refresh(repo *imageapi.ImageRepository) {
	c.store[repo.DockerImageRepository] = *repo
}

func (c *imageRepoCache) delete(repo *imageapi.ImageRepository) {
	delete(c.store, repo.DockerImageRepository)
}

func (c *imageRepoCache) cachedRepo(name string) imageapi.ImageRepository {
	return c.store[name]
}

// Image repo triggers
type imageRepoTriggers struct {
	reposToConfigs map[string]util.StringSet
	configsToRepos map[string]util.StringSet
}

func newImageRepoTriggers() imageRepoTriggers {
	return imageRepoTriggers{
		make(map[string]util.StringSet),
		make(map[string]util.StringSet),
	}
}

func (t *imageRepoTriggers) insert(configID string, repoIDs util.StringSet) {
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

func (t *imageRepoTriggers) configsForRepo(id string) util.StringSet {
	return t.reposToConfigs[id]
}

func (t *imageRepoTriggers) reposForConfig(id string) util.StringSet {
	return t.configsToRepos[id]
}

func (t *imageRepoTriggers) remove(configID string, repoIDs util.StringSet) {
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

func (t *imageRepoTriggers) hasRegisteredTriggers(repo *imageapi.ImageRepository) bool {
	id := repo.DockerImageRepository

	if _, ok := t.reposToConfigs[id]; ok {
		return true
	}

	return false
}

func (t *imageRepoTriggers) fire(
	repo *imageapi.ImageRepository,
	config *deployapi.DeploymentConfig,
	deployment *deployapi.Deployment) bool {

	var (
		repoName               = repo.DockerImageRepository
		referencedImageVersion = ReferencedImages(deployment)[repo.DockerImageRepository]
		params                 = ParamsForImageChangeTrigger(config, repoName)
		latestTagVersion       = repo.Tags[params.Tag]
	)

	return referencedImageVersion != latestTagVersion
}

// A DeploymentTriggerController is responsible for implementing the triggers registered by DeploymentConfigs
type DeploymentTriggerController struct {
	osClient          osclient.Interface
	imageRepoCache    imageRepoCache
	imageRepoTriggers imageRepoTriggers
	imageRepoWatch    watch.Interface
	configCache       deploymentConfigCache
	configTriggers    deploymentConfigTriggers
	deployConfigWatch watch.Interface
}

// NewDeploymentTriggerController creates a new DeploymentTriggerController.
func NewDeploymentTriggerController(osClient osclient.Interface) *DeploymentTriggerController {
	return &DeploymentTriggerController{
		osClient:          osClient,
		imageRepoCache:    newImageRepoCache(),
		imageRepoTriggers: newImageRepoTriggers(),
		configCache:       newDeploymentConfigCache(),
		configTriggers:    newDeploymentConfigTriggers(),
	}
}

func (c *DeploymentTriggerController) Run(period time.Duration) {
	go util.Forever(func() { c.runController() }, period)
}

func (c *DeploymentTriggerController) runController() {
	glog.Info("Bootstrapping deployment trigger controller")

	imageRepos, err := c.osClient.ListImageRepositories(labels.Everything())
	if err != nil {
		glog.Errorf("Bootstrap error: %v (%#v)", err, err)
		return
	}
	c.imageRepoCache.refreshList(imageRepos)

	deploymentConfigs, err := c.osClient.ListDeploymentConfigs(labels.Everything())
	if err != nil {
		glog.Errorf("Bootstrap error: %v (%#v)", err, err)
		return
	}

	glog.Info("Detecting missed triggers")
	for _, config := range deploymentConfigs.Items {
		c.refreshTriggers(&config)

		missed, err := c.detectMissedTrigger(&config)
		if err != nil {
			// TODO: better error handling
			glog.Errorf("Error handling missed trigger for deploymentConfig %v: %v", config.ID, err)
			continue
		}

		if missed {
			err = c.regenerateDeploymentConfig(&config)
			if err != nil {
				continue
			}
		}
	}

	err = c.subscribeToImageRepos()
	if err != nil {
		glog.Errorf("error subscribing to imageRepos: %v", err)
		return
	}

	err = c.subscribeToDeploymentConfigs()
	if err != nil {
		glog.Errorf("error subscribing to deploymentConfigs: %v", err)
		return
	}

	go c.watchDeploymentConfigs()
	go c.watchImageRepositories()

	select {}
}

func (c *DeploymentTriggerController) regenerateDeploymentConfig(config *deployapi.DeploymentConfig) error {
	newConfig, err := c.osClient.GenerateDeploymentConfig(config.ID)
	if err != nil {
		glog.Errorf("Error generating new version of deploymentConfig %v", config.ID)
		return err
	}

	_, err = c.osClient.UpdateDeploymentConfig(newConfig)
	if err != nil {
		glog.Errorf("Error updating deploymentConfig %v", config.ID)
		return err
	}

	return nil
}

func (c *DeploymentTriggerController) latestDeploymentForConfig(config *deployapi.DeploymentConfig) (*deployapi.Deployment, error) {
	latestDeploymentId := LatestDeploymentIDForConfig(config)
	deployment, err := c.osClient.GetDeployment(latestDeploymentId)
	if err != nil {
		// TODO: probably some error / race handling to do here
		return nil, err
	}

	return deployment, nil
}

func (c *DeploymentTriggerController) detectMissedTrigger(config *deployapi.DeploymentConfig) (bool, error) {
	deployment, err := c.latestDeploymentForConfig(config)
	if err != nil {
		// TODO: probably some error / race handling to do here
		return false, err
	}

	// one of two things can trigger a deployment here:
	// 1. one of the referenced image repos may have been updated
	// 2. the config's replicationControllerState may have changed
	if c.detectMissedImageTrigger(config, deployment) {
		return true, nil
	}

	return c.detectMissedConfigTrigger(config, deployment)
}

func (c *DeploymentTriggerController) detectMissedImageTrigger(config *deployapi.DeploymentConfig, deployment *deployapi.Deployment) bool {
	refImageVersions := ReferencedImages(deployment)

	for _, trigger := range config.Triggers {
		if trigger.Type != deployapi.DeploymentTriggerOnImageChange {
			continue
		}

		var (
			repoName = trigger.ImageChangeParams.RepositoryName
			repo     = c.imageRepoCache.cachedRepo(repoName)
			tag      = trigger.ImageChangeParams.Tag
		)
		if repo.Tags[tag] != refImageVersions[repoName] {
			return true
		}
	}

	return false
}

func (c *DeploymentTriggerController) detectMissedConfigTrigger(config *deployapi.DeploymentConfig, deployment *deployapi.Deployment) (bool, error) {
	return PodTemplatesEqual(deployment.ControllerTemplate.PodTemplate, config.Template.ControllerTemplate.PodTemplate), nil
}

func (c *DeploymentTriggerController) subscribeToImageRepos() error {
	glog.Info("Subscribing to image repositories")
	watch, err := c.osClient.WatchImageRepositories(labels.Everything(), labels.Everything(), 0)
	if err == nil {
		c.imageRepoWatch = watch
	}
	return err
}

func (c *DeploymentTriggerController) subscribeToDeploymentConfigs() error {
	glog.Info("Subscribing to deployment configs")
	watch, err := c.osClient.WatchDeploymentConfigs(labels.Everything(), labels.Everything(), 0)
	if err == nil {
		c.deployConfigWatch = watch
	}
	return err
}

func (c *DeploymentTriggerController) watchDeploymentConfigs() {
	configChan := c.deployConfigWatch.ResultChan()

	for {
		select {
		case configEvent, open := <-configChan:
			if !open {
				// watchChannel has been closed, or something else went
				// wrong with our etcd watch call. Let the util.Forever()
				// that called us call us again.
				return
			}

			config, ok := configEvent.Object.(*deployapi.DeploymentConfig)
			if !ok {
				glog.Errorf("Received unexpected object during deploymentConfig watch: %v", configEvent)
				continue
			}

			glog.Infof("Received deploymentConfig watch for ID %v", config.ID)

			if configEvent.Type == watch.Deleted {
				c.configTriggers.Delete(config.ID)
				c.configCache.delete(config)
				// TODO: refresh image repo filter
				continue
			}

			c.configCache.refresh(config)
			c.refreshTriggers(config)

			if c.configTriggers.fire(config) {
				// TODO: generate new deploymentConfig
			}
		}
	}
}

func (c *DeploymentTriggerController) refreshTriggers(config *deployapi.DeploymentConfig) {
	c.refreshImageRepoChangeTriggers(config)
	c.refreshConfigChangeTriggers(config)
}

func (c *DeploymentTriggerController) refreshImageRepoChangeTriggers(config *deployapi.DeploymentConfig) {
	glog.Infof("Refreshing image repo triggers for deploymentConfig %v", config.ID)
	configID := config.ID
	currentRepoIDs := ReferencedRepos(config)

	// Refresh the image repo imageRepoTriggers
	c.imageRepoTriggers.insert(configID, currentRepoIDs)

	// Delete triggers for the removed image repos
	deletedRepoIDs := Difference(c.imageRepoTriggers.reposForConfig(configID), currentRepoIDs)
	c.imageRepoTriggers.remove(configID, deletedRepoIDs)
}

func (c *DeploymentTriggerController) refreshConfigChangeTriggers(config *deployapi.DeploymentConfig) {
	// Update the config change trigger
	configChangeTriggerFound := false
	for _, trigger := range config.Triggers {
		if trigger.Type == deployapi.DeploymentTriggerOnConfigChange {
			configChangeTriggerFound = true
			break
		}
	}

	if configChangeTriggerFound {
		c.configTriggers.Insert(config.ID)
	} else {
		c.configTriggers.Delete(config.ID)
	}
}

func (c *DeploymentTriggerController) watchImageRepositories() {
	imageRepoChan := c.imageRepoWatch.ResultChan()

	for {
		select {
		case imageRepoEvent, open := <-imageRepoChan:
			if !open {
				return
			}

			imageRepo, ok := imageRepoEvent.Object.(*imageapi.ImageRepository)
			if !ok {
				glog.Infof("Received unexpected object during imageRepository watch: %v", imageRepoEvent)
				continue
			}

			glog.Infof("Received imageRepository watch for ID %v", imageRepo.ID)

			if imageRepoEvent.Type == watch.Deleted {
				c.imageRepoCache.delete(imageRepo)
				continue
			}

			if c.imageRepoTriggers.hasRegisteredTriggers(imageRepo) {
				c.handleImageRepoWatch(imageRepo)
			}
		}
	}
}

func (c *DeploymentTriggerController) handleImageRepoWatch(repo *imageapi.ImageRepository) {
	id := repo.ID

	configs := c.imageRepoTriggers.configsForRepo(id)

	for _, configID := range configs.List() {
		config := c.configCache.cachedConfig(configID)
		latestDeployment, err := c.latestDeploymentForConfig(&config)
		if err != nil {
			glog.Errorf("Error finding latest deployment for deploymentConfig %v: %v", configID, err)
			continue
		}

		if c.imageRepoTriggers.fire(repo, &config, latestDeployment) {
			// TODO: generate new deployment config
		}
	}
}
