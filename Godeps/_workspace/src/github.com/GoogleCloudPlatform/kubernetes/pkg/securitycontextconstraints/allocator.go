/*
Copyright 2014 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package securitycontextconstraints

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/securitycontextconstraints/selinux"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/securitycontextconstraints/user"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/fielderrors"
)

// simpleProvider is the default implementation of SecurityContextConstraintsProvider
type simpleProvider struct {
	scc               *api.SecurityContextConstraints
	runAsUserStrategy user.RunAsUserSecurityContextConstraintsStrategy
	seLinuxStrategy   selinux.SELinuxSecurityContextConstraintsStrategy
	client            client.Interface
}

func NewSimpleProvider(scc *api.SecurityContextConstraints, client client.Interface) (SecurityContextConstraintsProvider, error) {
	if client == nil || scc == nil {
		return nil, fmt.Errorf("NewSimpleProvider requires a client and SecurityContextConstraints")
	}

	var userStrat user.RunAsUserSecurityContextConstraintsStrategy = nil
	var err error = nil
	switch scc.RunAsUser.Type {
	case api.RunAsUserStrategyMustRunAs:
		userStrat, err = user.NewMustRunAs(&scc.RunAsUser)
	case api.RunAsUserStrategyMustRunAsNonRoot:
		userStrat, err = user.NewRunAsNonRoot(&scc.RunAsUser)
	case api.RunAsUserStrategyRunAsAny:
		userStrat, err = user.NewRunAsAny(&scc.RunAsUser)
	default:
		err = fmt.Errorf("Unrecognized RunAsUser strategy type %s", scc.RunAsUser.Type)
	}
	if err != nil {
		return nil, err
	}

	var seLinuxStrat selinux.SELinuxSecurityContextConstraintsStrategy = nil
	err = nil
	switch scc.SELinuxContext.Type {
	case api.SELinuxStrategyMustRunAs:
		seLinuxStrat, err = selinux.NewMustRunAs(&scc.SELinuxContext)
	case api.SELinuxStrategyRunAsAny:
		seLinuxStrat, err = selinux.NewRunAsAny(&scc.SELinuxContext)
	default:
		err = fmt.Errorf("Unrecognized SELinuxcontext strategy type %s", scc.SELinuxContext.Type)
	}
	if err != nil {
		return nil, err
	}

	return &simpleProvider{
		scc:               scc,
		runAsUserStrategy: userStrat,
		seLinuxStrategy:   seLinuxStrat,
		client:            client,
	}, nil
}

// Create a SecurityContext based on the given constraints.  If a setting is already set on the
// container's security context then it will not be changed.  Validation should be used after
// the context is created to ensure it complies with the required restrictions.
func (s *simpleProvider) CreateSecurityContext(pod *api.Pod, container *api.Container) (*api.SecurityContext, error) {
	var sc *api.SecurityContext = nil
	if container.SecurityContext != nil {
		sc = container.SecurityContext
	} else {
		sc = &api.SecurityContext{}
	}

	if sc.RunAsUser == nil {
		uid, err := s.runAsUserStrategy.Generate(pod, container)
		if err != nil {
			return nil, err
		}
		sc.RunAsUser = uid
	}

	if sc.SELinuxOptions == nil {
		seLinux, err := s.seLinuxStrategy.Generate(pod, container)
		if err != nil {
			return nil, err
		}
		sc.SELinuxOptions = seLinux
	}

	if sc.Privileged == nil {
		priv := false
		sc.Privileged = &priv
	}

	// No need to touch capabilities, they will validate or not.
	return sc, nil
}

// Ensure a container's SecurityContext is in compliance with the given constraints
func (s *simpleProvider) ValidateSecurityContext(pod *api.Pod, container *api.Container) fielderrors.ValidationErrorList {
	allErrs := fielderrors.ValidationErrorList{}

	if container.SecurityContext == nil {
		allErrs = append(allErrs, fielderrors.NewFieldInvalid("securityContext", container.SecurityContext, "No security context is set"))
		return allErrs
	}

	sc := container.SecurityContext
	allErrs = append(allErrs, s.runAsUserStrategy.Validate(pod, container)...)
	allErrs = append(allErrs, s.seLinuxStrategy.Validate(pod, container)...)

	if !s.scc.AllowPrivilegedContainer && *sc.Privileged {
		allErrs = append(allErrs, fielderrors.NewFieldInvalid("privileged", *sc.Privileged, "Privileged containers are not allowed"))
	}

	if sc.Capabilities != nil && len(sc.Capabilities.Add) > 0 {
		for _, cap := range sc.Capabilities.Add {
			found := false
			for _, allowedCap := range s.scc.AllowedCapabilities {
				if cap == allowedCap {
					found = true
					break
				}
			}
			if !found {
				allErrs = append(allErrs, fielderrors.NewFieldInvalid("capabilities.add", cap, "Capability is not allowed to be added"))
			}
		}
	}

	if !s.scc.AllowHostDirVolumePlugin {
		for _, vm := range container.VolumeMounts {
			volume, err := s.client.PersistentVolumes().Get(vm.Name)
			if err != nil {
				allErrs = append(allErrs, fmt.Errorf("unable to validate volume %s: %s", vm.Name, err.Error()))
				continue
			}
			if volume.Spec.HostPath != nil {
				allErrs = append(allErrs, fielderrors.NewFieldInvalid("container.VolumeMounts", vm.Name, "Host Volumes are not allowed to be used"))
			}
		}
	}
	return allErrs
}
