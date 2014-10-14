package controller

import (
  "testing"

  kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
  kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
  osclient "github.com/openshift/origin/pkg/client"
  deployapi "github.com/openshift/origin/pkg/deploy/api"
)

func basicDeployment() *deployapi.Deployment {
  return &deployapi.Deployment{
    JSONBase: kapi.JSONBase{ID: "deploy1"},
    State:    deployapi.DeploymentStateNew,
    Strategy: deployapi.DeploymentStrategy{
      Type: "customPod",
      CustomPod: &deployapi.CustomPodDeploymentStrategy{
        Image:       "registry:8080/repo1:ref1",
        Environment: []kapi.EnvVar{},
      },
    },
    ControllerTemplate: kapi.ReplicationControllerState{
      PodTemplate: kapi.PodTemplate{
        DesiredState: kapi.PodState{
          Manifest: kapi.ContainerManifest{
            Containers: []kapi.Container{
              {
                Name:  "container1",
                Image: "registry:8080/repo1:ref1",
              },
            },
          },
        },
      },
    },
  }
}

type FakeKubeClient struct {
  kclient.Fake
  Pod   *kapi.Pod
  Error error
}

func (c *FakeKubeClient) GetPod(ctx kapi.Context, name string) (*kapi.Pod, error) {
  return c.Pod, c.Error
}

func (c *FakeKubeClient) CreatePod(ctx kapi.Context, pod *kapi.Pod) (*kapi.Pod, error) {
  c.Actions = append(c.Actions, kclient.FakeAction{Action: "create-pod", Value: pod})
  return pod, nil
}

type dcTestHelper struct {
  OsClient   *osclient.Fake
  KubeClient *FakeKubeClient
  Deployment *deployapi.Deployment
  Controller *DeploymentController
}

func newDCTestHelper() *dcTestHelper {
  osClient := &osclient.Fake{}
  kClient := &FakeKubeClient{}

  deployment := basicDeployment()

  config := &DeploymentControllerConfig{
    OsClient:    osClient,
    KubeClient:  kClient,
    Environment: []kapi.EnvVar{},
    NextDeployment: func() *deployapi.Deployment {
      return deployment
    },
  }

  return &dcTestHelper{
    OsClient:   osClient,
    KubeClient: kClient,
    Deployment: deployment,
    Controller: NewDeploymentController(config),
  }
}

func TestHandleNewDeployment(t *testing.T) {
  helper := newDCTestHelper()

  // Verify new -> pending
  helper.Controller.HandleDeployment()

  // TODO: stronger assertions on the actual pod
  if e, a := "create-pod", helper.KubeClient.Actions[0].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }

  if e, a := deployapi.DeploymentStatePending, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).State; e != a {
    t.Fatalf("expected deployment state %s, got %s", e, a)
  }
}

func TestHandlePendingDeploymentPendingPod(t *testing.T) {
  helper := newDCTestHelper()

  // Verify pending -> pending given the pod isn't yet running
  helper.Deployment.State = deployapi.DeploymentStatePending
  helper.KubeClient.Pod = &kapi.Pod{
    CurrentState: kapi.PodState{
      Status: kapi.PodWaiting,
    },
  }

  helper.Controller.HandleDeployment()

  if len(helper.OsClient.Actions) != 0 {
    t.Fatalf("expected no client actions, found %v", helper.OsClient.Actions)
  }
}

func TestHandlePendingDeploymentRunningPod(t *testing.T) {
  helper := newDCTestHelper()

  // Verify pending -> running now that the pod is running
  helper.Deployment.State = deployapi.DeploymentStatePending
  helper.KubeClient.Pod = &kapi.Pod{
    CurrentState: kapi.PodState{
      Status: kapi.PodRunning,
    },
  }

  helper.Controller.HandleDeployment()

  if e, a := deployapi.DeploymentStateRunning, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).State; e != a {
    t.Fatalf("expected deployment state %s, got %s", e, a)
  }
}

func TestHandleRunningDeploymentRunningPod(t *testing.T) {
  helper := newDCTestHelper()

  // Verify running -> running as the pod is still running
  helper.Deployment.State = deployapi.DeploymentStateRunning
  helper.KubeClient.Pod = &kapi.Pod{
    CurrentState: kapi.PodState{
      Status: kapi.PodRunning,
    },
  }

  helper.Controller.HandleDeployment()

  if len(helper.OsClient.Actions) != 0 {
    t.Fatalf("expected no client actions, found %v", helper.OsClient.Actions)
  }
}

func TestHandleRunningDeploymentTerminatedOkPod(t *testing.T) {
  helper := newDCTestHelper()

  // Verify running -> complete as the pod terminated successfully
  helper.Deployment.State = deployapi.DeploymentStateRunning
  helper.KubeClient.Pod = &kapi.Pod{
    CurrentState: kapi.PodState{
      Status: kapi.PodTerminated,
      Info: kapi.PodInfo{
        "container1": kapi.ContainerStatus{
          State: kapi.ContainerState{
            Termination: &kapi.ContainerStateTerminated{
              ExitCode: 0,
            },
          },
        },
      },
    },
  }

  helper.Controller.HandleDeployment()

  if e, a := deployapi.DeploymentStateComplete, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).State; e != a {
    t.Fatalf("expected deployment state %s, got %s", e, a)
  }

  // ensure the pod was cleaned up
  if e, a := "delete-pod", helper.KubeClient.Actions[0].Action; e != a {
    t.Fatalf("expected %s action, got %s", e, a)
  }
}

func TestHandleRunningDeploymentTerminatedFailPod(t *testing.T) {
  helper := newDCTestHelper()

  // Verify running -> complete as the pod terminated successfully
  helper.Deployment.State = deployapi.DeploymentStateRunning
  helper.KubeClient.Pod = &kapi.Pod{
    CurrentState: kapi.PodState{
      Status: kapi.PodTerminated,
      Info: kapi.PodInfo{
        "container1": kapi.ContainerStatus{
          State: kapi.ContainerState{
            Termination: &kapi.ContainerStateTerminated{
              ExitCode: 1,
            },
          },
        },
      },
    },
  }

  helper.Controller.HandleDeployment()

  if e, a := deployapi.DeploymentStateFailed, helper.OsClient.Actions[0].Value.(*deployapi.Deployment).State; e != a {
    t.Fatalf("expected deployment state %s, got %s", e, a)
  }

  for _, a := range helper.OsClient.Actions {
    if a.Action == "delete-pod" {
      t.Fatalf("unexpected call to delete-pod")
    }
  }
}
