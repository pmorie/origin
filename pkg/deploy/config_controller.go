package deploy

import (
	"time"

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

	err := c.subscribeToDeploymentConfigs()
	if err != nil {
		glog.Errorf("error subscribing to deploymentConfigs: %v", err)
		return
	}

	go c.watchDeploymentConfigs()

	select {}
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

			// TODO:
			// create deployment if the new config doesn't match the old one
		}
	}
}
