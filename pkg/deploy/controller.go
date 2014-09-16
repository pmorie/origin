package deploy

import (
	"time"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kubeclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"
	osclient "github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// A DeploymentController is responsible for executing Deployment objects stored in etcd
type DeploymentController struct {
	osClient     osclient.Interface
	kubeClient   kubeclient.Interface
	stateHandler DeploymentStateHandler
	deployWatch  watch.Interface
	shutdown     chan struct{}
}

// DeploymentStateHandler holds methods that handle the possible deployment states.
type DeploymentStateHandler interface {
	HandleNew(kapi.Context, *deployapi.Deployment) error
	HandlePending(kapi.Context, *deployapi.Deployment) error
	HandleRunning(kapi.Context, *deployapi.Deployment) error
}

// DefaultDeploymentRunner is the default implementation of DeploymentRunner interface.
type DefaultDeploymentHandler struct {
	osClient    osclient.Interface
	kubeClient  kubeclient.Interface
	environment []kapi.EnvVar
}

// NewDeploymentController creates a new DeploymentController.
func NewDeploymentController(kubeClient kubeclient.Interface, osClient osclient.Interface, initialEnvironment []kapi.EnvVar) *DeploymentController {
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
	go util.Forever(func() { dc.SyncDeployments() }, period)
}

func (dc *DeploymentController) Shutdown() {
	close(dc.shutdown)
}

// The main synchronization loop.  Iterates through all deployments and handles the current state
// for each.
func (dc *DeploymentController) SyncDeployments() {
	ctx := kapi.NewContext()
	dc.shutdown = make(chan struct{})

	deployments, err := dc.osClient.ListDeployments(ctx, labels.Everything())
	if err != nil {
		glog.Errorf("Synchronization error: %v (%#v)", err, err)
		return
	}

	for ix := range deployments.Items {
		id := deployments.Items[ix].ID
		deployment, err := dc.osClient.GetDeployment(ctx, id)
		if err != nil {
			glog.Errorf("Got error retrieving deployment with id %s -- %v", id, err)
			continue
		}
		err = dc.syncDeployment(ctx, deployment)
		if err != nil {
			glog.Errorf("Error synchronizing: %#v", err)
		}
	}

	glog.Info("Subscribing to deployment configs")
	if dc.deployWatch, err = dc.osClient.WatchDeployments(ctx, labels.Everything(), labels.Everything(), 0); err != nil {
		glog.Errorf("Error subscribing to deployments: %#v", err)
		return
	}

	go dc.watchDeployments(ctx)

	<-dc.shutdown
}

// Invokes the appropriate handler for the current state of the given deployment.
func (dc *DeploymentController) syncDeployment(ctx kapi.Context, deployment *deployapi.Deployment) error {
	glog.Infof("Synchronizing deployment id: %v state: %v resourceVersion: %v", deployment.ID, deployment.State, deployment.ResourceVersion)
	var err error = nil
	switch deployment.State {
	case deployapi.DeploymentStateNew:
		err = dc.stateHandler.HandleNew(ctx, deployment)
	case deployapi.DeploymentStatePending:
		err = dc.stateHandler.HandlePending(ctx, deployment)
	case deployapi.DeploymentStateRunning:
		err = dc.stateHandler.HandleRunning(ctx, deployment)
	}
	return err
}

func (dc *DeploymentController) watchDeployments(ctx kapi.Context) {
	for {
		select {
		case <-dc.shutdown:
			return
		case event, open := <-dc.deployWatch.ResultChan():
			if !open {
				// watchChannel has been closed, or something else went
				// wrong with our etcd watch call. Let the util.Forever()
				// that called us call us again.
				return
			}

			deployment, ok := event.Object.(*deployapi.Deployment)
			if !ok {
				glog.Errorf("Received unexpected object during deployment watch: %v", event)
				continue
			}

			if err := dc.syncDeployment(ctx, deployment); err != nil {
				glog.Errorf("Error synchronizing: %#v", err)
			}
		}
	}
}

func (dh *DefaultDeploymentHandler) saveDeployment(ctx kapi.Context, deployment *deployapi.Deployment) error {
	glog.Infof("Saving deployment %v state: %v", deployment.ID, deployment.State)
	_, err := dh.osClient.UpdateDeployment(ctx, deployment)
	if err != nil {
		glog.Errorf("Received error while saving deployment %v: %v", deployment.ID, err)
	}
	return err
}

func (dh *DefaultDeploymentHandler) makeDeploymentPod(deployment *deployapi.Deployment) *kapi.Pod {
	podID := deploymentPodID(deployment)

	envVars := deployment.Strategy.CustomPod.Environment
	envVars = append(envVars, kapi.EnvVar{Name: "KUBERNETES_DEPLOYMENT_ID", Value: deployment.ID})
	for _, env := range dh.environment {
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

func deploymentPodID(deployment *deployapi.Deployment) string {
	return "deploy-" + deployment.ID
}

// Handler for a deployment in the 'new' state.
func (dh *DefaultDeploymentHandler) HandleNew(ctx kapi.Context, deployment *deployapi.Deployment) error {
	deploymentPod := dh.makeDeploymentPod(deployment)
	glog.Infof("Attempting to create deployment pod: %+v", deploymentPod)
	if pod, err := dh.kubeClient.CreatePod(kapi.NewContext(), deploymentPod); err != nil {
		glog.Warningf("Received error creating pod: %v", err)
		deployment.State = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Successfully created pod %+v", pod)
		deployment.State = deployapi.DeploymentStatePending
	}

	return dh.saveDeployment(ctx, deployment)
}

// Handler for a deployment in the 'pending' state
func (dh *DefaultDeploymentHandler) HandlePending(ctx kapi.Context, deployment *deployapi.Deployment) error {
	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)
	pod, err := dh.kubeClient.GetPod(ctx, podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		deployment.State = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)

		switch pod.CurrentState.Status {
		case kapi.PodRunning:
			deployment.State = deployapi.DeploymentStateRunning
		case kapi.PodTerminated:
			dh.checkForTerminatedDeploymentPod(deployment, pod)
		}
	}

	return dh.saveDeployment(ctx, deployment)
}

// Handler for a deployment in the 'running' state
func (dh *DefaultDeploymentHandler) HandleRunning(ctx kapi.Context, deployment *deployapi.Deployment) error {
	podID := deploymentPodID(deployment)
	glog.Infof("Retrieving deployment pod id %s", podID)
	pod, err := dh.kubeClient.GetPod(ctx, podID)
	if err != nil {
		glog.Errorf("Error retrieving pod for deployment ID %v: %#v", deployment.ID, err)
		deployment.State = deployapi.DeploymentStateFailed
	} else {
		glog.Infof("Deployment pod is %+v", pod)
		dh.checkForTerminatedDeploymentPod(deployment, pod)
	}

	return dh.saveDeployment(ctx, deployment)
}

func (dh *DefaultDeploymentHandler) checkForTerminatedDeploymentPod(deployment *deployapi.Deployment, pod *kapi.Pod) {
	if pod.CurrentState.Status != kapi.PodTerminated {
		glog.Infof("The deployment has not yet finished. Pod status is %s. Continuing", pod.CurrentState.Status)
		return
	}

	deployment.State = deployapi.DeploymentStateComplete
	for _, info := range pod.CurrentState.Info {
		if info.State.Termination != nil && info.State.Termination.ExitCode != 0 {
			deployment.State = deployapi.DeploymentStateFailed
		}
	}

	if deployment.State == deployapi.DeploymentStateComplete {
		podID := deploymentPodID(deployment)
		glog.Infof("Removing deployment pod for ID %v", podID)
		dh.kubeClient.DeletePod(kapi.NewContext(), podID)
	}

	glog.Infof("The deployment pod has finished. Setting deployment state to %s", deployment.State)
	return
}
