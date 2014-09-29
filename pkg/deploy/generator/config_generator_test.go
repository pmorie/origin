package generator

import (
	deploytest "github.com/openshift/origin/pkg/deploy/registry/test"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/image/registry/test"
	"testing"
)

func TestGenerateFromMissingDeploymentConfig(t *testing.T) {
	deploymentConfigRegistry := deploytest.NewDeploymentConfigRegistry()
	imageRepoRegistry := imagetest.NewImageRepositoryRegistry()

	imageRepoRegistry.ImageRepositories = &imageapi.ImageRepositoryList{
		Items: []imageapi.ImageRepository{},
	}

	generator := NewDeploymentConfigGenerator(deploymentConfigRegistry, imageRepoRegistry)

	config, err := generator.Generate("1234")

	if config != nil {
		t.Errorf("Unexpected deployment config generated: %#v", config)
	}

	if err != nil {
		t.Errorf("Expected an error")
	}
}
