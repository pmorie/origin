package generator

import (
	"errors"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"

	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

type Storage struct {
	generator DeploymentConfigGenerator
	codec     runtime.Codec
}

func NewStorage(generator DeploymentConfigGenerator, codec runtime.Codec) apiserver.RESTStorage {
	return &Storage{generator: generator, codec: codec}
}

func (s *Storage) New() runtime.Object {
	return &deployapi.DeploymentConfig{}
}

func (s *Storage) List(ctx api.Context, labels, fields labels.Selector) (runtime.Object, error) {
	return nil, errors.New("deploy/generator.Storage.List() is not implemented.")
}

func (s *Storage) Get(ctx api.Context, id string) (runtime.Object, error) {
	return s.generator.Generate(id)
}

func (s *Storage) Delete(ctx api.Context, id string) (<-chan runtime.Object, error) {
	return nil, errors.New("deploy/generator.Storage.Delete() is not implemented.")
}

func (s *Storage) Update(ctx api.Context, obj runtime.Object) (<-chan runtime.Object, error) {
	return nil, errors.New("deploy/generator.Storage.Update() is not implemented.")
}

func (s *Storage) Create(ctx api.Context, obj runtime.Object) (<-chan runtime.Object, error) {
	return nil, errors.New("deploy/generator.Storage.Create() is not implemented.")
}
