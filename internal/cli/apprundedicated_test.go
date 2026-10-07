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
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) code = %d; stderr: %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help does not contain %q", text)
				}
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
		if code := run(args, &stdout, &stderr); code == 0 {
			t.Errorf("run(%v) succeeded, want invalid update input error", args)
		}
		if !strings.Contains(stderr.String(), "--active-version または --deactivate") {
			t.Errorf("run(%v) error = %q, want mutually-exclusive input error", args, stderr.String())
		}
	}
}

func TestRunAppRunDedicatedRequiresPrivateVersionRequestInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"apprun-dedicated-api", "version", "create",
		"--application-id", "fd16da34-68dc-4ccb-a3db-391207b5dd40",
		"--request", `{"image":"private-image","registryPassword":"private-secret"}`,
	}
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("inline secret version request succeeded")
	}
	if !strings.Contains(stderr.String(), "@path.json") || strings.Contains(stderr.String(), "private-secret") {
		t.Fatalf("version request error = %q, want safe file-input guidance without secret", stderr.String())
	}
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
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) code = %d; stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() > 0 {
			t.Errorf("run(%v) stderr = %q", args, stderr.String())
		}
		return stdout.Bytes()
	}

	var createdCluster v1.CreatedCluster
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "cluster", "create", "--request",
		`{"name":"mock-cluster","servicePrincipalID":"123456789012","ports":[{"port":443,"protocol":"https"}]}`), &createdCluster); err != nil {
		t.Fatal(err)
	}
	clusterOutput := runCommand("apprun-dedicated-api", "cluster", "list")
	var clusters struct {
		Items []struct {
			ClusterID v1.ClusterID `json:"clusterID"`
			Name      string       `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(clusterOutput, &clusters); err != nil {
		t.Fatal(err)
	}
	if len(clusters.Items) != 1 || clusters.Items[0].Name != "mock-cluster" {
		t.Fatalf("cluster list = %s, want mock-cluster", clusterOutput)
	}
	if createdCluster.ClusterID != clusters.Items[0].ClusterID {
		t.Fatalf("cluster create returned ID %v, list returned %v", createdCluster.ClusterID, clusters.Items[0].ClusterID)
	}
	clusterID := jsonString(t, createdCluster.ClusterID)

	var readCluster applicationOutput
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "cluster", "read", clusterID), &readCluster); err != nil {
		t.Fatal(err)
	}
	if readCluster.Name != "mock-cluster" {
		t.Fatalf("cluster read returned name %q, want mock-cluster", readCluster.Name)
	}
	runCommand("apprun-dedicated-api", "cluster", "update", clusterID, "--request", `{"servicePrincipalID":"123456789012"}`)

	var createdGroup v1.CreatedAutoScalingGroup
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "auto-scaling-group", "create",
		"--cluster-id", clusterID,
		"--request", `{"name":"mock-asg","zone":"is1b","nameServers":["210.188.224.10"],"workerServiceClassPath":"cloud/apprun/dedicated/worker/1vcpu_2gb","minNodes":1,"maxNodes":1,"interfaces":[{"interfaceIndex":0,"upstream":"shared","ipPool":[],"connectsToLB":false}]}`), &createdGroup); err != nil {
		t.Fatal(err)
	}
	groupID := jsonString(t, createdGroup.AutoScalingGroupID)
	var groups struct {
		Items []struct {
			AutoScalingGroupID v1.AutoScalingGroupID `json:"autoScalingGroupID"`
		} `json:"items"`
	}
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "auto-scaling-group", "list", "--cluster-id", clusterID), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups.Items) != 1 || jsonString(t, groups.Items[0].AutoScalingGroupID) != groupID {
		t.Fatalf("auto-scaling-group list = %#v, want group %s", groups.Items, groupID)
	}
	runCommand("apprun-dedicated-api", "auto-scaling-group", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID)

	var workerNodes struct {
		Items []struct {
			WorkerNodeID v1.WorkerNodeID `json:"workerNodeID"`
		} `json:"items"`
	}
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "worker-node", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID), &workerNodes); err != nil {
		t.Fatal(err)
	}
	if len(workerNodes.Items) == 0 {
		t.Fatal("worker-node list returned no nodes")
	}
	workerNodeID := jsonString(t, workerNodes.Items[0].WorkerNodeID)
	runCommand("apprun-dedicated-api", "worker-node", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID)
	runCommand("apprun-dedicated-api", "worker-node", "update", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID, "--draining", "true")
	runCommand("apprun-dedicated-api", "worker-node", "update", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--worker-node-id", workerNodeID, "--draining", "false")

	var createdLB v1.CreatedLoadBalancer
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "load-balancer", "create",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID,
		"--request", `{"name":"mock-lb","serviceClassPath":"cloud/apprun/dedicated/lb/1vcpu_2gb","nameServers":["210.188.224.10"],"interfaces":[{"interfaceIndex":0,"upstream":"shared","ipPool":[]}]}`), &createdLB); err != nil {
		t.Fatal(err)
	}
	lbID := jsonString(t, createdLB.LoadBalancerID)
	runCommand("apprun-dedicated-api", "load-balancer", "read", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)
	var loadBalancers struct {
		Items []v1.ReadLoadBalancerSummary `json:"items"`
	}
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "load-balancer", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID), &loadBalancers); err != nil {
		t.Fatal(err)
	}
	if len(loadBalancers.Items) != 1 {
		t.Fatalf("load-balancer list returned %d results, want 1", len(loadBalancers.Items))
	}
	runCommand("apprun-dedicated-api", "load-balancer", "node", "list",
		"--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)

	var createdCertificate v1.CreatedCertificate
	certificateFile := t.TempDir() + "/certificate.json"
	certificateJSON := `{"name":"mock-cert","certificatePem":"-----BEGIN CERTIFICATE-----\nMIItest\n-----END CERTIFICATE-----","privatekeyPem":"-----BEGIN PRIVATE KEY-----\nMIItest\n-----END PRIVATE KEY-----"}`
	if err := os.WriteFile(certificateFile, []byte(certificateJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "certificate", "create", "--cluster-id", clusterID, "--request", "@"+certificateFile), &createdCertificate); err != nil {
		t.Fatal(err)
	}
	certificateID := jsonString(t, createdCertificate.CertificateID)
	runCommand("apprun-dedicated-api", "certificate", "read", "--cluster-id", clusterID, "--certificate-id", certificateID)
	if err := os.WriteFile(certificateFile, []byte(strings.ReplaceAll(certificateJSON, "mock-cert", "mock-cert-updated")), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommand("apprun-dedicated-api", "certificate", "update", "--cluster-id", clusterID,
		"--certificate-id", certificateID, "--request", "@"+certificateFile)
	runCommand("apprun-dedicated-api", "certificate", "delete", "--cluster-id", clusterID, "--certificate-id", certificateID)
	runCommand("apprun-dedicated-api", "service-class", "list-lb")
	runCommand("apprun-dedicated-api", "service-class", "list-worker")

	var createdApp v1.CreatedApplication
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "application", "create",
		"--name", "mock-app", "--cluster-id", clusterID), &createdApp); err != nil {
		t.Fatal(err)
	}
	appID := jsonString(t, createdApp.ApplicationID)

	var app applicationOutput
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "application", "read", appID), &app); err != nil {
		t.Fatal(err)
	}
	if app.Name != "mock-app" {
		t.Fatalf("application name = %q, want mock-app", app.Name)
	}

	requestPath := t.TempDir() + "/version.json"
	if err := os.WriteFile(requestPath, []byte(`{"cpu":1000,"memory":2048,"scalingMode":"manual","fixedScale":1,"image":"nginx:latest","registryPasswordAction":"keep","exposedPorts":[{"targetPort":80}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var createdVersion v1.ReadApplicationVersionSummary
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "version", "create",
		"--application-id", appID, "--request", "@"+requestPath), &createdVersion); err != nil {
		t.Fatal(err)
	}
	if createdVersion.Version < 1 {
		t.Fatalf("created version = %d, want positive version", createdVersion.Version)
	}

	var versions struct {
		Items []v1.ApplicationVersionDeploymentStatus `json:"items"`
	}
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "version", "list",
		"--application-id", appID, "--max-items", "30"), &versions); err != nil {
		t.Fatal(err)
	}
	if len(versions.Items) != 1 || versions.Items[0].Version != createdVersion.Version {
		t.Fatalf("version list = %#v, want version %d", versions.Items, createdVersion.Version)
	}

	runCommand("apprun-dedicated-api", "application", "update", appID, "--active-version", "1")
	var containers []map[string]any
	if err := json.Unmarshal(runCommand("apprun-dedicated-api", "application", "containers", appID), &containers); err != nil {
		t.Fatal(err)
	}
	runCommand("apprun-dedicated-api", "application", "update", appID, "--deactivate")
	runCommand("apprun-dedicated-api", "version", "delete", "--application-id", appID, "1")
	runCommand("apprun-dedicated-api", "application", "delete", appID)
	runCommand("apprun-dedicated-api", "load-balancer", "delete", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID, "--load-balancer-id", lbID)
	runCommand("apprun-dedicated-api", "auto-scaling-group", "delete", "--cluster-id", clusterID, "--auto-scaling-group-id", groupID)
	runCommand("apprun-dedicated-api", "cluster", "delete", clusterID)

	if violations := server.SpecViolations(); len(violations) != 0 {
		t.Fatalf("sakumock recorded OpenAPI response violations: %+v", violations)
	}
}

type applicationOutput struct {
	Name string `json:"name"`
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Trim(string(data), `"`)
}
