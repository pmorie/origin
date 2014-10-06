package integration

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	etcdconfig "github.com/coreos/etcd/config"
	"github.com/coreos/etcd/etcd"
	etcdclient "github.com/coreos/go-etcd/etcd"

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

var testPodInfoGetter = &podInfoGetter{}

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
	if _, err := openshift.Client.CreateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create DeploymentConfig: %v", err)
	}

	newConfig, genErr := openshift.Client.GenerateDeploymentConfig(config.ID)
	if genErr != nil {
		t.Fatalf("Error generating config: %v", genErr)
	}

	if newConfig == nil {
		t.Fatalf("Expected a generated config from id %s", config.ID)
	}

	if _, err := openshift.Client.UpdateDeploymentConfig(newConfig); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v", err)
	}

	deploymentExists := func() bool {
		deployments, listErr := openshift.Client.ListDeployments(labels.Everything())
		if listErr != nil {
			t.Fatalf("Couldn't list deployments: %v", listErr)
		}

		return len(deployments.Items) > 0
	}

	if !retry(5, time.Second/2, deploymentExists, "deployment check") {
		t.Fatalf("Expected a deployment to exist")
	}

	deployments, _ := openshift.Client.ListDeployments(labels.Everything())
	if len(deployments.Items) != 1 {
		t.Fatalf("Expected 1 deployment, got %d", len(deployments.Items))
	}

	deploymentLabel := deployments.Items[0].Labels["configID"]
	if deploymentLabel != newConfig.ID {
		t.Fatalf("Expected deployment configID label '%s', got '%s'", deploymentLabel, newConfig.ID)
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

	if _, err := openshift.Client.CreateImageRepository(imageRepo); err != nil {
		t.Fatalf("Couldn't create ImageRepository: %v", err)
	}

	config := imageChangeDeploymentConfig()
	if _, err := openshift.Client.CreateDeploymentConfig(config); err != nil {
		t.Fatalf("Couldn't create DeploymentConfig: %v", err)
	}

	newConfig, genErr := openshift.Client.GenerateDeploymentConfig(config.ID)
	if genErr != nil {
		t.Fatalf("Error generating config: %v", genErr)
	}

	if newConfig == nil {
		t.Fatalf("Expected a generated config from id %s", config.ID)
	}

	if _, err := openshift.Client.UpdateDeploymentConfig(newConfig); err != nil {
		t.Fatalf("Couldn't create updated DeploymentConfig: %v", err)
	}

	deploymentExists := func() bool {
		deployments, listErr := openshift.Client.ListDeployments(labels.Everything())
		if listErr != nil {
			t.Fatalf("Couldn't list deployments: %v", listErr)
		}

		return len(deployments.Items) > 0
	}

	if !retry(5, time.Second/2, deploymentExists, "deployment check") {
		t.Fatalf("Expected a deployment to exist")
	}

	deployments, _ := openshift.Client.ListDeployments(labels.Everything())
	if len(deployments.Items) != 1 {
		t.Fatalf("Expected 1 deployment, got %d", len(deployments.Items))
	}

	deploymentLabel := deployments.Items[0].Labels["configID"]
	if deploymentLabel != newConfig.ID {
		t.Fatalf("Expected deployment configID label '%s', got '%s'", deploymentLabel, newConfig.ID)
	}
}

type podInfoGetter struct {
	PodInfo kapi.PodInfo
	Error   error
}

func (p *podInfoGetter) GetPodInfo(host, podID string) (kapi.PodInfo, error) {
	return p.PodInfo, p.Error
}

type testOpenshift struct {
	Client  *osclient.Client
	server  *httptest.Server
	etcd    *etcd.Etcd
	etcdDir string
}

func (o *testOpenshift) Shutdown() {
	glog.Info("Shutting down test openshift")
	o.server.CloseClientConnections()
	o.server.Close()
	o.etcd.Stop()

	os.RemoveAll(o.etcdDir)
}

func NewTestOpenshift(t *testing.T) *testOpenshift {
	glog.Info("Starting test openshift")

	openshift := &testOpenshift{}

	// TODO: auto-assign a random port
	etcdAddr := "localhost:5050"
	etcdDir, err := ioutil.TempDir(os.TempDir(), "etcd")

	if err != nil {
		t.Fatalf("Couldn't create temp dir for etcd: %v", err)
	}

	openshift.etcdDir = etcdDir

	etcdConfig := etcdconfig.New()
	etcdConfig.Addr = etcdAddr
	etcdConfig.BindAddr = etcdAddr
	etcdConfig.DataDir = etcdDir
	etcdConfig.Name = "openshift.local"

	// initialize etcd
	openshift.etcd = etcd.New(etcdConfig)
	glog.Infof("Starting etcd at http://%s", etcdAddr)
	go openshift.etcd.Run()

	etcdClient := etcdclient.NewClient([]string{"http://" + etcdAddr})

	for !etcdClient.SyncCluster() {
		glog.Info("Waiting for etcd to become available...")
		time.Sleep(1 * time.Second)
	}

	etcdHelper, _ := master.NewEtcdHelper(etcdClient.GetCluster(), klatest.Version)

	osMux := http.NewServeMux()
	openshift.server = httptest.NewServer(osMux)

	kubeClient := client.NewOrDie(openshift.server.URL, klatest.Version, nil)
	osClient, _ := osclient.New(openshift.server.URL, latest.Version, nil)

	openshift.Client = osClient

	kmaster := master.New(&master.Config{
		Client:             kubeClient,
		EtcdHelper:         etcdHelper,
		PodInfoGetter:      testPodInfoGetter,
		HealthCheckMinions: false,
		Minions:            []string{"127.0.0.1"},
	})

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
	apiserver.NewAPIGroup(storage, v1beta1.Codec).InstallREST(osMux, osPrefix)
	apiserver.InstallSupport(osMux)

	info, err := kubeClient.ServerVersion()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if e, a := version.Get(), *info; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#v, got %#v", e, a)
	}

	env := []kapi.EnvVar{{Name: "KUBERNETES_MASTER", Value: openshift.server.URL}}
	deployController := deploy.NewDeploymentController(kubeClient, osClient, env)
	deployController.Run(5 * time.Second)

	deployConfigController := deploy.NewDeploymentConfigController(osClient)
	deployConfigController.Run(5 * time.Second)

	deployTriggerController := deploy.NewDeploymentTriggerController(osClient)
	deployTriggerController.Run(5 * time.Second)

	return openshift
}
