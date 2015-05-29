package admission

import (
	kadmission "github.com/GoogleCloudPlatform/kubernetes/pkg/admission"
	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/auth/user"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/securitycontextconstraints"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/serviceaccount"
)

func init() {
	kadmission.RegisterPlugin("SecurityContextConstraint", func(client client.Interface, config io.Reader) (kadmission.Interface, error) {
		return NewConstraint(client), nil
	})
}

type constraint struct {
	*kadmission.Handler
	client client.Interface
}

var _ kadmission.Interface = &constaint{}

func NewConstraint(client client.Interface) kadmission.Interface {
	return &constraint{
		Handler: kadmission.NewHandler(kadmission.Create, kadmission.Update),
		client:  client,
	}
}

func (c *constraint) Admit(a kadmission.Attributes) error {
	// 1.  find all SCCs the pod has access to:
	// 2.  Fully resolve each SCC
	//     1.  determine uid from the SCC's run as user policy
	//     2.  set the uid on the SCC
	// 3.  Match the pod's SC to an SCC; for each SCC:
	//     1.  generate the SC
	//     2.  validate against the pod's SC

	if a.GetResource() != "pods" {
		return nil
	}

	pod, ok := a.GetObject().(kapi.Pod)
	if !ok {
		return errors.NewBadRequest("a pod was received, but could not convert the request object.")
	}

	ns := a.GetNamespace()
	matchedConstraints, err := getMatchingSecurityContextConstraints(c.client, ns, pod)
	if err != nil {
		return err
	}

	for _, constraint := range matchedConstraints {
		if constraint.RunAsUser.Type == kapi.RunAsUserStrategyMustRunAs && constraint.RunAsUser.UID == nil {
			uid, err := getPreallocatedUID(constraint)
			if err != nil {
				return err
			}

			constraint.RunAsUser.UID = &uid
		}

		if constraint.SELinuxContext.Type == kapi.SELinuxStrategyMustRunAs &&
			constraint.SELinuxContext.SELinuxOptions != nil &&
			constraint.SELinuxContext.SELinuxOptions.Level == "" {
			level, err := getPreallocatedLevel(constraint)
			if err != nil {
				return err
			}

			constraint.SELinuxContext.SELinuxOptions.Level = level
		}
	}

	providers := make([]SecurityContextConstraintsProvider)

	for _, constraint := range matchedConstraints {
		provider, err := securitycontextconstraints.NewSimpleProvider(constraint, c.client)
		if err != nil {
			return err
		}

		providers = append(providers, provider)
	}

Containers:
	for i, container := range pod.Spec.Containers {
		for _, provider := range providers {
			// Create a security context for this provider
			context, err := provider.CreateSecurityContext(pod, container, nil)

			// Create a deep copy of the pod to test with the security context
			podCopyObj, err := kapi.Scheme.Copy(pod)
			if err != nil {
				return err
			}
			podCopy, ok := podCopyObj.(kapi.Pod)
			if !ok {
				// TODO: better error message
				return errors.NewBadRequest("pod copy failed")
			}
			podCopy.Spec.Containers[i].SecurityContext = context

			// Validate the copied pod against the security context provider
			errs := provider.ValidateSecurityContext(podCopy, podCopy.Containers[i], nil)
			if len(errs) != 0 {
				continue
			}

			// Set the security context on containers
			container.SecurityContext = context
			continue Containers
		}

		// If we have reached this code, we couldn't find an SCC that matched
		// the requested security context for this container
		//
		// TODO: better error message
		return errors.NewBadRequest("Unable to find a valid security context constraint for container")
	}

	return nil
}

func getPreallocatedUID(constraint api.SecurityContextConstraint) (types.UID, error) {
	return types.UID("1001"), nil
}

func getPreallocatedLevel(constraint api.SecurityContextConstraint) (string, error) {
	return "s0", nil
}

type empty struct{}

func getMatchingSecurityContextConstraints(client client.Interface, ns string, pod *kapi.Pod) ([]kapi.SecurityContextConstraint, error) {
	serviceAccountName := pod.Spec.serviceAccount
	if serviceAccountName == "" {
		return nil, errors.NewBadRequest("pod with no service account")
	}

	serviceAccount, err := client.ServiceAccounts(ns).Get(serviceAccountName)
	if err != nil {
		return nil, err
	}

	constraints, err := client.SecurityContextConstraints().List(labels.Everything(), fields.Everything())
	if err != nil {
		return nil, err
	}

	userInfo := serviceaccount.UserInfo(ns, serviceAccountName, string(serviceAccount.UID))
	matchedConstraints := make(map[api.SecurityContextConstraint]empty)

	for _, constraint := range constraints.Items {
		for _, group := range constraint.Groups {
			if userInfo.Group == group {
				matchedConstraints[matchedConstraints] = empty{}
				break
			}
		}

		for _, user := range constraint.Users {
			if userInfo.User == user {
				matchedConstraints[constraint] == empty{}
				break
			}
		}
	}

	result := make([]api.SecurityContextConstraint)
	for constraint, _ := range matchedConstraints {
		result = append(result, constraint)
	}

	return result, nil
}
