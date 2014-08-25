package deploy

import (
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kubeclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// A DeploymentController is responsible for executing Deployment objects stored in etcd
type DeploymentController struct {
	osClient     osclient.Interface
	kubeClient   kubeclient.Interface
	syncTime     <-chan time.Time
	stateHandler DeploymentStateHandler
}

// DeploymentStateHandler holds methods that handle the possible deployment states.
type DeploymentStateHandler interface {
	HandleNew(*deployapi.Deployment) error
	HandlePending(*deployapi.Deployment) error
	HandleRunning(*deployapi.Deployment) error
}

// DefaultDeploymentRunner is the default implementation of DeploymentRunner interface.
type DefaultDeploymentHandler struct {
	osClient    osclient.Interface
	kubeClient  kubeclient.Interface
	environment []api.EnvVar
}

// MakeDeploymentController creates a new DeploymentController.
func MakeDeploymentController(kubeClient kubeclient.Interface, osClient osclient.Interface, initialEnvironment []api.EnvVar) *DeploymentController {
	dc := &DeploymentController{
		kubeClient: kubeClient,
		osClient:   osClient,
		stateHandler: &DefaultDeploymentHandler{
			osClient:    osClient,
			kubeClient:  kubeClient,
			environment: initialEnvironment,
		},
	}
	return dc
}

// Run begins watching and synchronizing deployment states.
func (dc *DeploymentController) Run(period time.Duration) {
	dc.syncTime = time.Tick(period)
	go util.Forever(func() { dc.synchronize() }, period)
}

// The main synchronization loop.  Iterates through all deployments and handles the current state
// for each.
func (dc *DeploymentController) synchronize() {
	deployments, err := dc.osClient.ListDeployments(labels.Everything())
	if err != nil {
		glog.Errorf("Synchronization error: %v (%#v)", err, err)
		return
	}

	for ix := range deployments.Items {
		id := deployments.Items[ix].ID
		deployment, err := dc.osClient.GetDeployment(id)
		if err != nil {
			glog.Errorf("Got error retrieving deployment with id %s -- %v", id, err)
			continue
		}
		err = dc.syncDeployment(&deployment)
		if err != nil {
			glog.Errorf("Error synchronizing: %#v", err)
		}
	}
}

// Invokes the appropriate handler for the current state of the given deployment.
func (dc *DeploymentController) syncDeployment(deployment *deployapi.Deployment) error {
	glog.Infof("Synchronizing deployment id: %v status: %v resourceVersion: %v", deployment.ID, deployment.Status, deployment.ResourceVersion)
	var err error = nil
	switch deployment.Status {
	case deployapi.DeploymentNew:
		err = dc.stateHandler.HandleNew(deployment)
	case deployapi.DeploymentPending:
		err = dc.stateHandler.HandlePending(deployment)
	case deployapi.DeploymentRunning:
		err = dc.stateHandler.HandleRunning(deployment)
	}
	return err
}

func (dh *DefaultDeploymentHandler) saveDeployment(deployment *deployapi.Deployment) error {
	glog.Infof("Saving deployment %v status: %v", deployment.ID, deployment.Status)
	_, err := dh.osClient.UpdateDeployment(*deployment)
	if err != nil {
		glog.Errorf("Received error while saving deployment %v: %v", deployment.ID, err)
	}
	return err
}

func (dh *DefaultDeploymentHandler) makeDeploymentPod(deployment *deployapi.Deployment) api.Pod {
	podID := deploymentPodID(deployment)

	envVars := deployment.Strategy.CustomPod.Environment
	envVars = append(envVars, api.EnvVar{Name: "KUBERNETES_DEPLOYMENT_ID", Value: deployment.ID})
	for _, env := range dh.environment {
		envVars = append(envVars, env)
	}

	return api.Pod{
		JSONBase: api.JSONBase{
			ID: podID,
		},
		DesiredState: api.PodState{
			Manifest: api.ContainerManifest{
				Version: "v1beta1",
				Containers: []api.Container{
					{
						Name:          "deployment",
						Image:         deployment.Strategy.CustomPod.Image,
						Env:           envVars,
						RestartPolicy: "runOnce",
					},
				},
			},
			RestartPolicy: api.RestartPolicy{
				Type: api.RestartNever,
			},
		},
	}
}

func deploymentPodID(deployment *deployapi.Deployment) string {
	return "deploy-" + deployment.ConfigID
}

// Handler for a deployment in the 'new' state.
func (dh *DefaultDeploymentHandler) HandleNew(deployment *deployapi.Deployment) error {
	deploymentPod := dh.makeDeploymentPod(deployment)
	glog.Infof("Attempting to create deployment pod: %+v", deploymentPod)
	if pod, err := dh.kubeClient.CreatePod(deploymentPod); err != nil {
		glog.Warningf("Received error creating pod: %v", err)
		deployment.Status = deployapi.DeploymentFailed
	} else {
		glog.Infof("Successfully created pod %+v", pod)
		deployment.Status = deployapi.DeploymentPending
	}

	return dh.saveDeployment(deployment)
}

// Handler for a deployment in the 'pending' state
func (dh *DefaultDeploymentHandler) HandlePending(deployment *deployapi.Deployment) error {
	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)
	pod, err := dh.kubeClient.GetPod(podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		deployment.Status = deployapi.DeploymentFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)

		switch pod.CurrentState.Status {
		case api.PodRunning:
			deployment.Status = deployapi.DeploymentRunning
		case api.PodTerminated:
			checkForTerminatedDeploymentPod(deployment, pod)
		}
	}

	return dh.saveDeployment(deployment)
}

// Handler for a deployment in the 'running' state
func (dh *DefaultDeploymentHandler) HandleRunning(deployment *deployapi.Deployment) error {
	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)
	pod, err := dh.kubeClient.GetPod(podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		deployment.Status = deployapi.DeploymentFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)
		checkForTerminatedDeploymentPod(deployment, pod)
	}

	return dh.saveDeployment(deployment)
}

func checkForTerminatedDeploymentPod(deployment *deployapi.Deployment, pod api.Pod) {
	if pod.CurrentState.Status != api.PodTerminated {
		glog.Infof("The deployment has not yet finished. Pod status is %s. Continuing", pod.CurrentState.Status)
		return
	}

	deployment.Status = deployapi.DeploymentComplete
	for _, info := range pod.CurrentState.Info {
		if info.State.ExitCode != 0 {
			deployment.Status = deployapi.DeploymentFailed
		}
	}
	glog.Infof("The deployment pod has finished. Setting deployment status to %s", deployment.Status)
	return
}
