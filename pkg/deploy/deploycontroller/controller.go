package deploymentcontroller

import (
	"time"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kubeclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// A DeploymentController is responsible for executing Deployment objects stored in etcd
type DeploymentController struct {
	config *Config
}

type Config struct {
	OsClient       osclient.Interface
	KubeClient     kubeclient.Interface
	Environment    []kapi.EnvVar
	NextDeployment func() *deployapi.Deployment
}

// NewDeploymentController creates a new DeploymentController.
func New(config *Config) *DeploymentController {
	return &DeploymentController{
		config: config,
	}
}

// Run begins watching and synchronizing deployment states.
func (dc *DeploymentController) Run(period time.Duration) {
	go util.Forever(func() { dc.HandleDeployment() }, period)
}

// Invokes the appropriate handler for the current state of the given deployment.
func (dc *DeploymentController) HandleDeployment() error {
	deployment := dc.config.NextDeployment()
	ctx := kapi.WithNamespace(kapi.NewContext(), deployment.Namespace)
	glog.Infof("Synchronizing deployment id: %v state: %v resourceVersion: %v", deployment.ID, deployment.State, deployment.ResourceVersion)

	nextState := deployment.State
	switch deployment.State {
	case deployapi.DeploymentStateNew:
		nextState = dc.handleNew(ctx, deployment)
	case deployapi.DeploymentStatePending:
		nextState = dc.handlePending(ctx, deployment)
	case deployapi.DeploymentStateRunning:
		nextState = dc.handleRunning(ctx, deployment)
	}

	if deployment.State != nextState {
		deployment.State = nextState
		return dc.saveDeployment(ctx, deployment)
	} else {
		return nil
	}
}

// Handler for a deployment in the 'new' state.
func (dc *DeploymentController) handleNew(ctx kapi.Context, deployment *deployapi.Deployment) deployapi.DeploymentState {
	nextState := deployment.State

	deploymentPod := dc.makeDeploymentPod(deployment)
	glog.Infof("Attempting to create deployment pod: %+v", deploymentPod)
	if pod, err := dc.config.KubeClient.CreatePod(kapi.NewContext(), deploymentPod); err != nil {
		glog.Warningf("Received error creating pod: %v", err)
		nextState = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Successfully created pod %+v", pod)
		nextState = deployapi.DeploymentStatePending
	}

	return nextState
}

// Handler for a deployment in the 'pending' state
func (dc *DeploymentController) handlePending(ctx kapi.Context, deployment *deployapi.Deployment) deployapi.DeploymentState {
	nextState := deployment.State

	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)

	pod, err := dc.config.KubeClient.GetPod(ctx, podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		nextState = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)

		switch pod.CurrentState.Status {
		case kapi.PodRunning:
			nextState = deployapi.DeploymentStateRunning
		case kapi.PodTerminated:
			nextState = dc.checkForTerminatedDeploymentPod(deployment, pod)
		}
	}

	return nextState
}

// Handler for a deployment in the 'running' state
func (dc *DeploymentController) handleRunning(ctx kapi.Context, deployment *deployapi.Deployment) deployapi.DeploymentState {
	nextState := deployment.State

	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)

	pod, err := dc.config.KubeClient.GetPod(ctx, podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		nextState = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)
		nextState = dc.checkForTerminatedDeploymentPod(deployment, pod)
	}

	return nextState
}

func deploymentPodID(deployment *deployapi.Deployment) string {
	return "deploy-" + deployment.ID
}

func (dc *DeploymentController) checkForTerminatedDeploymentPod(deployment *deployapi.Deployment, pod *kapi.Pod) deployapi.DeploymentState {
	nextState := deployment.State
	if pod.CurrentState.Status != kapi.PodTerminated {
		glog.Infof("The deployment has not yet finished. Pod status is %s. Continuing", pod.CurrentState.Status)
		return nextState
	}

	nextState = deployapi.DeploymentStateComplete
	for _, info := range pod.CurrentState.Info {
		if info.State.Termination != nil && info.State.Termination.ExitCode != 0 {
			nextState = deployapi.DeploymentStateFailed
		}
	}

	if nextState == deployapi.DeploymentStateComplete {
		podID := deploymentPodID(deployment)
		glog.Infof("Removing deployment pod for ID %v", podID)
		dc.config.KubeClient.DeletePod(kapi.NewContext(), podID)
	}

	glog.Infof("The deployment pod has finished. Setting deployment state to %s", deployment.State)
	return nextState
}

func (dc *DeploymentController) saveDeployment(ctx kapi.Context, deployment *deployapi.Deployment) error {
	glog.Infof("Saving deployment %v state: %v", deployment.ID, deployment.State)
	_, err := dc.config.OsClient.UpdateDeployment(ctx, deployment)
	if err != nil {
		glog.Errorf("Received error while saving deployment %v: %v", deployment.ID, err)
	}
	return err
}

func (dc *DeploymentController) makeDeploymentPod(deployment *deployapi.Deployment) *kapi.Pod {
	podID := deploymentPodID(deployment)

	envVars := deployment.Strategy.CustomPod.Environment
	envVars = append(envVars, kapi.EnvVar{Name: "KUBERNETES_DEPLOYMENT_ID", Value: deployment.ID})
	for _, env := range dc.config.Environment {
		envVars = append(envVars, env)
	}

	return &kapi.Pod{
		JSONBase: kapi.JSONBase{
			ID: podID,
		},
		DesiredState: kapi.PodState{
			Manifest: kapi.ContainerManifest{
				Version: "v1beta1",
				Containers: []kapi.Container{
					{
						Name:  "deployment",
						Image: deployment.Strategy.CustomPod.Image,
						Env:   envVars,
					},
				},
				RestartPolicy: kapi.RestartPolicy{
					Never: &kapi.RestartPolicyNever{},
				},
			},
		},
	}
}
