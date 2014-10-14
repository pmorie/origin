package imagechangetrigger

import (
  "testing"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  "github.com/GoogleCloudPlatform/kubernetes/pkg/util"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
  imageapi "github.com/openshift/origin/pkg/image/api"
)

type TestHelper struct {
  Client     *FakeOsClient
  ImageRepo  *imageapi.ImageRepository
  Controller *ImageChangeTriggerController
}

func NewTestHelper() *TestHelper {
  var (
    client = &FakeOsClient{}
    helper = &TestHelper{
      Client:    client,
      ImageRepo: originalImageRepo(),
    }
    config = &Config{
      Client: client,
      NextImageRepository: func() *imageapi.ImageRepository {
        return helper.ImageRepo
      },
      DeploymentConfigStore: newFakeStore(),
    }
  )
  helper.Controller = New(config)
  helper.Client.GenerationResult = regeneratedConfig()

  return helper
}

func TestImageChangeForUnregisteredTag(t *testing.T) {
  helper := NewTestHelper()
  helper.Controller.OneImageRepo()
  helper.ImageRepo = unregisteredTagUpdate()
  helper.Controller.OneImageRepo()

  if len(helper.Client.Actions) != 0 {
    t.Fatalf("expected no client activity, found: %s", helper.Client.Actions)
  }
}

func TestImageChange(t *testing.T) {
  helper := NewTestHelper()
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

type FakeOsClient struct {
  osclient.Fake
  GenerationResult *deployapi.DeploymentConfig
  Error            error
}

func (c *FakeOsClient) GenerateDeploymentConfig(ctx kapi.Context, id string) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "generate-deployment-config", Value: id})
  return c.GenerationResult, c.Error
}

func (c *FakeOsClient) UpdateDeploymentConfig(ctx kapi.Context, config *deployapi.DeploymentConfig) (*deployapi.DeploymentConfig, error) {
  c.Actions = append(c.Actions, osclient.FakeAction{Action: "update-deployment-config", Value: config})
  return config, c.Error
}

type fakeStore struct {
  DeploymentConfig *deployapi.DeploymentConfig
}

func newFakeStore() fakeStore {
  return fakeStore{imageChangeDeploymentConfig()}
}

func (s fakeStore) Add(id string, obj interface{})    {}
func (s fakeStore) Update(id string, obj interface{}) {}
func (s fakeStore) Delete(id string)                  {}
func (s fakeStore) List() []interface{} {
  return []interface{}{s.DeploymentConfig}
}
func (s fakeStore) Contains() util.StringSet {
  return util.NewStringSet()
}
func (s fakeStore) Get(id string) (item interface{}, exists bool) {
  return nil, false
}
func (s fakeStore) Replace(idToObj map[string]interface{}) {}
