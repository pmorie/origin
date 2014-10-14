package configcontroller

import (
  "testing"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  kerrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
)

func manualDeploymentConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "manual-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerManual,
      },
    },
    Template: deployapi.DeploymentTemplate{
      Strategy: deployapi.DeploymentStrategy{
        Type: "customPod",
        CustomPod: &deployapi.CustomPodDeploymentStrategy{
          Image: "registry:8080/openshift/kube-deploy",
        },
      },
      ControllerTemplate: kapi.ReplicationControllerState{
        Replicas: 1,
        ReplicaSelector: map[string]string{
          "name": "test-pod",
        },
        PodTemplate: kapi.PodTemplate{
          Labels: map[string]string{
            "name": "test-pod",
          },
          DesiredState: kapi.PodState{
            Manifest: kapi.ContainerManifest{
              Version: "v1beta1",
              Containers: []kapi.Container{
                {
                  Name:  "container-1",
                  Image: "registry:8080/openshift/test-image:ref-1",
                },
              },
            },
          },
        },
      },
    },
  }
}

func matchingDeployment() *deployapi.Deployment {
  return &deployapi.Deployment{
    JSONBase: kapi.JSONBase{ID: "manual-deploy-config-1"},
    State:    deployapi.DeploymentStateNew,
    Strategy: deployapi.DeploymentStrategy{
      Type: "customPod",
      CustomPod: &deployapi.CustomPodDeploymentStrategy{
        Image:       "registry:8080/repo1:ref1",
        Environment: []kapi.EnvVar{},
      },
    },
    ControllerTemplate: kapi.ReplicationControllerState{
      Replicas: 1,
      ReplicaSelector: map[string]string{
        "name": "test-pod",
      },
      PodTemplate: kapi.PodTemplate{
        Labels: map[string]string{
          "name": "test-pod",
        },
        DesiredState: kapi.PodState{
          Manifest: kapi.ContainerManifest{
            Version: "v1beta1",
            Containers: []kapi.Container{
              {
                Name:  "container-1",
                Image: "registry:8080/openshift/test-image:ref-1",
              },
            },
          },
        },
      },
    },
  }
}

type FakeOsClient struct {
  osclient.Fake
  Deployment *deployapi.Deployment
  Error      error
}

func (c *FakeOsClient) GetDeployment(ctx kapi.Context, id string) (*deployapi.Deployment, error) {
  return c.Deployment, c.Error
}

func (c *FakeOsClient) CreateDeployment(ctx kapi.Context, deployment *deployapi.Deployment) (*deployapi.Deployment, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "create-deployment", Value: deployment})
  return deployment, c.Error
}

type TestHelper struct {
  OsClient                   *FakeOsClient
  DeploymentConfig           *deployapi.DeploymentConfig
  DeploymentConfigController *DeploymentConfigController
}

func NewTestHelper() *TestHelper {
  osClient := &FakeOsClient{}

  deploymentConfig := manualDeploymentConfig()

  config := &Config{
    Client: osClient,
    NextDeploymentConfig: func() *deployapi.DeploymentConfig {
      return deploymentConfig
    },
  }

  return &TestHelper{
    OsClient:                   osClient,
    DeploymentConfig:           deploymentConfig,
    DeploymentConfigController: New(config),
  }
}

func TestHandleNewDeploymentConfig(t *testing.T) {
  helper := NewTestHelper()

  helper.DeploymentConfig.LatestVersion = 0

  helper.DeploymentConfigController.HandleDeploymentConfig()

  if len(helper.OsClient.Actions) != 0 {
    t.Fatalf("expected no client activity, found: %s", helper.OsClient.Actions)
  }
}

func TestHandleInitialDeployment(t *testing.T) {
  helper := NewTestHelper()

  helper.DeploymentConfig.LatestVersion = 1
  helper.OsClient.Error = kerrors.NewNotFound("deployment", "id")

  helper.DeploymentConfigController.HandleDeploymentConfig()

  if e, a := helper.DeploymentConfig.ID, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).Labels[deployapi.DeploymentConfigIDLabel]; e != a {
    t.Fatalf("expected deployment with label %s, got %s", e, a)
  }
}

func TestHandleConfigChangeNoPodTemplateDiff(t *testing.T) {
  helper := NewTestHelper()

  helper.DeploymentConfig.LatestVersion = 1
  helper.OsClient.Deployment = matchingDeployment()

  // verify that no new deployment was made due to a lack
  // of differences in the pod templates
  helper.DeploymentConfigController.HandleDeploymentConfig()

  for _, a := range helper.OsClient.Actions {
    if a.Action == "create-deployment" {
      t.Fatalf("unexpected call to create-deployment")
    }
  }
}

func TestHandleConfigChangeWithPodTemplateDiff(t *testing.T) {
  helper := NewTestHelper()

  helper.DeploymentConfig.LatestVersion = 1
  helper.OsClient.Deployment = matchingDeployment()
  helper.DeploymentConfig.Template.ControllerTemplate.PodTemplate.Labels["foo"] = "bar"

  // verify that a new deployment results from the change in config
  helper.DeploymentConfigController.HandleDeploymentConfig()

  if e, a := helper.DeploymentConfig.ID, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).Labels[deployapi.DeploymentConfigIDLabel]; e != a {
    t.Fatalf("expected deployment with label %s, got %s", e, a)
  }
}
