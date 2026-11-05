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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	proxylb "github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	proxylbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	"github.com/sacloud/skr/internal/proxylbapi"
	"github.com/stretchr/testify/require"
)

type proxylbAPITestDouble struct {
	findRequest               *proxylb.FindRequest
	readRequest               *proxylb.ReadRequest
	createRequest             *proxylb.CreateRequest
	updateRequest             *proxylb.UpdateRequest
	deleteRequest             *proxylb.DeleteRequest
	getCertificatesRequest    *proxylb.GetCertificatesRequest
	setCertificatesRequest    *proxylb.SetCertificatesRequest
	deleteCertificatesRequest *proxylb.DeleteCertificatesRequest
	renewRequest              *proxylb.RenewLetsEncryptCertRequest
	healthStatusRequest       *proxylb.HealthStatusRequest
	monitorConnectionRequest  *proxylb.MonitorConnectionRequest
	err                       error
}

func (api *proxylbAPITestDouble) FindWithContext(_ context.Context, request *proxylb.FindRequest) ([]*iaas.ProxyLB, error) {
	api.findRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.ProxyLB{{ID: 123, Name: "test-proxylb"}}, nil
}

func (api *proxylbAPITestDouble) ReadWithContext(_ context.Context, request *proxylb.ReadRequest) (*iaas.ProxyLB, error) {
	api.readRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.ProxyLB{ID: request.ID, Name: "test-proxylb"}, nil
}

func (api *proxylbAPITestDouble) CreateWithContext(_ context.Context, request *proxylb.CreateRequest) (*iaas.ProxyLB, error) {
	api.createRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.ProxyLB{ID: 123, Name: request.Name, FQDN: "123.proxylb.jp"}, nil
}

func (api *proxylbAPITestDouble) UpdateWithContext(_ context.Context, request *proxylb.UpdateRequest) (*iaas.ProxyLB, error) {
	api.updateRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.ProxyLB{ID: request.ID, Name: "test-proxylb"}, nil
}

func (api *proxylbAPITestDouble) DeleteWithContext(_ context.Context, request *proxylb.DeleteRequest) error {
	api.deleteRequest = request
	return api.err
}

func (api *proxylbAPITestDouble) GetCertificatesWithContext(_ context.Context, request *proxylb.GetCertificatesRequest) (*iaas.ProxyLBCertificates, error) {
	api.getCertificatesRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.ProxyLBCertificates{PrimaryCert: &iaas.ProxyLBPrimaryCert{
		ServerCertificate:     "server-cert",
		PrivateKey:            "private-key",
		CertificateCommonName: "example.com",
	}}, nil
}

func (api *proxylbAPITestDouble) SetCertificatesWithContext(_ context.Context, request *proxylb.SetCertificatesRequest) error {
	api.setCertificatesRequest = request
	return api.err
}

func (api *proxylbAPITestDouble) DeleteCertificatesWithContext(_ context.Context, request *proxylb.DeleteCertificatesRequest) error {
	api.deleteCertificatesRequest = request
	return api.err
}

func (api *proxylbAPITestDouble) RenewLetsEncryptCertWithContext(_ context.Context, request *proxylb.RenewLetsEncryptCertRequest) error {
	api.renewRequest = request
	return api.err
}

func (api *proxylbAPITestDouble) HealthStatusWithContext(_ context.Context, request *proxylb.HealthStatusRequest) (*iaas.ProxyLBHealth, error) {
	api.healthStatusRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.ProxyLBHealth{ActiveConn: 3, CPS: 12.5, CurrentVIP: "192.0.2.100", Servers: []*iaas.LoadBalancerServerStatus{
		{ActiveConn: 1, Status: "up", IPAddress: "192.0.2.10", Port: 80},
	}}, nil
}

func (api *proxylbAPITestDouble) MonitorConnectionWithContext(_ context.Context, request *proxylb.MonitorConnectionRequest) ([]*iaas.MonitorConnectionValue, error) {
	api.monitorConnectionRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.MonitorConnectionValue{{ActiveConnections: 4, ConnectionsPerSec: 2}}, nil
}

func setProxyLBAPIFactory(commandLine *cli, factory proxylbapi.APIFactory) {
	commandLine.ProxyLBAPI = proxylbapi.NewCommands(proxylbapi.Runtime{
		DecodeRequest:          decodeRequest,
		DecodeProtectedRequest: decodeProtectedRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
	}, factory)
}

func TestRunProxyLBAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"proxylb-api", "--help"}, []string{"find", "read", "create", "update", "delete",
			"get-certificates", "set-certificates", "delete-certificates", "renew-lets-encrypt-cert",
			"health-status", "monitor-connection", "エンハンスドロードバランサ", "グローバルリソース"}},
		{[]string{"proxylb-api", "create", "--help"}, []string{"--name", "--plan", "--region",
			"--health-check-protocol", "--bind-port", "--server", "--sorry-server-ip",
			"--lets-encrypt-enabled", "--sticky-session", "--gzip", "--backend-http-keep-alive",
			"--proxy-protocol", "--syslog-server", "--timeout-inactive-sec", "--strict-rule",
			"--request", "@path.json", "AddResponseHeader", "proxy-mode=http"}},
		{[]string{"proxylb-api", "update", "--help"}, []string{"--id", "--plan", "省略した設定は変更しません",
			"ヘルスチェック設定全体を置き換える", "一覧全体を置き換えます", "--bind-port", "--server"}},
		{[]string{"proxylb-api", "set-certificates", "--help"}, []string{"--id", "--primary-cert",
			"--primary-key", "--primary-intermediate", "--additional-certs", "@path", "標準入力", "秘密鍵"}},
		{[]string{"proxylb-api", "monitor-connection", "--help"}, []string{"--id", "--start", "--end", "RFC3339"}},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, want := range test.want {
				require.Contains(t, help, want, "help does not contain %q: %s", want, help)
			}
		})
	}
}

func TestProxyLBCommandOperations(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr=%q", args, stderr.String())
		return stdout.Bytes()
	}

	listed := run("proxylb-api", "find", "--name", "test-proxylb", "--tags", "web,production",
		"--count", "10", "--from", "5", "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	err := json.Unmarshal(listed, &projection)
	require.NoError(t, err)
	if len(projection) != 1 || string(projection[0]["Name"]) != `"test-proxylb"` {
		t.Fatalf("find output = %s", listed)
	}
	if api.findRequest == nil || len(api.findRequest.Names) != 1 || api.findRequest.Names[0] != "test-proxylb" ||
		!reflectStrings(api.findRequest.Tags, []string{"web", "production"}) ||
		api.findRequest.Count != 10 || api.findRequest.From != 5 {
		t.Fatalf("find request = %+v", api.findRequest)
	}

	created := run("proxylb-api", "create", "--name", "test-proxylb", "--plan", "100", "--region", "is1",
		"--description", "desc", "--tags", "web", "--icon-id", "9",
		"--health-check-protocol", "http", "--health-check-delay-loop", "10", "--health-check-path", "/health",
		"--sorry-server-ip", "192.0.2.99", "--sorry-server-port", "80",
		"--bind-port", "proxy-mode=http,port=80",
		"--bind-port", "proxy-mode=https,port=443,support-http2=true,ssl-policy=TLS-1-2-2019-04",
		"--server", "ip-address=192.0.2.10,port=80,enabled=true",
		"--server", "ip-address=192.0.2.11,port=80,server-group=group1,proxy-ssl=true",
		"--sticky-session", "--gzip", "--backend-http-keep-alive", "aggressive",
		"--proxy-protocol", "--syslog-server", "192.0.2.50", "--syslog-port", "514",
		"--timeout-inactive-sec", "30", "--monitoring-suite-log-enabled", "--strict-rule")
	var createResult iaas.ProxyLB
	err2 := json.Unmarshal(created, &createResult)
	require.NoError(t, err2)
	if api.createRequest == nil || api.createRequest.Name != "test-proxylb" ||
		api.createRequest.Plan != types.EProxyLBPlan(100) || api.createRequest.Region != types.EProxyLBRegion("is1") ||
		api.createRequest.Description != "desc" || !reflectStrings(api.createRequest.Tags, []string{"web"}) ||
		api.createRequest.HealthCheck == nil || api.createRequest.HealthCheck.Protocol != types.EProxyLBHealthCheckProtocol("http") ||
		api.createRequest.HealthCheck.DelayLoop != 10 || api.createRequest.HealthCheck.Path != "/health" ||
		api.createRequest.SorryServer == nil || api.createRequest.SorryServer.IPAddress != "192.0.2.99" ||
		api.createRequest.SorryServer.Port != 80 ||
		len(api.createRequest.BindPorts) != 2 ||
		api.createRequest.BindPorts[0].ProxyMode != types.EProxyLBProxyMode("http") ||
		api.createRequest.BindPorts[0].Port != 80 ||
		api.createRequest.BindPorts[1].ProxyMode != types.EProxyLBProxyMode("https") ||
		api.createRequest.BindPorts[1].Port != 443 || !api.createRequest.BindPorts[1].SupportHTTP2 ||
		api.createRequest.BindPorts[1].SSLPolicy != "TLS-1-2-2019-04" ||
		len(api.createRequest.Servers) != 2 ||
		api.createRequest.Servers[0].IPAddress != "192.0.2.10" || api.createRequest.Servers[0].Port != 80 || !api.createRequest.Servers[0].Enabled ||
		api.createRequest.Servers[1].ServerGroup != "group1" || !api.createRequest.Servers[1].ProxySSL ||
		api.createRequest.StickySession == nil || !api.createRequest.StickySession.Enabled || api.createRequest.StickySession.Method != "cookie" ||
		api.createRequest.Gzip == nil || !api.createRequest.Gzip.Enabled ||
		api.createRequest.BackendHttpKeepAlive == nil || api.createRequest.BackendHttpKeepAlive.Mode != types.EProxyLBBackendHttpKeepAlive("aggressive") ||
		api.createRequest.ProxyProtocol == nil || !api.createRequest.ProxyProtocol.Enabled ||
		api.createRequest.Syslog == nil || api.createRequest.Syslog.Port != 514 ||
		api.createRequest.Timeout == nil || api.createRequest.Timeout.InactiveSec != 30 ||
		api.createRequest.MonitoringSuiteLog == nil || !api.createRequest.MonitoringSuiteLog.Enabled ||
		api.createRequest.StrictRule == nil || !api.createRequest.StrictRule.Enabled {
		t.Fatalf("create request = %+v", api.createRequest)
	}

	run("proxylb-api", "read", "--id", "123")
	require.NotNil(t, api.readRequest, "read request = %+v", api.readRequest)
	require.EqualValues(t, 123, api.readRequest.ID, "unexpected value")

	run("proxylb-api", "update", "--id", "123", "--description", "", "--plan", "500",
		"--health-check-protocol", "tcp", "--health-check-delay-loop", "20",
		"--bind-port", "proxy-mode=http,port=8080", "--server", "ip-address=192.0.2.12,port=80,enabled=false",
		"--sticky-session=false", "--gzip=false", "--proxy-protocol=false", "--strict-rule=false",
		"--monitoring-suite-log-enabled=false")
	if api.updateRequest == nil || api.updateRequest.ID != 123 || api.updateRequest.Description == nil ||
		*api.updateRequest.Description != "" || api.updateRequest.Plan == nil || *api.updateRequest.Plan != types.EProxyLBPlan(500) ||
		api.updateRequest.HealthCheck == nil || api.updateRequest.HealthCheck.Protocol != types.EProxyLBHealthCheckProtocol("tcp") ||
		api.updateRequest.HealthCheck.DelayLoop != 20 ||
		api.updateRequest.BindPorts == nil || len(*api.updateRequest.BindPorts) != 1 || (*api.updateRequest.BindPorts)[0].Port != 8080 ||
		api.updateRequest.Servers == nil || len(*api.updateRequest.Servers) != 1 || (*api.updateRequest.Servers)[0].IPAddress != "192.0.2.12" || (*api.updateRequest.Servers)[0].Enabled ||
		api.updateRequest.StickySession == nil || *api.updateRequest.StickySession != (iaas.ProxyLBStickySession{Enabled: false, Method: "cookie"}) ||
		api.updateRequest.Gzip == nil || !api.updateRequest.Gzip.Enabled == false ||
		api.updateRequest.ProxyProtocol == nil || api.updateRequest.ProxyProtocol.Enabled ||
		api.updateRequest.MonitoringSuiteLog == nil || api.updateRequest.MonitoringSuiteLog.Enabled ||
		api.updateRequest.StrictRule == nil || api.updateRequest.StrictRule.Enabled {
		t.Fatalf("update request = %+v", api.updateRequest)
	}
	require.Nil(t, api.updateRequest.Name, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.Tags, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.IconID, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.SorryServer, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.LetsEncrypt, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.Syslog, "omitted update fields were populated: %+v", api.updateRequest)
	// 未指定のタイムアウトは空オブジェクトを避けるために既定値が設定される
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, api.updateRequest.Timeout, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.OriginGuard, "omitted update fields were populated: %+v", api.updateRequest)

	run("proxylb-api", "delete", "--id", "123", "--fail-if-not-found")
	require.NotNil(t, api.deleteRequest, "delete request = %+v", api.deleteRequest)
	require.EqualValues(t, 123, api.deleteRequest.ID, "unexpected value")
	require.True(t, api.deleteRequest.FailIfNotFound, "unexpected value")

	run("proxylb-api", "get-certificates", "--id", "123")
	require.NotNil(t, api.getCertificatesRequest, "get-certificates request = %+v", api.getCertificatesRequest)
	require.EqualValues(t, 123, api.getCertificatesRequest.ID, "unexpected value")

	run("proxylb-api", "delete-certificates", "--id", "123")
	require.NotNil(t, api.deleteCertificatesRequest, "delete-certificates request = %+v", api.deleteCertificatesRequest)
	require.EqualValues(t, 123, api.deleteCertificatesRequest.ID, "unexpected value")

	run("proxylb-api", "renew-lets-encrypt-cert", "--id", "123")
	require.NotNil(t, api.renewRequest, "renew-lets-encrypt-cert request = %+v", api.renewRequest)
	require.EqualValues(t, 123, api.renewRequest.ID, "unexpected value")

	run("proxylb-api", "health-status", "--id", "123")
	require.NotNil(t, api.healthStatusRequest, "health-status request = %+v", api.healthStatusRequest)
	require.EqualValues(t, 123, api.healthStatusRequest.ID, "unexpected value")

	run("proxylb-api", "monitor-connection", "--id", "123")
	require.NotNil(t, api.monitorConnectionRequest, "monitor-connection request = %+v", api.monitorConnectionRequest)
	require.EqualValues(t, 123, api.monitorConnectionRequest.ID, "unexpected value")
}

func TestProxyLBMonitorConnectionCondition(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runCLI([]string{"proxylb-api", "monitor-connection", "--id", "123",
		"--start", "2026-01-01T00:00:00Z", "--end", "2026-01-01T01:00:00Z"}, &stdout, &stderr, commandLine),
		"stderr: %s", stderr.String())
	require.NotNil(t, api.monitorConnectionRequest, "monitor-connection request = %+v", api.monitorConnectionRequest)
	require.Equal(t, "2026-01-01T00:00:00Z", api.monitorConnectionRequest.Start.UTC().Format("2006-01-02T15:04:05Z"), "unexpected start")
	require.Equal(t, "2026-01-01T01:00:00Z", api.monitorConnectionRequest.End.UTC().Format("2006-01-02T15:04:05Z"), "unexpected end")

	var stdout2, stderr2 bytes.Buffer
	code := runCLI([]string{"proxylb-api", "monitor-connection", "--id", "123", "--start", "invalid"}, &stdout2, &stderr2, commandLine)
	require.EqualValues(t, 1, code, "code=%d stderr=%q", code, stderr2.String())
	require.Contains(t, stderr2.String(), "RFC3339", "missing expected text %q", "RFC3339")
}

func TestProxyLBJSONInputAndMutualExclusion(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	run := func(args ...string) (int, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		return code, stderr.String()
	}
	code, stderr := run("proxylb-api", "create", "--request",
		`{"Name":"json-proxylb","Plan":100,"Region":"tk1","HealthCheck":{"Protocol":"http","DelayLoop":10,"Path":"/"},"BindPorts":[{"ProxyMode":"http","Port":80}]}`)
	require.Equal(t, 0, code, "JSON create code=%d stderr=%q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if api.createRequest == nil || api.createRequest.Name != "json-proxylb" ||
		api.createRequest.Plan != types.EProxyLBPlan(100) || api.createRequest.Region != types.EProxyLBRegion("tk1") ||
		api.createRequest.HealthCheck == nil || len(api.createRequest.BindPorts) != 1 {
		t.Fatalf("JSON create request = %+v", api.createRequest)
	}
	// 未指定のタイムアウトは既定値が設定される
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, api.createRequest.Timeout, "JSON create request = %+v", api.createRequest)

	code, stderr = run("proxylb-api", "update", "--request",
		`{"ID":123,"Description":"json-updated","HealthCheck":{"Protocol":"tcp","DelayLoop":20},"Rules":[]}`)
	require.Equal(t, 0, code, "JSON update code=%d stderr=%q", code, stderr)
	if api.updateRequest == nil || api.updateRequest.ID != 123 || api.updateRequest.Description == nil ||
		*api.updateRequest.Description != "json-updated" || api.updateRequest.Rules == nil {
		t.Fatalf("JSON update request = %+v", api.updateRequest)
	}
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, api.updateRequest.Timeout, "JSON update request = %+v", api.updateRequest)

	for _, args := range [][]string{
		{"proxylb-api", "create", "--request", `{"Name":"x"}`, "--name", "x"},
		{"proxylb-api", "update", "--request", `{"ID":123}`, "--description", "d"},
		{"proxylb-api", "delete", "--request", `{"ID":123}`, "--fail-if-not-found=false"},
		{"proxylb-api", "monitor-connection", "--request", `{"ID":123}`, "--start", "2026-01-01T00:00:00Z"},
	} {
		code, stderr = run(args...)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stderr %q", args, code, stderr)
		require.Contains(t, stderr, "--request と個別フラグ", "missing expected text %q in %q", "--request と個別フラグ", stderr)
	}

	// set-certificates の --request には秘密鍵が含まれるため直接指定は受け付けず、
	// 保護したファイルまたは標準入力のみを受け付けます
	code, stderr = run("proxylb-api", "set-certificates", "--request", `{"PrimaryCerts":{}}`, "--primary-cert", "@cert.pem")
	require.EqualValues(t, 1, code, "code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--request は @path.json または -", "missing expected text in %q", stderr)

	dir := t.TempDir()
	requestPath := filepath.Join(dir, "certs.json")
	certPath := filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(requestPath, []byte(`{"PrimaryCerts":{"ServerCertificate":"c","PrivateKey":"k"}}`), 0o600))
	require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
	code, stderr = run("proxylb-api", "set-certificates", "--request", "@"+requestPath, "--primary-cert", "@"+certPath)
	require.EqualValues(t, 1, code, "code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--request と個別フラグ", "missing expected text %q in %q", "--request と個別フラグ", stderr)
}

func TestProxyLBCommandErrorsAreReturned(t *testing.T) {
	apiErr := errors.New("ProxyLB API failure")
	api := &proxylbAPITestDouble{err: apiErr}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"proxylb-api", "read", "--id", "123"}, &stdout, &stderr, commandLine)
	if code != 1 || stdout.Len() != 0 || stderr.String() != apiErr.Error()+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestProxyLBFlagValidationErrors(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	for _, args := range [][]string{
		{"proxylb-api", "create", "--plan", "100"},
		{"proxylb-api", "create", "--name", "x"},
		// ヘルスチェックは API が必須とする設定
		{"proxylb-api", "create", "--name", "x", "--plan", "100"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--health-check-protocol", "ping", "--health-check-delay-loop", "10"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--health-check-path", "/"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--health-check-protocol", "http", "--health-check-delay-loop", "10"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--health-check-protocol", "http", "--health-check-delay-loop", "0", "--health-check-path", "/"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--sorry-server-ip", "192.0.2.1"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--region", "tk2"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--backend-http-keep-alive", "fast"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--bind-port", "port=80"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--bind-port", "proxy-mode=https,port=443,redirect-to-https=true"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--bind-port", "proxy-mode=http,port=80,ssl-policy=TLS-1-2-2019-04"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--bind-port", "proxy-mode=http,port=0"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--bind-port", "proxy-mode=http,port=80,unknown=1"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--server", "ip-address=192.0.2.10"},
		{"proxylb-api", "create", "--name", "x", "--plan", "100", "--server", "ip-address=192.0.2.10,port=80,enabled=yes"},
		{"proxylb-api", "update", "--id", "123"},
	} {
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "run(%v) stdout=%q", args, stdout.String())
	}
}

type proxylbAPICaller struct {
	method string
	uri    string
	body   map[string]any
}

func (caller *proxylbAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	caller.method, caller.uri = method, uri
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &caller.body); err != nil {
			return nil, err
		}
	}
	return []byte(`{"CommonServiceItem":{"ID":321,"Name":"sdk-created","Settings":{"ProxyLB":{"HealthCheck":{"Protocol":"http","DelayLoop":10},"BindPorts":[{"ProxyMode":"http","Port":80}],"Servers":[{"IPAddress":"192.0.2.10","Port":80,"Enabled":true,"ServerGroup":"","ProxySSL":false}]}},"Status":{"FQDN":"321.proxylb.jp","Region":"is1"}}}`), nil
}

func TestProxyLBCLIUsesSDKRequestConversion(t *testing.T) {
	caller := &proxylbAPICaller{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return proxylbSDK.New(caller), nil })

	var stdout, stderr bytes.Buffer
	args := []string{
		"proxylb-api", "create", "--name", "sdk-created", "--plan", "100", "--region", "is1",
		"--health-check-protocol", "http", "--health-check-delay-loop", "10", "--health-check-path", "/",
		"--bind-port", "proxy-mode=http,port=80",
		"--server", "ip-address=192.0.2.10,port=80,enabled=true",
	}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.Equal(t, "POST", caller.method, "SDK call = %s %s", caller.method, caller.uri)
	require.Contains(t, caller.uri, "/commonserviceitem", "missing expected text %q", "/commonserviceitem")
	item, ok := caller.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	settings := item["Settings"].(map[string]any)
	proxyLBSettings := settings["ProxyLB"].(map[string]any)
	healthCheck := proxyLBSettings["HealthCheck"].(map[string]any)
	bindPorts := proxyLBSettings["BindPorts"].([]any)
	servers := proxyLBSettings["Servers"].([]any)
	timeout := proxyLBSettings["Timeout"].(map[string]any)
	if item["Name"] != "sdk-created" || healthCheck["Protocol"] != "http" || healthCheck["DelayLoop"] != float64(10) || healthCheck["Path"] != "/" || len(bindPorts) != 1 || len(servers) != 1 || timeout["InactiveSec"] != float64(10) {
		t.Fatalf("converted SDK request = %#v", item)
	}
	bindPort := bindPorts[0].(map[string]any)
	if bindPort["ProxyMode"] != "http" || bindPort["Port"] != float64(80) {
		t.Fatalf("converted bind port = %#v", bindPort)
	}
	server := servers[0].(map[string]any)
	if server["IPAddress"] != "192.0.2.10" || server["Port"] != float64(80) || server["Enabled"] != true {
		t.Fatalf("converted server = %#v", server)
	}
	var result iaas.ProxyLB
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err)
	require.EqualValues(t, 321, result.ID, "decoded SDK result = %+v", result)
	require.Equal(t, "321.proxylb.jp", result.FQDN, "unexpected value")
}

func TestProxyLBSetCertificatesInput(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "key.pem")
	intermediatePath := filepath.Join(dir, "intermediate.pem")
	additionalPath := filepath.Join(dir, "additional.json")
	require.NoError(t, os.WriteFile(certPath, []byte("-----BEGIN CERTIFICATE-----\nserver\n-----END CERTIFICATE-----\n"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n"), 0o600))
	require.NoError(t, os.WriteFile(intermediatePath, []byte("-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----\n"), 0o600))
	additionalJSON := `[{"ServerCertificate":"-----BEGIN CERTIFICATE-----\nadditional\n-----END CERTIFICATE-----","PrivateKey":"-----BEGIN PRIVATE KEY-----\nadditional-key\n-----END PRIVATE KEY-----"}]`
	require.NoError(t, os.WriteFile(additionalPath, []byte(additionalJSON), 0o600))

	var stdout, stderr bytes.Buffer
	args := []string{"proxylb-api", "set-certificates", "--id", "123",
		"--primary-cert", "@" + certPath, "--primary-key", "@" + keyPath,
		"--primary-intermediate", "@" + intermediatePath, "--additional-certs", "@" + additionalPath}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.NotNil(t, api.setCertificatesRequest, "set-certificates request = %+v", api.setCertificatesRequest)
	require.EqualValues(t, 123, api.setCertificatesRequest.ID, "unexpected value")
	require.NotNil(t, api.setCertificatesRequest.PrimaryCerts, "primary certs should be set")
	require.Contains(t, api.setCertificatesRequest.PrimaryCerts.ServerCertificate, "server")
	require.Contains(t, api.setCertificatesRequest.PrimaryCerts.PrivateKey, "key")
	require.Contains(t, api.setCertificatesRequest.PrimaryCerts.IntermediateCertificate, "intermediate")
	require.Len(t, api.setCertificatesRequest.AdditionalCerts, 1)
	require.Contains(t, api.setCertificatesRequest.AdditionalCerts[0].ServerCertificate, "additional")
	require.Contains(t, api.setCertificatesRequest.AdditionalCerts[0].PrivateKey, "additional-key")
}

func TestProxyLBSetCertificatesStdin(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	stdin := os.Stdin
	t.Cleanup(func() { os.Stdin = stdin })
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	fmt.Fprintln(w, "-----BEGIN CERTIFICATE-----")
	fmt.Fprintln(w, "stdin-cert")
	fmt.Fprintln(w, "-----END CERTIFICATE-----")
	require.NoError(t, w.Close())

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(keyPath, []byte("-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n"), 0o600))

	var stdout, stderr bytes.Buffer
	args := []string{"proxylb-api", "set-certificates", "--id", "123",
		"--primary-cert", "-", "--primary-key", "@" + keyPath}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.NotNil(t, api.setCertificatesRequest, "set-certificates request = %+v", api.setCertificatesRequest)
	require.Contains(t, api.setCertificatesRequest.PrimaryCerts.ServerCertificate, "stdin-cert")
	require.Contains(t, api.setCertificatesRequest.PrimaryCerts.PrivateKey, "key")
}

func TestProxyLBSetCertificatesErrors(t *testing.T) {
	api := &proxylbAPITestDouble{}
	commandLine := newCLI()
	setProxyLBAPIFactory(commandLine, func() (proxylbapi.API, error) { return api, nil })

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0o600))

	for _, args := range [][]string{
		{"proxylb-api", "set-certificates", "--id", "123"},
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-cert", "@" + certPath},
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-key", "@" + keyPath},
		// 秘密情報のインライン指定は拒否する
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-cert", "inline-cert", "--primary-key", "@" + keyPath},
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-cert", "@" + certPath, "--primary-key", "inline-key"},
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-cert", "@" + certPath,
			"--primary-key", "@" + keyPath, "--additional-certs", `[{"PrivateKey":"inline"}]`},
		{"proxylb-api", "set-certificates", "--id", "123", "--primary-cert", "@/nonexistent/file.pem",
			"--primary-key", "@" + keyPath},
	} {
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "run(%v) stdout=%q", args, stdout.String())
	}
}
