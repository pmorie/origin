package controller

import (
	"errors"
	"strings"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

type Config struct {
	Client osclient.Interface

	NextImageRepository func() *imageapi.ImageRepository

	DeploymentConfigStore cache.Store
}

// A DeploymentTriggerController is responsible for implementing the triggers registered by DeploymentConfigs
type DeploymentTriggerController struct {
	config *Config
}

// NewDeploymentTriggerController creates a new DeploymentTriggerController.
func New(config *Config) *DeploymentTriggerController {
	return &DeploymentTriggerController{
		config: config,
	}
}

func (c *DeploymentTriggerController) Run() {
	go util.Forever(c.OneImageRepo, 0)
}

func (c *DeploymentTriggerController) OneImageRepo() {
	imageRepo := c.config.NextImageRepository()
	configIDs := []string{}

	for _, c := range c.config.DeploymentConfigStore.List() {
		config := c.(*deployapi.DeploymentConfig)

		for _, params := range configImageTriggers(config) {
			for _, container := range config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers {
				repoName, tag := parseImage(container.Image)
				if repoName != params.RepositoryName {
					continue
				}

				if tag != imageRepo.Tags[params.Tag] {
					configIDs = append(configIDs, config.ID)
				}
			}
		}
	}

	for _, configID := range configIDs {
		err := c.regenerate(kapi.NewContext(), configID)
		if err != nil {
			glog.Infof("Error regenerating deploymentConfig %v: %v", configID, err)
		}
	}
}

func parseImage(name string) (string, string) {
	split := strings.Split(name, ":")
	if len(split) != 2 {
		return "", ""
	}

	return split[0], split[1]
}

func configImageTriggers(config *deployapi.DeploymentConfig) []deployapi.DeploymentTriggerImageChangeParams {
	res := []deployapi.DeploymentTriggerImageChangeParams{}

	for _, trigger := range config.Triggers {
		if trigger.Type != deployapi.DeploymentTriggerOnImageChange {
			continue
		}

		if !trigger.ImageChangeParams.Automatic {
			continue
		}

		res = append(res, *trigger.ImageChangeParams)
	}

	return res
}

func (c *DeploymentTriggerController) regenerate(ctx kapi.Context, configID string) error {
	newConfig, err := c.config.Client.GenerateDeploymentConfig(ctx, configID)
	if err != nil {
		glog.Errorf("Error generating new version of deploymentConfig %v", configID)
		return err
	}

	if newConfig == nil {
		glog.Errorf("Generator returned nil for config %s", configID)
		return errors.New("Generator returned nil")
	}

	ctx = kapi.WithNamespace(ctx, newConfig.Namespace)
	_, err = c.config.Client.UpdateDeploymentConfig(ctx, newConfig)
	if err != nil {
		glog.Errorf("Error updating deploymentConfig %v", configID)
		return err
	}

	return nil
}
