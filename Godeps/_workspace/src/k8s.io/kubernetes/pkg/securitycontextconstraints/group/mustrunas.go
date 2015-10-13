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

package group

import (
	"fmt"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/fielderrors"
)

// mustRunAs implements the GroupSecurityContextConstraintsStrategy interface
type mustRunAs struct {
	ranges []api.IDRange
	field string
}

var _ GroupSecurityContextConstraintsStrategy = &mustRunAs{}

// NewMustRunAs provides a new MustRunAs strategy based on ranges.
func NewMustRunAs(ranges []api.IDRange, field string) (GroupSecurityContextConstraintsStrategy, error){
	if len(ranges) == 0 {
		return nil, fmt.Errorf("ranges must be supplied for MustRunAs")
	}
	return &mustRunAs{
		ranges: ranges,
		field: field,
	}, nil
}


// Generate creates the group based on policy rules.  By default this returns the first group of the
// first range (min val).
func (s *mustRunAs) Generate(pod *api.Pod) ([]int64, error){
	return []int64{s.ranges[0].Min}, nil
}

// Validate ensures that the specified values fall within the range of the strategy.
// Groups are passed in here to allow this strategy to support multiple group fields (fsgroup and
// supplemental groups).
func (s *mustRunAs) Validate(pod *api.Pod, groups []int64) fielderrors.ValidationErrorList {
	allErrs := fielderrors.ValidationErrorList{}

	if pod.Spec.SecurityContext == nil {
		detail := fmt.Sprintf("unable to validate nil security context for pod %s", pod.Name)
		allErrs = append(allErrs, fielderrors.NewFieldInvalid("securityContext", pod.Spec.SecurityContext, detail))
		return allErrs
	}

	if len(groups) == 0 && len(s.ranges) > 0 {
		detail := fmt.Sprintf("unable to validate empty groups against required ranges")
		allErrs = append(allErrs, fielderrors.NewFieldInvalid(s.field, groups, detail))
	}

	for _, group := range groups {
		if !s.groupHasValidRange(group) {
			detail := fmt.Sprintf("%d is not an allowed group", group)
			allErrs = append(allErrs, fielderrors.NewFieldInvalid(s.field, groups, detail))
		}
	}

	return allErrs
}

func (s *mustRunAs) groupHasValidRange(group int64) bool {
	for _, rng := range s.ranges {
		if fallsInRange(group, rng) {
			return true
		}
	}
	return false
}

func fallsInRange(group int64, rng api.IDRange) bool {
	return group >= rng.Min && group <= rng.Max
}
