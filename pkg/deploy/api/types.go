package api

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
)

type CustomPodDeploymentStrategy struct {
	Image       string       `json:"image,omitempty" yaml:"image,omitempty"`
	Environment []api.EnvVar `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// Equivalent to LivenessProbe
type DeploymentStrategy struct {
	Type      string                       `json:"type,omitempty" yaml:"type,omitempty"`
	CustomPod *CustomPodDeploymentStrategy `json:"customPod,omitempty" yaml:"customPod,omitempty"`
}

type DeploymentTemplate struct {
	Strategy           DeploymentStrategy             `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ControllerTemplate api.ReplicationControllerState `json:"controllerTemplate,omitempty" yaml:"controllerTemplate,omitempty"`
}

type DeploymentStatus string

const (
	DeploymentNew      DeploymentStatus = "new"
	DeploymentPending  DeploymentStatus = "pending"
	DeploymentRunning  DeploymentStatus = "running"
	DeploymentComplete DeploymentStatus = "complete"
	DeploymentFailed   DeploymentStatus = "failed"
)

// A Deployment represents a single unique realization of a DeploymentConfig.
type Deployment struct {
	api.JSONBase       `json:",inline" yaml:",inline"`
	Labels             map[string]string              `json:"labels,omitempty" yaml:"labels,omitempty"`
	Strategy           DeploymentStrategy             `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ControllerTemplate api.ReplicationControllerState `json:"controllerTemplate,omitempty" yaml:"controllerTemplate,omitempty"`
	Status             DeploymentStatus               `json:"status,omitempty" yaml:"status,omitempty"`
	ConfigID           string                         `json:"configId,omitempty" yaml:"configId,omitempty"`
}

type DeploymentTriggerPolicy string

const (
	DeploymentTriggerOnImageChange  DeploymentTriggerPolicy = "image-change"
	DeploymentTriggerOnConfigChange DeploymentTriggerPolicy = "config-change"
	DeploymentTriggerManual         DeploymentTriggerPolicy = "manual"
)

type DeploymentConfig struct {
	api.JSONBase  `json:",inline" yaml:",inline"`
	Labels        map[string]string              `json:"labels,omitempty" yaml:"labels,omitempty"`
	TriggerPolicy DeploymentTriggerPolicy        `json:"triggerPolicy,omitempty" yaml:"triggerPolicy,omitempty"`
	Template      DeploymentTemplate             `json:"template,omitempty" yaml:"template,omitempty"`
	CurrentState  api.ReplicationControllerState `json:"currentState" yaml:"currentState,omitempty"`
}

// A DeploymentConfigList is a collection of deployment configs
type DeploymentConfigList struct {
	api.JSONBase `json:",inline" yaml:",inline"`
	Items        []DeploymentConfig `json:"items,omitempty" yaml:"items,omitempty"`
}

// A DeploymentList is a collection of deployments.
type DeploymentList struct {
	api.JSONBase `json:",inline" yaml:",inline"`
	Items        []Deployment `json:"items,omitempty" yaml:"items,omitempty"`
}
