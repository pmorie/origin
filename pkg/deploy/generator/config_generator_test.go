package generator

import (
	kubeapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deploytest "github.com/openshift/origin/pkg/deploy/registry/test"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/image/registry/test"
	"testing"
)

func basicDeploymentConfig() *deployapi.DeploymentConfig {
	return &deployapi.DeploymentConfig{
		JSONBase:      kubeapi.JSONBase{ID: "deploy1"},
		LatestVersion: 1,
		Triggers: []deployapi.DeploymentTriggerPolicy{
			{
				Type: deployapi.DeploymentTriggerOnImageChange,
				ImageChangeParams: &deployapi.DeploymentTriggerImageChangeParams{
					ContainerNames: []string{
						"container1",
					},
					RepositoryName: "imageRepo1",
					Tag:            "tag1",
				},
			},
		},
		Template: deployapi.DeploymentTemplate{
			ControllerTemplate: kubeapi.ReplicationControllerState{
				PodTemplate: kubeapi.PodTemplate{
					DesiredState: kubeapi.PodState{
						Manifest: kubeapi.ContainerManifest{
							Containers: []kubeapi.Container{
								{
									Name:  "container1",
									Image: "registry:8080/repo1:ref1",
								},
								{
									Name:  "container2",
									Image: "registry:8080/repo1:ref2",
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestGenerateFromMissingDeploymentConfig(t *testing.T) {
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = &imageapi.ImageRepositoryList{
		Items: []imageapi.ImageRepository{},
	}

	generator := NewDeploymentConfigGenerator(deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("1234")

	if config != nil {
		t.Fatalf("Unexpected deployment config generated: %#v", config)
	}

	if err == nil {
		t.Fatalf("Expected an error")
	}
}

func TestGenerateFromConfigWithNoRepoReferences(t *testing.T) {
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = &imageapi.ImageRepositoryList{
		Items: []imageapi.ImageRepository{},
	}

	deploymentConfigRegistry.DeploymentConfig = basicDeploymentConfig()

	generator := NewDeploymentConfigGenerator(deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("deploy1")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if config == nil {
		t.Fatalf("Expected non-nil config")
	}

	if config.LatestVersion != 1 {
		t.Fatalf("Expected config LatestVersion=1, got %d", config.LatestVersion)
	}
}

func TestGenerateFromConfigWithUpdatedImageRef(t *testing.T) {
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepository = &imageapi.ImageRepository{
		JSONBase:              kubeapi.JSONBase{ID: "imageRepo1"},
		DockerImageRepository: "registry:8080/repo1",
		Tags: map[string]string{
			"tag1": "ref3",
		},
	}

	deploymentConfigRegistry.DeploymentConfig = basicDeploymentConfig()

	generator := NewDeploymentConfigGenerator(deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("deploy1")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if config == nil {
		t.Fatalf("Expected non-nil config")
	}

	if config.LatestVersion != 2 {
		t.Fatalf("Expected config LatestVersion=2, got %d", config.LatestVersion)
	}

	expected := "registry:8080/repo1:ref3"
	actual := config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers[0].Image
	if expected != actual {
		t.Fatalf("Expected container image %s, got %s", expected, actual)
	}
}
