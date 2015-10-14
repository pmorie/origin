// +build integration,docker

package integration

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	kapi "k8s.io/kubernetes/pkg/api"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"

	"github.com/fsouza/go-dockerclient"
	"github.com/openshift/origin/pkg/cmd/server/bootstrappolicy"
	testutil "github.com/openshift/origin/test/util"
	testserver "github.com/openshift/origin/test/util/server"
	"k8s.io/kubernetes/pkg/util/wait"
)

// init ensures docker exists for this test
func init() {
	testutil.RequireDocker()
}

// TestSupplementalGroups tests that a requested fs group and supplemental group
// pass through the system and to the resulting docker config.
func TestSupplementalGroups(t *testing.T) {
	kubeClient := setupSupplementalGroupsTest(t)

	fsGroup := 1111
	supGroup := 2222

	pod := &kapi.Pod{
		ObjectMeta: kapi.ObjectMeta{
			Name: "supplemental-groups",
		},
		Spec: kapi.PodSpec{
			SecurityContext: &kapi.PodSecurityContext{
				FSGroup:            &fsGroup,
				SupplementalGroups: []int{supGroup},
			},
			Containers: []kapi.Container{
				{
					Name:  "supplemental-groups",
					Image: "openshift/origin-pod",
				},
			},
		},
	}

	_, err := kubeClient.Pods(testutil.Namespace()).Create(pod)
	if err != nil {
		t.Fatalf("unable to create pod: %v", err)
	}
	defer kubeClient.Pods(testutil.Namespace()).Delete(pod.Name, nil)

	validateGroups(fsGroup, []int{supGroup}, pod, kubeClient, t)
}

func TestPreallocatedGroups(t *testing.T) {
	kubeClient := setupSupplementalGroupsTest(t)
	pod := &kapi.Pod{
		ObjectMeta: kapi.ObjectMeta{
			Name: "supplemental-groups",
		},
		Spec: kapi.PodSpec{
			SecurityContext: &kapi.PodSecurityContext{},
			Containers: []kapi.Container{
				{
					Name:  "supplemental-groups",
					Image: "openshift/origin-pod",
				},
			},
		},
	}

	_, err := kubeClient.Pods(testutil.Namespace()).Create(pod)
	if err != nil {
		t.Fatalf("unable to create pod: %v", err)
	}
	defer kubeClient.Pods(testutil.Namespace()).Delete(pod.Name, nil)

	p, err := kubeClient.Pods(testutil.Namespace()).Get(pod.Name)
	if err != nil {
		t.Fatalf("unable to get pod: %v", err)
	}
	if p.Spec.SecurityContext == nil || len(p.Spec.SecurityContext.SupplementalGroups) == 0 {
		t.Fatalf("unexpected security context, expected to have preallocated sup group set: %v", p.Spec.SecurityContext)
	}
	if p.Spec.SecurityContext == nil || p.Spec.SecurityContext.FSGroup == nil {
		t.Fatalf("unexpected security context, expected to have preallocated fs group set: %v", p.Spec.SecurityContext)
	}

	validateGroups(*p.Spec.SecurityContext.FSGroup, p.Spec.SecurityContext.SupplementalGroups, pod, kubeClient, t)
}

func validateGroups(fsGroup int, supplementalGroups []int, pod *kapi.Pod, kubeClient *kclient.Client, t *testing.T) {
	var containerID string = ""
	err := wait.Poll(100*time.Millisecond, 5*time.Second, func() (bool, error) {
		p, err := kubeClient.Pods(testutil.Namespace()).Get(pod.Name)

		if err != nil {
			return false, fmt.Errorf("unable to get pod: %v", err)
		}

		if len(p.Status.ContainerStatuses) > 0 {
			for _, status := range p.Status.ContainerStatuses {
				if len(status.ContainerID) > 0 {
					containerID = strings.Replace(status.ContainerID, "docker://", "", -1)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		t.Fatalf(err.Error())
	}
	if len(containerID) == 0 {
		t.Fatalf("container id was not set, cannot continue")
	}

	dockerCli, err := testutil.NewDockerClient()
	if err != nil {
		t.Fatalf("unable to get docker client: %v", err)
	}

	container, err := dockerCli.InspectContainer(containerID)
	if err != nil {
		t.Fatalf("unable to inspect container with id %s: %v", containerID, err)
	}

	if !configHasGroup(fsGroup, container.HostConfig) {
		t.Errorf("requested fs group was not found on resulting docker config: %v", container.HostConfig.GroupAdd)
	}

	for _, g := range supplementalGroups {
		if !configHasGroup(g, container.HostConfig) {
			t.Errorf("requested sup group was not found on resulting docker config %v", container.HostConfig.GroupAdd)
		}
	}
}

func configHasGroup(group int, config *docker.HostConfig) bool {
	strGroup := strconv.Itoa(group)
	for _, g := range config.GroupAdd {
		if g == strGroup {
			return true
		}
	}
	return false
}

func setupSupplementalGroupsTest(t *testing.T) *kclient.Client {
	_, _, clusterAdminKubeConfig, err := testserver.StartTestAllInOne()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	kubeClient, err := testutil.GetClusterAdminKubeClient(clusterAdminKubeConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = kubeClient.Namespaces().Create(&kapi.Namespace{
		ObjectMeta: kapi.ObjectMeta{Name: testutil.Namespace()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = testserver.WaitForServiceAccounts(kubeClient, testutil.Namespace(), []string{bootstrappolicy.DefaultServiceAccountName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return kubeClient
}
