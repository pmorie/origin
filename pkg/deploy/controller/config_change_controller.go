package controller

import (
  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  cache "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  util "github.com/GoogleCloudPlatform/kubernetes/pkg/util"

  deployapi "github.com/openshift/origin/pkg/deploy/api"
  deployutil "github.com/openshift/origin/pkg/deploy/util"

  "github.com/golang/glog"
)

// ConfigChangeController watches for changes to DeploymentConfigs and regenerates them only
// when detecting a change to the PodTemplate of a DeploymentConfig containing a ConfigChange
// trigger.
type ConfigChangeController struct {
  DeploymentConfigInterface deploymentConfigInterface
  NextDeploymentConfig      func() *deployapi.DeploymentConfig
  DeploymentStore           cache.Store
}

type deploymentConfigInterface interface {
  GenerateDeploymentConfig(kapi.Context, string) (*deployapi.DeploymentConfig, error)
  UpdateDeploymentConfig(kapi.Context, *deployapi.DeploymentConfig) (*deployapi.DeploymentConfig, error)
}

// Run watches for config change events.
func (dc *ConfigChangeController) Run() {
  go util.Forever(func() { dc.HandleDeploymentConfig() }, 0)
}

// HandleDeploymentConfig handles the next DeploymentConfig change that happens.
func (dc *ConfigChangeController) HandleDeploymentConfig() error {
  config := dc.NextDeploymentConfig()

  hasChangeTrigger := false
  for _, trigger := range config.Triggers {
    if trigger.Type == deployapi.DeploymentTriggerOnConfigChange {
      hasChangeTrigger = true
      break
    }
  }

  if !hasChangeTrigger {
    glog.V(4).Infof("Config has no change trigger; skipping")
    return nil
  }

  if config.LatestVersion == 0 {
    glog.V(4).Info("Ignoring config change with LatestVersion=0")
    return nil
  }

  latestDeploymentId := deployutil.LatestDeploymentIDForConfig(config)
  obj, exists := dc.DeploymentStore.Get(latestDeploymentId)

  if !exists {
    glog.V(4).Info("Ignoring config change due to lack of existing deployment")
    return nil
  }

  deployment := obj.(*deployapi.Deployment)

  if deployutil.PodTemplatesEqual(config.Template.ControllerTemplate.PodTemplate, deployment.ControllerTemplate.PodTemplate) {
    glog.V(4).Infof("Ignoring updated config %s with LatestVersion=%d because it matches deployment %s", config.ID, config.LatestVersion, deployment.ID)
    return nil
  }

  ctx := kapi.WithNamespace(kapi.NewContext(), config.Namespace)
  newConfig, err := dc.DeploymentConfigInterface.GenerateDeploymentConfig(ctx, config.ID)
  if err != nil {
    glog.V(2).Infof("Error generating new version of deploymentConfig %v", config.ID)
    return err
  }

  glog.V(4).Infof("Updating config %s (LatestVersion: %d -> %d) to advance existing deployment %s", config.ID, config.LatestVersion, newConfig.LatestVersion, deployment.ID)

  _, err = dc.DeploymentConfigInterface.UpdateDeploymentConfig(ctx, newConfig)
  if err != nil {
    glog.V(2).Infof("Error updating deploymentConfig %v", config.ID)
    return err
  }

  return nil
}
