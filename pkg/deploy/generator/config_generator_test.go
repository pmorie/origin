package generator

import (
	kubeapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deploytest "github.com/openshift/origin/pkg/deploy/registry/test"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/image/registry/test"
	"testing"
)

func basicPodTemplate() kubeapi.PodTemplate {
	return kubeapi.PodTemplate{
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
	}
}
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
					RepositoryName: "registry:8080/repo1",
					Tag:            "tag1",
				},
			},
		},
		Template: deployapi.DeploymentTemplate{
			ControllerTemplate: kubeapi.ReplicationControllerState{
				PodTemplate: basicPodTemplate(),
			},
		},
	}
}

func basicImageRepo() *imageapi.ImageRepositoryList {
	return &imageapi.ImageRepositoryList{
		Items: []imageapi.ImageRepository{
			{
				JSONBase:              kubeapi.JSONBase{ID: "imageRepo1"},
				DockerImageRepository: "registry:8080/repo1",
				Tags: map[string]string{
					"tag1": "ref1",
				},
			},
		},
	}
}

func updatedImageRepo() *imageapi.ImageRepositoryList {
	return &imageapi.ImageRepositoryList{
		Items: []imageapi.ImageRepository{
			{
				JSONBase:              kubeapi.JSONBase{ID: "imageRepo1"},
				DockerImageRepository: "registry:8080/repo1",
				Tags: map[string]string{
					"tag1": "ref2",
				},
			},
		},
	}
}

func TestGenerateFromMissingDeploymentConfig(t *testing.T) {
	deploymentRegistry := deploytest.NewDeploymentRegistry()
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = basicImageRepo()

	generator := NewDeploymentConfigGenerator(deploymentRegistry, deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("1234")

	if config != nil {
		t.Fatalf("Unexpected deployment config generated: %#v", config)
	}

	if err == nil {
		t.Fatalf("Expected an error")
	}
}

func TestGenerateFromConfigWithoutTagChange(t *testing.T) {
	deploymentRegistry := deploytest.NewDeploymentRegistry()
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = basicImageRepo()

	deploymentConfigRegistry.DeploymentConfig = basicDeploymentConfig()

	// use a deployment which matches the config
	deploymentRegistry.Deployment = &deployapi.Deployment{
		ControllerTemplate: kubeapi.ReplicationControllerState{
			PodTemplate: basicPodTemplate(),
		},
	}

	generator := NewDeploymentConfigGenerator(deploymentRegistry, deploymentConfigRegistry, imageRepoRegistry)

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

func TestGenerateFromConfigWithNoDeployment(t *testing.T) {
	deploymentRegistry := deploytest.NewDeploymentRegistry()
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = basicImageRepo()

	deploymentConfigRegistry.DeploymentConfig = basicDeploymentConfig()

	generator := NewDeploymentConfigGenerator(deploymentRegistry, deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("deploy2")

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
	deploymentRegistry := deploytest.NewDeploymentRegistry()
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = updatedImageRepo()

	deploymentConfigRegistry.DeploymentConfig = basicDeploymentConfig()

	deploymentRegistry.Deployment = &deployapi.Deployment{
		ControllerTemplate: kubeapi.ReplicationControllerState{
			PodTemplate: basicPodTemplate(),
		},
	}

	generator := NewDeploymentConfigGenerator(deploymentRegistry, deploymentConfigRegistry, imageRepoRegistry)

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

	expected := "registry:8080/repo1:ref2"
	actual := config.Template.ControllerTemplate.PodTemplate.DesiredState.Manifest.Containers[0].Image
	if expected != actual {
		t.Fatalf("Expected container image %s, got %s", expected, actual)
	}
}
