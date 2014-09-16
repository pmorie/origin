package trigger

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
)

// A filter for deployment config IDs
type DeploymentConfigTriggers struct {
	util.StringSet
}

func NewDeploymentConfigTriggers() DeploymentConfigTriggers {
	return DeploymentConfigTriggers{util.StringSet{}}
}

func (t *DeploymentConfigTriggers) Fire(config *deployapi.DeploymentConfig) bool {
	return t.Has(config.ID)
}
