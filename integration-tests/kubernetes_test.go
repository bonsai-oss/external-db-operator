//go:build integration

package integration_tests

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func kubectl(args ...string) *exec.Cmd {
	return exec.Command("kubectl", args...)
}

func TestDatabase(t *testing.T) {
	kubectl("apply", "-f", "../manifests/crd.yaml").Run()
	kubectl("apply", "-f", "../manifests/rbac.yaml").Run()

	deploymentFile, deploymentFileOpenError := os.Open("../manifests/deployment.yaml")
	if deploymentFileOpenError != nil {
		t.Fatalf("failed to open deployment manifest: %v", deploymentFileOpenError)
	}
	deploymentFileContent, manifestLoadError := io.ReadAll(deploymentFile)
	if manifestLoadError != nil {
		t.Fatalf("failed to load manifest: %v", manifestLoadError)
	}
	deploymentManifest := string(deploymentFileContent)

	for _, testCase := range []struct {
		name     string
		provider string
		dsn      string
	}{
		{
			name:     "postgres",
			provider: "postgres",
			dsn:      "postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			// Deploy test database
			kubectl("apply", "-f", "../manifests/databases/"+testCase.provider+".yaml").Run()

			testCaseDeploymentManifest := strings.Replace(deploymentManifest, "<DSN>", testCase.dsn, -1)
			testCaseDeploymentManifest = strings.Replace(testCaseDeploymentManifest, "<PROVIDER>", testCase.provider, -1)
			apply := kubectl("apply", "-f", "-")
			apply.Stdin = strings.NewReader(testCaseDeploymentManifest)
			if applyOperatorOutput, applyOperatorError := apply.CombinedOutput(); applyOperatorError != nil {
				t.Errorf("failed to apply operator: %v \n %v", applyOperatorError, string(applyOperatorOutput))
				return
			} else {
				t.Log(string(applyOperatorOutput))
			}

			if output, waitError := kubectl("wait", "--for=condition=Available", "--timeout=1m", "deployment/external-db-operator-"+testCase.provider).CombinedOutput(); waitError != nil {
				t.Errorf("failed to wait for operator: %v \n %v", waitError, string(output))
				return
			} else {
				t.Log(string(output))
			}
		})
	}
}
