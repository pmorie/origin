package controller

import (
  "testing"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  deploytest "github.com/openshift/origin/pkg/deploy/controller/test"
)

type cctcTestHelper struct {
  Client           *FakeOsClient
  DeploymentConfig *deployapi.DeploymentConfig
  Controller       *ConfigChangeTriggerController
}

func newCctcTestHelper() *cctcTestHelper {
  client := &FakeOsClient{}
  helper := &cctcTestHelper{
    Client:           client,
    DeploymentConfig: initialConfig(),
  }
  config := &ConfigChangeTriggerControllerConfig{
    OsClient: client,
    NextDeploymentConfig: func() *deployapi.DeploymentConfig {
      return helper.DeploymentConfig
    },
    DeploymentStore: deploytest.NewFakeDeploymentStore(matchingInitialDeployment()),
  }
  helper.Controller = NewConfigChangeTriggerController(config)

  return helper
}

// Test the controller's response to a new DeploymentConfig
func TestNewConfig(t *testing.T) {
  helper := newCctcTestHelper()
  helper.Controller.HandleDeploymentConfig()

  if len(helper.Client.Actions) != 0 {
    t.Fatalf("expected no client activity, found: %s", helper.Client.Actions)
  }
}

// Test the controller's response when the pod template is changed
func TestChangeWithTemplateDiff(t *testing.T) {
  helper := newCctcTestHelper()
  helper.Controller.HandleDeploymentConfig()
  helper.DeploymentConfig = diffedConfig()
  helper.Controller.HandleDeploymentConfig()

  if num := len(helper.Client.Actions); num != 2 {
    t.Errorf("Expected 2 actions, got %v", num)
  }

  if e, a := "generate-deployment-config", helper.Client.Actions[0].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }

  if e, a := "update-deployment-config", helper.Client.Actions[1].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }
}

func TestChangeWithoutTemplateDiff(t *testing.T) {
  helper := newCctcTestHelper()
  helper.Controller.HandleDeploymentConfig()
  helper.Controller.HandleDeploymentConfig()

  if len(helper.Client.Actions) != 0 {
    t.Fatalf("expected no client activity, found: %s", helper.Client.Actions)
  }
}

func initialConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "test-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerOnConfigChange,
      },
    },
    LatestVersion: 2,
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

func diffedConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "test-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerOnConfigChange,
      },
    },
    LatestVersion: 2,
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
          "name": "test-pod-2",
        },
        PodTemplate: kapi.PodTemplate{
          Labels: map[string]string{
            "name": "test-pod-2",
          },
          DesiredState: kapi.PodState{
            Manifest: kapi.ContainerManifest{
              Version: "v1beta1",
              Containers: []kapi.Container{
                {
                  Name:  "container-2",
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

func generatedConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "manual-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerOnConfigChange,
      },
    },
    LatestVersion: 3,
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
                  Image: "registry:8080/openshift/test-image:ref-2",
                },
              },
            },
          },
        },
      },
    },
  }
}

func matchingInitialDeployment() *deployapi.Deployment {
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
  DeploymentConfig *deployapi.DeploymentConfig
  Error            error
}

func (c *FakeOsClient) GenerateDeploymentConfig(ctx kapi.Context, id string) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "generate-deployment-config", Value: id})
  return c.DeploymentConfig, c.Error
}

func (c *FakeOsClient) UpdateDeploymentConfig(ctx kapi.Context, config *deployapi.DeploymentConfig) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "update-deployment-config", Value: config})
  return config, c.Error
}
