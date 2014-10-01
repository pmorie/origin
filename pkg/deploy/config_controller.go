package deploy

import (
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// A DeploymentConfigController is responsible for implementing the triggers registered by DeploymentConfigs
// TODO: needs cache of some kind
type DeploymentConfigController struct {
	osClient          osclient.Interface
	deployConfigWatch watch.Interface
}

// NewDeploymentConfigController creates a new DeploymentConfigController.
func NewDeploymentConfigController(osClient osclient.Interface) *DeploymentConfigController {
	return &DeploymentConfigController{osClient: osClient}
}

func (c *DeploymentConfigController) Run(period time.Duration) {
	go util.Forever(func() { c.runController() }, period)
}

func (c *DeploymentConfigController) runController() {
	glog.Info("Bootstrapping deploymentConfig controller")

	deploymentConfigs, err := c.osClient.ListDeploymentConfigs(labels.Everything())
	if err != nil {
		glog.Errorf("Bootstrap error: %v (%#v)", err, err)
		return
	}

	glog.Info("Determine whether to deploy deploymentConfigs")
	for _, config := range deploymentConfigs.Items {
		c.handle(&config)
	}

	err = c.subscribeToDeploymentConfigs()
	if err != nil {
		glog.Errorf("error subscribing to deploymentConfigs: %v", err)
		return
	}

	go c.watchDeploymentConfigs()

	select {}
}

// TODO: reduce code duplication between trigger and config controllers
func (c *DeploymentConfigController) latestDeploymentForConfig(config *deployapi.DeploymentConfig) (*deployapi.Deployment, error) {
	latestDeploymentId := LatestDeploymentIDForConfig(config)
	deployment, err := c.osClient.GetDeployment(latestDeploymentId)
	if err != nil {
		// TODO: probably some error / race handling to do here
		return nil, err
	}

	return deployment, nil
}

func (c *DeploymentConfigController) handle(config *deployapi.DeploymentConfig) error {
	deploy, err := c.shouldDeploy(config)
	if err != nil {
		// TODO: better error handling
		glog.Errorf("Error determining whether to redeploy deploymentConfig %v: %#v", config.ID, err)
		return err
	}

	if !deploy {
		return nil
	}

	err = c.deploy(config)
	if err != nil {
		return err
	}

	return nil
}

func (c *DeploymentConfigController) shouldDeploy(config *deployapi.DeploymentConfig) (bool, error) {
	if config.LatestVersion == 0 {
		return false, nil
	}

	deployment, err := c.latestDeploymentForConfig(config)
	if err != nil {
		if IsNotFoundError(err) {
			return true, nil
		} else {
			return false, err
		}
	}

	return !PodTemplatesEqual(deployment.ControllerTemplate.PodTemplate, config.Template.ControllerTemplate.PodTemplate), nil
}

func (c *DeploymentConfigController) deploy(config *deployapi.DeploymentConfig) error {
	labels := make(map[string]string)
	for k, v := range config.Labels {
		labels[k] = v
	}
	labels["configID"] = config.ID

	deployment := &deployapi.Deployment{
		JSONBase: api.JSONBase{
			ID: LatestDeploymentIDForConfig(config),
		},
		Labels:             labels,
		Strategy:           config.Template.Strategy,
		ControllerTemplate: config.Template.ControllerTemplate,
	}

	_, err := c.osClient.CreateDeployment(deployment)

	return err
}

func (c *DeploymentConfigController) subscribeToDeploymentConfigs() error {
	glog.Info("Subscribing to deployment configs")
	watch, err := c.osClient.WatchDeploymentConfigs(labels.Everything(), labels.Everything(), 0)
	if err == nil {
		c.deployConfigWatch = watch
	}
	return err
}

func (c *DeploymentConfigController) watchDeploymentConfigs() {
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
			c.handle(config)
		}
	}
}
