package controller

import (
  "testing"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  deploytest "github.com/openshift/origin/pkg/deploy/controller/test"
  imageapi "github.com/openshift/origin/pkg/image/api"
)

type icTestHelper struct {
  Client     *icFakeOsClient
  ImageRepo  *imageapi.ImageRepository
  Controller *ImageChangeTriggerController
}

func newIcTestHelper() *icTestHelper {
  var (
    client = &icFakeOsClient{}
    helper = &icTestHelper{
      Client:    client,
      ImageRepo: originalImageRepo(),
    }
    config = &ImageChangeControllerConfig{
      Client: client,
      NextImageRepository: func() *imageapi.ImageRepository {
        return helper.ImageRepo
      },
      DeploymentConfigStore: deploytest.NewFakeDeploymentConfigStore(imageChangeDeploymentConfig()),
    }
  )
  helper.Controller = NewImageChangeTriggerController(config)
  helper.Client.GenerationResult = regeneratedConfig()

  return helper
}

func TestImageChangeForUnregisteredTag(t *testing.T) {
  helper := newIcTestHelper()
  helper.Controller.OneImageRepo()
  helper.ImageRepo = unregisteredTagUpdate()
  helper.Controller.OneImageRepo()

  if len(helper.Client.Actions) != 0 {
    t.Fatalf("expected no client activity, found: %s", helper.Client.Actions)
  }
}

func TestImageChange(t *testing.T) {
  helper := newIcTestHelper()
  helper.Controller.OneImageRepo()
  helper.ImageRepo = tagUpdate()
  helper.Controller.OneImageRepo()

  if num := len(helper.Client.Actions); num != 2 {
    t.Errorf("Expected 2 actions, got: %v", num)
  }

  if e, a := "generate-deployment-config", helper.Client.Actions[0].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }

  if e, a := "update-deployment-config", helper.Client.Actions[1].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }
}

// Utilities and convenience methods

func originalImageRepo() *imageapi.ImageRepository {
  return &imageapi.ImageRepository{
    JSONBase:              kapi.JSONBase{ID: "test-image-repo"},
    DockerImageRepository: "registry:8080/openshift/test-image",
    Tags: map[string]string{
      "test-tag": "ref-1",
    },
  }
}

func unregisteredTagUpdate() *imageapi.ImageRepository {
  return &imageapi.ImageRepository{
    JSONBase:              kapi.JSONBase{ID: "test-image-repo"},
    DockerImageRepository: "registry:8080/openshift/test-image",
    Tags: map[string]string{
      "test-tag":       "ref-1",
      "other-test-tag": "ref-x",
    },
  }
}

func tagUpdate() *imageapi.ImageRepository {
  return &imageapi.ImageRepository{
    JSONBase:              kapi.JSONBase{ID: "test-image-repo"},
    DockerImageRepository: "registry:8080/openshift/test-image",
    Tags: map[string]string{
      "test-tag": "ref-2",
    },
  }
}

func imageChangeDeploymentConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "image-change-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerOnImageChange,
        ImageChangeParams: &deployapi.DeploymentTriggerImageChangeParams{
          Automatic:      true,
          ContainerNames: []string{"container-1"},
          RepositoryName: "registry:8080/openshift/test-image",
          Tag:            "test-tag",
        },
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

func regeneratedConfig() *deployapi.DeploymentConfig {
  return &deployapi.DeploymentConfig{
    JSONBase: kapi.JSONBase{ID: "image-change-deploy-config"},
    Triggers: []deployapi.DeploymentTriggerPolicy{
      {
        Type: deployapi.DeploymentTriggerOnImageChange,
        ImageChangeParams: &deployapi.DeploymentTriggerImageChangeParams{
          Automatic:      true,
          ContainerNames: []string{"container-1"},
          RepositoryName: "registry:8080/openshift/test-image",
          Tag:            "test-tag",
        },
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

type icFakeOsClient struct {
  osclient.Fake
  GenerationResult *deployapi.DeploymentConfig
  Error            error
}

func (c *icFakeOsClient) GenerateDeploymentConfig(ctx kapi.Context, id string) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "generate-deployment-config", Value: id})
  return c.GenerationResult, c.Error
}

func (c *icFakeOsClient) UpdateDeploymentConfig(ctx kapi.Context, config *deployapi.DeploymentConfig) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "update-deployment-config", Value: config})
  return config, c.Error
}
