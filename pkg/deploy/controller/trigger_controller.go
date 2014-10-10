package controller

import (
	"errors"
	"sync"
	"time"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deploytrigger "github.com/openshift/origin/pkg/deploy/trigger"
	deployutil "github.com/openshift/origin/pkg/deploy/util"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// TODO: refactor to upstream client/cache Reflector/Store pattern

// Cache of config ID -> deploymentConfig
type deploymentConfigCache struct {
	store map[string]deployapi.DeploymentConfig
	sync.Mutex
}

func newDeploymentConfigCache() deploymentConfigCache {
	return deploymentConfigCache{
		store: make(map[string]deployapi.DeploymentConfig),
	}
}

func (c *deploymentConfigCache) refreshList(configs *deployapi.DeploymentConfigList) {
	for _, config := range configs.Items {
		c.refresh(&config)
	}
}

// Returns true if the version changed
func (c *deploymentConfigCache) refresh(config *deployapi.DeploymentConfig) bool {
	c.Lock()
	defer c.Unlock()
	currentConfig, ok := c.store[config.ID]
	c.store[config.ID] = *config

	return !ok || config.LatestVersion != currentConfig.LatestVersion
}

func (c *deploymentConfigCache) delete(config *deployapi.DeploymentConfig) {
	c.Lock()
	defer c.Unlock()
	delete(c.store, config.ID)
}

func (c *deploymentConfigCache) cachedConfig(id string) deployapi.DeploymentConfig {
	c.Lock()
	defer c.Unlock()
	return c.store[id]
}

// A cache of DockerImageRepository -> ImageRepository
type imageRepoCache struct {
	store map[string]imageapi.ImageRepository
	sync.Mutex
}

func newImageRepoCache() imageRepoCache {
	return imageRepoCache{
		store: make(map[string]imageapi.ImageRepository),
	}
}

func (c *imageRepoCache) refreshList(repos *imageapi.ImageRepositoryList) {
	for _, repo := range repos.Items {
		c.refresh(&repo)
	}
}

func (c *imageRepoCache) refresh(repo *imageapi.ImageRepository) {
	c.Lock()
	defer c.Unlock()
	c.store[repo.DockerImageRepository] = *repo
}

func (c *imageRepoCache) delete(repo *imageapi.ImageRepository) {
	c.Lock()
	defer c.Unlock()
	delete(c.store, repo.DockerImageRepository)
}

func (c *imageRepoCache) cachedRepo(name string) imageapi.ImageRepository {
	c.Lock()
	defer c.Unlock()
	return c.store[name]
}

// A DeploymentTriggerController is responsible for implementing the triggers registered by DeploymentConfigs
type DeploymentTriggerController struct {
	osClient          osclient.Interface
	imageRepoCache    imageRepoCache
	imageRepoTriggers deploytrigger.ImageRepoTriggers
	imageRepoWatch    watch.Interface
	configCache       deploymentConfigCache
	configTriggers    deploytrigger.DeploymentConfigTriggers
	deployConfigWatch watch.Interface
	shutdown          chan struct{}
}

// NewDeploymentTriggerController creates a new DeploymentTriggerController.
func NewDeploymentTriggerController(osClient osclient.Interface) *DeploymentTriggerController {
	return &DeploymentTriggerController{
		osClient:          osClient,
		imageRepoCache:    newImageRepoCache(),
		imageRepoTriggers: deploytrigger.NewImageRepoTriggers(),
		configCache:       newDeploymentConfigCache(),
		configTriggers:    deploytrigger.NewDeploymentConfigTriggers(),
	}
}

func (c *DeploymentTriggerController) Run(period time.Duration) {
	go util.Forever(func() { c.SyncDeploymentTriggers() }, period)
}

func (c *DeploymentTriggerController) Shutdown() {
	close(c.shutdown)
}

func (c *DeploymentTriggerController) SyncDeploymentTriggers() {
	ctx := kapi.NewContext()
	glog.Info("Bootstrapping deployment trigger controller")

	c.shutdown = make(chan struct{})

	imageRepos, err := c.osClient.ListImageRepositories(ctx, labels.Everything())
	if err != nil {
		glog.Errorf("Bootstrap error: %v (%#v)", err, err)
		return
	}
	c.imageRepoCache.refreshList(imageRepos)

	deploymentConfigs, err := c.osClient.ListDeploymentConfigs(ctx, labels.Everything())
	if err != nil {
		glog.Errorf("Bootstrap error: %v (%#v)", err, err)
		return
	}

	glog.Info("Detecting missed triggers")
	for _, config := range deploymentConfigs.Items {
		c.refreshTriggers(&config)

		missed, err := c.detectMissedTrigger(ctx, &config)
		if err != nil {
			// TODO: better error handling
			glog.Errorf("Error handling missed trigger for deploymentConfig %v: %v", config.ID, err)
			continue
		}

		if missed {
			err = c.regenerate(ctx, config.ID)
			if err != nil {
				continue
			}
		}
	}

	err = c.subscribeToImageRepos(ctx)
	if err != nil {
		glog.Errorf("error subscribing to imageRepos: %v", err)
		return
	}

	err = c.subscribeToDeploymentConfigs(ctx)
	if err != nil {
		glog.Errorf("error subscribing to deploymentConfigs: %v", err)
		return
	}

	go c.watchDeploymentConfigs(ctx)
	go c.watchImageRepositories(ctx)

	<-c.shutdown
}

func (c *DeploymentTriggerController) regenerate(ctx kapi.Context, configID string) error {
	config := c.configCache.cachedConfig(configID)
	if config.LatestVersion == 0 {
		glog.Infof("Ignoring regeneration for deploymentConfig %v because LatestVersion=0", configID)
		return nil
	}
	newConfig, err := c.osClient.GenerateDeploymentConfig(ctx, configID)
	if err != nil {
		glog.Errorf("Error generating new version of deploymentConfig %v", configID)
		return err
	}

	if newConfig == nil {
		glog.Errorf("Generator returned nil for config %s", configID)
		return errors.New("Generator returned nil")
	}

	_, err = c.osClient.UpdateDeploymentConfig(ctx, newConfig)
	if err != nil {
		glog.Errorf("Error updating deploymentConfig %v", configID)
		return err
	}

	return nil
}

func (c *DeploymentTriggerController) latestDeploymentForConfig(ctx kapi.Context, config *deployapi.DeploymentConfig) (*deployapi.Deployment, error) {
	latestDeploymentId := deployutil.LatestDeploymentIDForConfig(config)
	deployment, err := c.osClient.GetDeployment(ctx, latestDeploymentId)
	if err != nil {
		// TODO: probably some error / race handling to do here
		return nil, err
	}

	return deployment, nil
}

func (c *DeploymentTriggerController) detectMissedTrigger(ctx kapi.Context, config *deployapi.DeploymentConfig) (bool, error) {
	deployment, err := c.latestDeploymentForConfig(ctx, config)
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
	refImageVersions := deployutil.ReferencedImages(deployment)

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
	return deployutil.PodTemplatesEqual(deployment.ControllerTemplate.PodTemplate, config.Template.ControllerTemplate.PodTemplate), nil
}

func (c *DeploymentTriggerController) subscribeToImageRepos(ctx kapi.Context) error {
	glog.Info("Subscribing to image repositories")
	watch, err := c.osClient.WatchImageRepositories(ctx, labels.Everything(), labels.Everything(), 0)
	if err == nil {
		c.imageRepoWatch = watch
	}
	return err
}

func (c *DeploymentTriggerController) subscribeToDeploymentConfigs(ctx kapi.Context) error {
	glog.Info("Subscribing to deployment configs")
	watch, err := c.osClient.WatchDeploymentConfigs(ctx, labels.Everything(), labels.Everything(), 0)
	if err == nil {
		c.deployConfigWatch = watch
	}
	return err
}

func (c *DeploymentTriggerController) watchDeploymentConfigs(ctx kapi.Context) {
	configChan := c.deployConfigWatch.ResultChan()

	for {
		select {
		case <-c.shutdown:
			return
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

			c.refreshTriggers(config)
			versionChanged := c.configCache.refresh(config)
			if versionChanged {
				glog.Infof("Ignoring deploymentConfig watch for ID: %v because LatestVersion changed:", config.ID)
				continue
			}

			if c.configTriggers.Fire(config) {
				glog.Infof("regenerating deploymentConfig %v", config.ID)
				err := c.regenerate(ctx, config.ID)
				if err != nil {
					glog.Infof("Error generating new deploymentConfig for id %v: %v", config.ID, err)
					continue
				}
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
	currentRepoIDs := reposWithAutomaticTriggers(config)

	glog.Infof("deploymentConfig %v references imageRepositories %v", configID, currentRepoIDs)

	// Refresh the image repo imageRepoTriggers
	c.imageRepoTriggers.Insert(configID, currentRepoIDs)

	// Delete triggers for the removed image repos
	deletedRepoIDs := deployutil.Difference(c.imageRepoTriggers.ReposForConfig(configID), currentRepoIDs)
	c.imageRepoTriggers.Remove(configID, deletedRepoIDs)
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

func (c *DeploymentTriggerController) watchImageRepositories(ctx kapi.Context) {
	imageRepoChan := c.imageRepoWatch.ResultChan()

	for {
		select {
		case <-c.shutdown:
			return
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

			if c.imageRepoTriggers.HasRegisteredTriggers(imageRepo) {
				c.handleImageRepoWatch(ctx, imageRepo)
			} else {
				glog.Infof("Repository %v has no registered triggers, skipping")
			}
		}
	}
}

func (c *DeploymentTriggerController) handleImageRepoWatch(ctx kapi.Context, repo *imageapi.ImageRepository) {
	id := repo.DockerImageRepository
	glog.Infof("Handling triggers for imageRepository %#v:", repo)
	configs := c.imageRepoTriggers.ConfigsForRepo(id)
	glog.Infof("configs: %v", configs)
	for _, configID := range configs.List() {
		// TODO: handle not-in-cache error
		config := c.configCache.cachedConfig(configID)
		latestDeployment, err := c.latestDeploymentForConfig(ctx, &config)
		if err != nil {
			glog.Errorf("Error finding latest deployment for deploymentConfig %v: %v", configID, err)
			continue
		}

		if c.imageRepoTriggers.Fire(repo, &config, latestDeployment) {
			glog.Infof("Regeneratoring deploymentConfig %v", configID)
			err := c.regenerate(ctx, configID)
			if err != nil {
				glog.Infof("Error generating new deploymentConfig for id %v: %v", configID, err)
				continue
			}
		}
	}
}

// Returns the image repositories names a config has triggers registered for
// and which are set to Automatic.
func reposWithAutomaticTriggers(config *deployapi.DeploymentConfig) util.StringSet {
	repoIDs := util.StringSet{}

	if config == nil || config.Triggers == nil {
		return repoIDs
	}

	for _, trigger := range config.Triggers {
		if trigger.Type == deployapi.DeploymentTriggerOnImageChange && trigger.ImageChangeParams.Automatic {
			repoIDs.Insert(trigger.ImageChangeParams.RepositoryName)
		}
	}

	return repoIDs
}
