package generator

import (
	"fmt"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"net/http"
	"strings"
)

type controller struct {
	generator DeploymentConfigGenerator
	codec     runtime.Codec
}

// urlVars holds parsed URL parts.
type urlVars struct {
	deploymentConfigID string
}

func NewDeploymentConfigGeneratorController(generator DeploymentConfigGenerator, codec runtime.Codec) http.Handler {
	return &controller{generator: generator, codec: codec}
}

func (c *controller) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	uv, err := parseUrl(req.URL.Path)
	if err != nil {
		notFound(w, err.Error())
		return
	}

	deployConfig, genErr := c.generator.Generate(uv.deploymentConfigID)
	if genErr != nil {
		badRequest(w, genErr.Error())
		return
	}

	// TODO: write respose using codec
	b, encErr := c.codec.Encode(deployConfig)
	if encErr != nil {
		badRequest(w, encErr.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Print(w, b)
}

func parseUrl(url string) (uv urlVars, err error) {
	parts := splitPath(url)
	if len(parts) != 1 {
		err = fmt.Errorf("Unexpected URL %s", url)
		return
	}
	uv = urlVars{parts[0]}
	return
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

func notFound(w http.ResponseWriter, args ...string) {
	http.Error(w, strings.Join(args, ""), http.StatusNotFound)
}

func badRequest(w http.ResponseWriter, args ...string) {
	http.Error(w, strings.Join(args, ""), http.StatusBadRequest)
}
