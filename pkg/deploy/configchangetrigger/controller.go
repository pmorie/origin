package configchangetrigger

import (
  "time"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  cache "github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
  util "github.com/GoogleCloudPlatform/kubernetes/pkg/util"

  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  deployutil "github.com/openshift/origin/pkg/deploy/util"

  "github.com/golang/glog"
)

type ConfigChangeTriggerController struct {
  config *Config
}

type Config struct {
  OsClient             osclient.Interface
  NextDeploymentConfig func() *deployapi.DeploymentConfig
  DeploymentStore      cache.Store
}

func New(config *Config) *ConfigChangeTriggerController {
  return &ConfigChangeTriggerController{
    config: config,
  }
}

func (dc *ConfigChangeTriggerController) Run(period time.Duration) {
  go util.Forever(func() { dc.HandleDeploymentConfig() }, period)
}

func (dc *ConfigChangeTriggerController) HandleDeploymentConfig() error {
  config := dc.config.NextDeploymentConfig()

  hasChangeTrigger := false
  for _, trigger := range config.Triggers {
    if trigger.Type == deployapi.DeploymentTriggerOnConfigChange {
      hasChangeTrigger = true
      break
    }
  }

  if !hasChangeTrigger {
    return nil
  }

  latestDeploymentId := deployutil.LatestDeploymentIDForConfig(config)
  obj, exists := dc.config.DeploymentStore.Get(latestDeploymentId)

  if !exists || !deployutil.PodTemplatesEqual(config.Template.ControllerTemplate.PodTemplate,
    obj.(*deployapi.Deployment).ControllerTemplate.PodTemplate) {
    ctx := kapi.WithNamespace(kapi.NewContext(), config.Namespace)
    newConfig, err := dc.config.OsClient.GenerateDeploymentConfig(ctx, config.ID)
    if err != nil {
      glog.Errorf("Error generating new version of deploymentConfig %v", config.ID)
      return err
    }

    _, err = dc.config.OsClient.UpdateDeploymentConfig(ctx, newConfig)
    if err != nil {
      glog.Errorf("Error updating deploymentConfig %v", config.ID)
      return err
    }
  }

  return nil
}
