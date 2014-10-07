// +build integration,!no-etcd

package integration

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	klatest "github.com/GoogleCloudPlatform/kubernetes/pkg/api/latest"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/master"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/version"
	"github.com/golang/glog"

	"github.com/openshift/origin/pkg/api/latest"
	"github.com/openshift/origin/pkg/api/v1beta1"
	osclient "github.com/openshift/origin/pkg/client"

	deploy "github.com/openshift/origin/pkg/deploy"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deploygen "github.com/openshift/origin/pkg/deploy/generator"
	deployregistry "github.com/openshift/origin/pkg/deploy/registry/deploy"
	deployconfigregistry "github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	deployetcd "github.com/openshift/origin/pkg/deploy/registry/etcd"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imageetcd "github.com/openshift/origin/pkg/image/registry/etcd"
	"github.com/openshift/origin/pkg/image/registry/image"
	"github.com/openshift/origin/pkg/image/registry/imagerepository"
	"github.com/openshift/origin/pkg/image/registry/imagerepositorymapping"
)

func init() {
	requireEtcd()
}

func imageChangeDeploymentConfig() *deployapi.DeploymentConfig {
	return &deployapi.DeploymentConfig{
		JSONBase: kapi.JSONBase{ID: "image-deploy-config"},
		Triggers: []deployapi.DeploymentTriggerPolicy{
			{
				Type: deployapi.DeploymentTriggerOnImageChange,
				ImageChangeParams: &deployapi.DeploymentTriggerImageChangeParams{
					ContainerNames: []string{
						"container-1",
					},
					RepositoryName: "registry:8080/openshift/test-image",
					Tag:            "latest",
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
								{
									Name:  "container-2",
									Image: "registry:8080/openshift/another-test-image:ref-1",
								},
							},
						},
					},
				},
			},
		},
	}
}

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

func changeDeploymentConfig() *deployapi.DeploymentConfig {
	return &deployapi.DeploymentConfig{
		JSONBase: kapi.JSONBase{ID: "change-deploy-config"},
		Triggers: []deployapi.DeploymentTriggerPolicy{
			{
				Type: deployapi.DeploymentTriggerManual,
			},
			{
				Type:      deployapi.DeploymentTriggerOnConfigChange,
				Automatic: true,
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
									Env: []kapi.EnvVar{
										{
											Name:  "ENV_TEST",
											Value: "ENV_VALUE1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func retry(maxAttempts int, delay time.Duration, f func() bool, label string) bool {
	for i := 0; i <= maxAttempts; i++ {
		if f() {
			return true
		}

		time.Sleep(delay)
		glog.Infof("Retrying '%s' (attempt %d of %d)...", label, (i + 1), maxAttempts)
	}

	return false
}

func TestSuccessfulManualDeployment(t *testing.T) {
	openshift := NewTestOpenshift(t)
	defer openshift.Shutdown()

	config := manualDeploymentConfig()
	var err error

	if _, err := openshift.Client.CreateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create DeploymentConfig: %v %#v", err, config)
	}

	if config, err = openshift.Client.GenerateDeploymentConfig(config.ID); err != nil {
		t.Fatalf("Error generating config: %v", err)
	}

	if _, err := openshift.Client.UpdateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v %#v", err, config)
	}

	watch, err := openshift.Client.WatchDeployments(labels.Everything(),
		labels.Set{"configID": config.ID}.AsSelector(), 0)
	if err != nil {
		t.Fatalf("Couldn't subscribe to Deployments: %v", err)
	}

	event := <-watch.ResultChan()

	deployment := event.Object.(*deployapi.Deployment)

	if e, a := config.ID, deployment.Labels["configID"]; e != a {
		t.Fatalf("Expected deployment configID label '%s', got '%s'", e, a)
	}
}

func TestSimpleImageChangeTrigger(t *testing.T) {
	openshift := NewTestOpenshift(t)
	defer openshift.Shutdown()

	imageRepo := &imageapi.ImageRepository{
		JSONBase:              kapi.JSONBase{ID: "test-image-repo"},
		DockerImageRepository: "registry:8080/openshift/test-image",
		Tags: map[string]string{
			"latest": "ref-1",
		},
	}

	config := imageChangeDeploymentConfig()
	var err error

	if _, err := openshift.Client.CreateImageRepository(imageRepo); err != nil {
		t.Fatalf("Couldn't create ImageRepository: %v", err)
	}

	if _, err := openshift.Client.CreateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create DeploymentConfig: %v", err)
	}

	if config, err = openshift.Client.GenerateDeploymentConfig(config.ID); err != nil {
		t.Fatalf("Error generating config: %v", err)
	}

	if _, err := openshift.Client.UpdateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v", err)
	}

	watch, err := openshift.Client.WatchDeployments(labels.Everything(),
		labels.Set{"configID": config.ID}.AsSelector(), 0)
	if err != nil {
		t.Fatalf("Couldn't subscribe to Deployments %v", err)
	}

	event := <-watch.ResultChan()

	deployment := event.Object.(*deployapi.Deployment)

	if e, a := config.ID, deployment.Labels["configID"]; e != a {
		t.Fatalf("Expected deployment configID label '%s', got '%s'", e, a)
	}
}

func TestSimpleConfigChangeTrigger(t *testing.T) {
	openshift := NewTestOpenshift(t)
	defer openshift.Shutdown()

	config := changeDeploymentConfig()
	var err error

	// submit the initial deployment config
	if _, err := openshift.Client.CreateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create DeploymentConfig: %v", err)
	}

	// submit the initial generated config, which will cause an initial deployment
	if config, err = openshift.Client.GenerateDeploymentConfig(config.ID); err != nil {
		t.Fatalf("Error generating config: %v", err)
	}

	if _, err := openshift.Client.UpdateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v", err)
	}

	watch, err := openshift.Client.WatchDeployments(labels.Everything(),
		labels.Set{"configID": config.ID}.AsSelector(), 0)
	if err != nil {
		t.Fatalf("Couldn't subscribe to Deployments %v", err)
	}

	event := <-watch.ResultChan()

	// verify the initial deployment exists
	deployment := event.Object.(*deployapi.Deployment)

	if e, a := config.ID, deployment.Labels["configID"]; e != a {
		t.Fatalf("Expected deployment configID label '%s', got '%s'", e, a)
	}

	assertEnvVarEquals("ENV_TEST", "ENV_VALUE1", deployment, t)

	// submit a new config with an updated environment variable
	if config, err = openshift.Client.GenerateDeploymentConfig(config.ID); err != nil {
		t.Fatalf("Error generating config: %v", err)
	}

	config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers[0].Env[0].Value = "UPDATED"

	if _, err := openshift.Client.UpdateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v", err)
	}

	event = <-watch.ResultChan()
	deployment = event.Object.(*deployapi.Deployment)

	assertEnvVarEquals("ENV_TEST", "UPDATED", deployment, t)
}

func assertEnvVarEquals(name string, value string, deployment *deployapi.Deployment, t *testing.T) {
	env := deployment.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers[0].Env

	for _, e := range env {
		if e.Name == name && e.Value == value {
			return
		}
	}

	t.Fatalf("Expected env var with name %s and value %s", name, value)
}

type podInfoGetter struct {
	PodInfo kapi.PodInfo
	Error   error
}

func (p *podInfoGetter) GetPodInfo(host, podID string) (kapi.PodInfo, error) {
	return p.PodInfo, p.Error
}

type testOpenshift struct {
	Client          *osclient.Client
	server          *httptest.Server
	stopControllers chan struct{}
}

func (o *testOpenshift) Shutdown() {
	close(o.stopControllers)
	o.server.CloseClientConnections()
	o.server.Close()
	deleteAllEtcdKeys()
	glog.Info("Destroyed test openshift")
}

func NewTestOpenshift(t *testing.T) *testOpenshift {
	glog.Info("Starting test openshift")

	openshift := &testOpenshift{}

	etcdClient := newEtcdClient()
	etcdHelper, _ := master.NewEtcdHelper(etcdClient.GetCluster(), klatest.Version)

	osMux := http.NewServeMux()
	openshift.server = httptest.NewServer(osMux)

	kubeClient := client.NewOrDie(&client.Config{Host: openshift.server.URL, Version: klatest.Version})
	osClient, _ := osclient.New(&client.Config{Host: openshift.server.URL, Version: latest.Version})

	openshift.Client = osClient

	kmaster := master.New(&master.Config{
		Client:             kubeClient,
		EtcdHelper:         etcdHelper,
		PodInfoGetter:      &podInfoGetter{},
		HealthCheckMinions: false,
		Minions:            []string{"127.0.0.1"},
	})

	interfaces, _ := latest.InterfacesFor(latest.Version)

	imageEtcd := imageetcd.New(etcdHelper)
	deployEtcd := deployetcd.New(etcdHelper)
	deployConfigGen := deploygen.NewDeploymentConfigGenerator(deployEtcd, deployEtcd, imageEtcd)

	storage := map[string]apiserver.RESTStorage{
		"images":                  image.NewREST(imageEtcd),
		"imageRepositories":       imagerepository.NewREST(imageEtcd),
		"imageRepositoryMappings": imagerepositorymapping.NewREST(imageEtcd, imageEtcd),
		"deployments":             deployregistry.NewREST(deployEtcd),
		"deploymentConfigs":       deployconfigregistry.NewREST(deployEtcd),
		"genDeploymentConfigs":    deploygen.NewStorage(deployConfigGen, v1beta1.Codec),
	}

	apiserver.NewAPIGroup(kmaster.API_v1beta1()).InstallREST(osMux, "/api/v1beta1")
	osPrefix := "/osapi/v1beta1"
	apiserver.NewAPIGroup(storage, v1beta1.Codec, osPrefix, interfaces.SelfLinker).InstallREST(osMux, osPrefix)
	apiserver.InstallSupport(osMux)

	info, err := kubeClient.ServerVersion()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if e, a := version.Get(), *info; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#v, got %#v", e, a)
	}

	// start controllers
	openshift.stopControllers = make(chan struct{})

	env := []kapi.EnvVar{{Name: "KUBERNETES_MASTER", Value: openshift.server.URL}}
	deployController := deploy.NewDeploymentController(kubeClient, osClient, env)
	deployConfigController := deploy.NewDeploymentConfigController(osClient)
	deployTriggerController := deploy.NewDeploymentTriggerController(osClient)

	go deployController.SyncDeployments()
	go deployConfigController.SyncDeploymentConfigs()
	go deployTriggerController.SyncDeploymentTriggers()

	go func() {
		<-openshift.stopControllers

		glog.Info("Shutting down test controllers")
		deployController.Shutdown()
		deployConfigController.Shutdown()
		deployTriggerController.Shutdown()
	}()

	return openshift
}
