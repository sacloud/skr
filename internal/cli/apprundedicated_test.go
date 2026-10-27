// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	apprunmock "github.com/sacloud/sakumock/apprundedicated"
)

func TestRunAppRunDedicatedAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"apprun-dedicated-api", "--help"},
			want: []string{"cluster", "application", "version", "auto-scaling-group", "worker-node", "load-balancer", "certificate", "service-class", "SAKURA_ACCESS_TOKEN", "専用ワーカノード", "manual.sakura.ad.jp/cloud/apprun-dedicated/about.html"},
		},
		{
			args: []string{"apprun-dedicated-api", "application", "create", "--help"},
			want: []string{"--name", "--cluster-id"},
		},
		{
			args: []string{"apprun-dedicated-api", "version", "create", "--help"},
			want: []string{"--application-id", "--request", "version.CreateParams", "@path.json", "標準入力"},
		},
		{
			args: []string{"apprun-dedicated-api", "certificate", "create", "--help"},
			want: []string{"--cluster-id", "--request", "certificatePem", "privatekeyPem", "@path.json"},
		},
		{
			args: []string{"apprun-dedicated-api", "application", "update", "--help"},
			want: []string{"--active-version", "--deactivate"},
		},
		{
			args: []string{"apprun-dedicated-api", "worker-node", "update", "--help"},
			want: []string{"--draining=STRING", "true で draining", "false で解除"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help does not contain %q", text)
			}
		})
	}
}

func TestRunAppRunDedicatedRejectsInvalidApplicationUpdate(t *testing.T) {
	for _, args := range [][]string{
		{"apprun-dedicated-api", "application", "update", "fd16da34-68dc-4ccb-a3db-391207b5dd40"},
		{"apprun-dedicated-api", "application", "update", "fd16da34-68dc-4ccb-a3db-391207b5dd40", "--active-version", "1", "--deactivate"},
	} {
		var stdout, stderr bytes.Buffer
		require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
		require.Contains(t, stderr.String(), "--active-version または --deactivate", "run(%v) error = %q, want mutually-exclusive input error", args, stderr.String())
	}
}

func TestRunAppRunDedicatedRequiresPrivateVersionRequestInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"apprun-dedicated-api", "version", "create",
		"--application-id", "fd16da34-68dc-4ccb-a3db-391207b5dd40",
		"--request", `{"image":"private-image","registryPassword":"private-secret"}`,
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "@path.json", "version request error = %q, want safe file-input guidance without secret", stderr.String())
	require.NotContains(t, stderr.String(), "private-secret", "unexpected text %q", "private-secret")
}

func TestAppRunDedicatedAPIWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	var createdCluster v1.CreatedCluster
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "cluster", "create", "--request",
		`{"name":"mock-cluster","servicePrincipalID":"123456789012","ports":[{"port":443,"protocol":"https"}]}`), &createdCluster))
	clusterOutput := runCommand("apprun-dedicated-api", "cluster", "list")
	var clusters struct {
		Items []struct {
			ClusterID v1.ClusterID `json:"clusterID"`
			Name      string       `json:"name"`
		} `json:"items"`
	}
	err := json.Unmarshal(clusterOutput, &clusters)
	require.NoError(t, err)
	require.Len(t, clusters.Items, 1, "cluster list = %s, want mock-cluster", clusterOutput)
	require.Equal(t, "mock-cluster", clusters.Items[0].Name, "unexpected value")
	require.Equal(t, clusters.Items[0].ClusterID, createdCluster.ClusterID, "cluster create returned ID %v, list returned %v", createdCluster.ClusterID, clusters.Items[0].ClusterID)
	clusterID := jsonString(t, createdCluster.ClusterID)

	var readCluster applicationOutput
	err2 := json.Unmarshal(runCommand("apprun-dedicated-api", "cluster", "read", clusterID), &readCluster)
	require.NoError(t, err2)
	require.Equal(t, "mock-cluster", readCluster.Name, "cluster read returned name %q, want mock-cluster", readCluster.Name)
	runCommand("apprun-dedicated-api", "cluster", "update", clusterID, "--request", `{"servicePrincipalID":"123456789012"}`)

	var createdGroup v1.CreatedAutoScalingGroup
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "auto-scaling-group", "create",
		"--cluster-id", clusterID,
		"--request", `{"name":"mock-asg","zone":"is1b","nameServers":["210.188.224.10"],"workerServiceClassPath":"cloud/apprun/dedicated/worker/1vcpu_2gb","minNodes":1,"maxNodes":1,"interfaces":[{"interfaceIndex":0,"upstream":"shared","ipPool":[],"connectsToLB":false}]}`), &createdGroup))
	groupID := jsonString(t, createdGroup.AutoScalingGroupID)
	var groups struct {
		Items []struct {
			AutoScalingGroupID v1.AutoScalingGroupID `json:"autoScalingGroupID"`
		} `json:"items"`
	}
	err3 := json.Unmarshal(runCommand("apprun-dedicated-api", "auto-scaling-group", "list", "--cluster-id", clusterID), &groups)
	require.NoError(t, err3)
	if len(groups.Items) != 1 || jsonString(t, groups.Items[0].AutoScalingGroupID) != groupID {
		t.Fatalf("auto-scaling-group list = %#v, want group %s", groups.Items, groupID)
	}
	runCommand("apprun-dedicated-api", "auto-scaling-group", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID)

	var workerNodes struct {
		Items []struct {
			WorkerNodeID v1.WorkerNodeID `json:"workerNodeID"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "worker-node", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID), &workerNodes))
	require.NotEmpty(t, workerNodes.Items, "worker-node list returned no nodes")
	workerNodeID := jsonString(t, workerNodes.Items[0].WorkerNodeID)
	runCommand("apprun-dedicated-api", "worker-node", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID)
	runCommand("apprun-dedicated-api", "worker-node", "update", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID, "--draining", "true")
	runCommand("apprun-dedicated-api", "worker-node", "update", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID, "--draining", "false")

	var createdLB v1.CreatedLoadBalancer
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "load-balancer", "create",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID,
		"--request", `{"name":"mock-lb","serviceClassPath":"cloud/apprun/dedicated/lb/1vcpu_2gb","nameServers":["210.188.224.10"],"interfaces":[{"interfaceIndex":0,"upstream":"shared","ipPool":[]}]}`), &createdLB))
	lbID := jsonString(t, createdLB.LoadBalancerID)
	runCommand("apprun-dedicated-api", "load-balancer", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)
	var loadBalancers struct {
		Items []v1.ReadLoadBalancerSummary `json:"items"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "load-balancer", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID), &loadBalancers))
	require.Len(t, loadBalancers.Items, 1, "load-balancer list returned %d results, want 1", len(loadBalancers.Items))
	runCommand("apprun-dedicated-api", "load-balancer", "node", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)

	var createdCertificate v1.CreatedCertificate
	certificateFile := t.TempDir() + "/certificate.json"
	certificateJSON := `{"name":"mock-cert","certificatePem":"-----BEGIN CERTIFICATE-----\nMIItest\n-----END CERTIFICATE-----","privatekeyPem":"-----BEGIN PRIVATE KEY-----\nMIItest\n-----END PRIVATE KEY-----"}`
	err4 := os.WriteFile(certificateFile, []byte(certificateJSON), 0o600)
	require.NoError(t, err4)
	err5 := json.Unmarshal(runCommand("apprun-dedicated-api", "certificate", "create", "--cluster-id", clusterID, "--request", "@"+certificateFile), &createdCertificate)
	require.NoError(t, err5)
	certificateID := jsonString(t, createdCertificate.CertificateID)
	runCommand("apprun-dedicated-api", "certificate", "read", "--cluster-id", clusterID, "--certificate-id", certificateID)
	err6 := os.WriteFile(certificateFile, []byte(strings.ReplaceAll(certificateJSON, "mock-cert", "mock-cert-updated")), 0o600)
	require.NoError(t, err6)
	runCommand("apprun-dedicated-api", "certificate", "update", "--cluster-id", clusterID,
		"--certificate-id", certificateID, "--request", "@"+certificateFile)
	runCommand("apprun-dedicated-api", "certificate", "delete", "--cluster-id", clusterID, "--certificate-id", certificateID)
	runCommand("apprun-dedicated-api", "service-class", "list-lb")
	runCommand("apprun-dedicated-api", "service-class", "list-worker")

	var createdApp v1.CreatedApplication
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "application", "create",
		"--name", "mock-app", "--cluster-id", clusterID), &createdApp))
	appID := jsonString(t, createdApp.ApplicationID)

	var app applicationOutput
	err7 := json.Unmarshal(runCommand("apprun-dedicated-api", "application", "read", appID), &app)
	require.NoError(t, err7)
	require.Equal(t, "mock-app", app.Name, "application name = %q, want mock-app", app.Name)

	requestPath := t.TempDir() + "/version.json"
	err8 := os.WriteFile(requestPath, []byte(`{"cpu":1000,"memory":2048,"scalingMode":"manual","fixedScale":1,"image":"nginx:latest","registryPasswordAction":"keep","exposedPorts":[{"targetPort":80}]}`), 0o600)
	require.NoError(t, err8)
	var createdVersion v1.ReadApplicationVersionSummary
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "version", "create",
		"--application-id", appID, "--request", "@"+requestPath), &createdVersion))
	if createdVersion.Version < 1 {
		t.Fatalf("created version = %d, want positive version", createdVersion.Version)
	}

	var versions struct {
		Items []v1.ApplicationVersionDeploymentStatus `json:"items"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "version", "list",
		"--application-id", appID, "--max-items", "30"), &versions))
	require.Len(t, versions.Items, 1, "version list = %#v, want version %d", versions.Items, createdVersion.Version)
	require.Equal(t, createdVersion.Version, versions.Items[0].Version, "unexpected value")

	runCommand("apprun-dedicated-api", "application", "update", appID, "--active-version", "1")
	var containers []map[string]any
	err9 := json.Unmarshal(runCommand("apprun-dedicated-api", "application", "containers", appID), &containers)
	require.NoError(t, err9)
	runCommand("apprun-dedicated-api", "application", "update", appID, "--deactivate")
	runCommand("apprun-dedicated-api", "version", "delete", "--application-id", appID, "1")
	runCommand("apprun-dedicated-api", "application", "delete", appID)
	runCommand("apprun-dedicated-api", "load-balancer", "delete", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)
	runCommand("apprun-dedicated-api", "auto-scaling-group", "delete", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID)
	runCommand("apprun-dedicated-api", "cluster", "delete", clusterID)

	violations := server.SpecViolations()
	require.Len(t, violations, 0, "sakumock recorded OpenAPI response violations: %+v", violations)
}

type applicationOutput struct {
	Name string `json:"name"`
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return strings.Trim(string(data), `"`)
}
