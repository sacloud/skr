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

package proxylbapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	"github.com/stretchr/testify/require"
)

func TestParseBindPort(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		wantMode  types.EProxyLBProxyMode
		wantPort  int
		wantHTTPS bool
		wantH2    bool
		wantRedir bool
		wantSSL   string
	}{
		{name: "http", input: "proxy-mode=http,port=80", wantMode: types.ProxyLBProxyModes.HTTP, wantPort: 80},
		{name: "https with h2 and ssl-policy", input: "proxy-mode=https,port=443,support-http2=true,ssl-policy=TLS-1-2-2019-04",
			wantMode: types.ProxyLBProxyModes.HTTPS, wantPort: 443, wantH2: true, wantSSL: "TLS-1-2-2019-04"},
		{name: "http with redirect", input: "proxy-mode=http,port=80,redirect-to-https=true",
			wantMode: types.ProxyLBProxyModes.HTTP, wantPort: 80, wantRedir: true},
		{name: "tcp", input: "proxy-mode=tcp,port=8080", wantMode: types.ProxyLBProxyModes.TCP, wantPort: 8080},
	} {
		t.Run(test.name, func(t *testing.T) {
			bindPort, err := parseBindPort(test.input)
			require.NoError(t, err)
			require.Equal(t, test.wantMode, bindPort.ProxyMode, "unexpected proxy-mode")
			require.EqualValues(t, test.wantPort, bindPort.Port, "unexpected port")
			require.Equal(t, test.wantH2, bindPort.SupportHTTP2, "unexpected support-http2")
			require.Equal(t, test.wantRedir, bindPort.RedirectToHTTPS, "unexpected redirect-to-https")
			require.Equal(t, test.wantSSL, bindPort.SSLPolicy, "unexpected ssl-policy")
			require.Empty(t, bindPort.AddResponseHeader, "AddResponseHeader should stay JSON-only")
		})
	}
}

func TestParseBindPortErrors(t *testing.T) {
	for _, input := range []string{
		"port=80",
		"proxy-mode=ftp,port=80",
		"proxy-mode=http",
		"proxy-mode=http,port",
		"proxy-mode=http,port=0",
		"proxy-mode=http,port=65536",
		"proxy-mode=http,port=abc",
		"proxy-mode=http,port=80,unknown=1",
		"proxy-mode=http,port=80,port=443",
		"proxy-mode=http,port=80,redirect-to-https=yes",
		"proxy-mode=https,port=443,redirect-to-https=true",
		"proxy-mode=http,port=80,support-http2=true",
		"proxy-mode=http,port=80,ssl-policy=TLS-1-2-2019-04",
		"proxy-mode=http,port=80,",
		"=http,port=80",
	} {
		_, err := parseBindPort(input)
		require.Error(t, err, "parseBindPort(%q) should fail", input)
	}
}

func TestParseServer(t *testing.T) {
	server, err := parseServer("ip-address=192.0.2.10,port=80,server-group=group1,enabled=true,proxy-ssl=true")
	require.NoError(t, err)
	require.Equal(t, "192.0.2.10", server.IPAddress, "unexpected ip-address")
	require.EqualValues(t, 80, server.Port, "unexpected port")
	require.Equal(t, "group1", server.ServerGroup, "unexpected server-group")
	require.True(t, server.Enabled, "unexpected enabled")
	require.True(t, server.ProxySSL, "unexpected proxy-ssl")

	server, err = parseServer("ip-address=192.0.2.11,port=8080")
	require.NoError(t, err)
	require.Equal(t, "192.0.2.11", server.IPAddress, "unexpected ip-address")
	require.EqualValues(t, 8080, server.Port, "unexpected port")
	require.False(t, server.Enabled, "unexpected enabled")
	require.False(t, server.ProxySSL, "unexpected proxy-ssl")
}

func TestParseServerErrors(t *testing.T) {
	for _, input := range []string{
		"port=80",
		"ip-address=192.0.2.10",
		"ip-address=192.0.2.10,port",
		"ip-address=192.0.2.10,port=0",
		"ip-address=192.0.2.10,port=70000",
		"ip-address=192.0.2.10,port=80,unknown=1",
		"ip-address=192.0.2.10,port=80,port=443",
		"ip-address=192.0.2.10,port=80,enabled=yes",
		"ip-address=192.0.2.10,port=80,proxy-ssl=1",
		"ip-address=192.0.2.10,port=80,server-group=",
	} {
		_, err := parseServer(input)
		require.Error(t, err, "parseServer(%q) should fail", input)
	}
}

func TestCreateRequestFromFlags(t *testing.T) {
	command := &CreateCommand{
		Name:                       ptrString("web-lb"),
		Description:                ptrString("desc"),
		Tags:                       ptrStrings("web", "production"),
		IconID:                     ptrInt64(9),
		Plan:                       ptrInt(100),
		Region:                     ptrString("anycast"),
		UseVIPFailover:             ptrBool(true),
		HealthCheckProtocol:        ptrString("http"),
		HealthCheckDelayLoop:       ptrInt(10),
		HealthCheckHost:            ptrString("example.com"),
		HealthCheckPath:            ptrString("/health"),
		SorryServerIP:              ptrString("192.0.2.99"),
		SorryServerPort:            ptrInt(80),
		BindPorts:                  []string{"proxy-mode=http,port=80", "proxy-mode=https,port=443,support-http2=true"},
		Servers:                    []string{"ip-address=192.0.2.10,port=80,enabled=true", "ip-address=192.0.2.11,port=80,proxy-ssl=true"},
		LetsEncryptEnabled:         ptrBool(true),
		LetsEncryptCommonName:      ptrString("example.com"),
		LetsEncryptSubjectAltNames: ptrStrings("www.example.com"),
		StickySession:              ptrBool(true),
		Gzip:                       ptrBool(true),
		BackendHTTPKeepAlive:       ptrString("safe"),
		ProxyProtocol:              ptrBool(true),
		SyslogServer:               ptrString("192.0.2.50"),
		SyslogPort:                 ptrInt(514),
		TimeoutInactiveSec:         ptrInt(30),
		MonitoringSuiteLogEnabled:  ptrBool(true),
		StrictRule:                 ptrBool(true),
	}

	request, err := createRequestFromFlags(command)
	require.NoError(t, err)
	require.Equal(t, "web-lb", request.Name, "unexpected name")
	require.Equal(t, "desc", request.Description, "unexpected description")
	require.Equal(t, types.Tags{"web", "production"}, request.Tags, "unexpected tags")
	require.EqualValues(t, 9, request.IconID, "unexpected icon-id")
	require.EqualValues(t, 100, request.Plan, "unexpected plan")
	require.Equal(t, types.ProxyLBRegions.Anycast, request.Region, "unexpected region")
	require.True(t, request.UseVIPFailover, "unexpected use-vip-failover")
	require.NotNil(t, request.HealthCheck, "health check should be set")
	require.Equal(t, types.ProxyLBProtocols.HTTP, request.HealthCheck.Protocol, "unexpected protocol")
	require.Equal(t, 10, request.HealthCheck.DelayLoop, "unexpected delay-loop")
	require.Equal(t, "example.com", request.HealthCheck.Host, "unexpected host")
	require.Equal(t, "/health", request.HealthCheck.Path, "unexpected path")
	require.NotNil(t, request.SorryServer, "sorry server should be set")
	require.Equal(t, "192.0.2.99", request.SorryServer.IPAddress, "unexpected sorry server ip")
	require.Equal(t, 80, request.SorryServer.Port, "unexpected sorry server port")
	require.Len(t, request.BindPorts, 2, "unexpected bind ports")
	require.Len(t, request.Servers, 2, "unexpected servers")
	require.True(t, request.Servers[0].Enabled, "unexpected enabled")
	require.True(t, request.Servers[1].ProxySSL, "unexpected proxy-ssl")
	require.NotNil(t, request.LetsEncrypt, "lets encrypt should be set")
	require.True(t, request.LetsEncrypt.Enabled, "unexpected lets-encrypt enabled")
	require.Equal(t, "example.com", request.LetsEncrypt.CommonName, "unexpected lets-encrypt common name")
	require.Equal(t, []string{"www.example.com"}, request.LetsEncrypt.SubjectAltNames, "unexpected SAN")
	require.NotNil(t, request.StickySession, "sticky session should be set")
	require.True(t, request.StickySession.Enabled, "unexpected sticky session enabled")
	require.Equal(t, "cookie", request.StickySession.Method, "unexpected sticky session method")
	require.NotNil(t, request.Gzip, "gzip should be set")
	require.True(t, request.Gzip.Enabled, "unexpected gzip enabled")
	require.NotNil(t, request.BackendHttpKeepAlive, "backend http keep alive should be set")
	require.Equal(t, types.ProxyLBBackendHttpKeepAlive.Safe, request.BackendHttpKeepAlive.Mode, "unexpected keep alive mode")
	require.NotNil(t, request.ProxyProtocol, "proxy protocol should be set")
	require.True(t, request.ProxyProtocol.Enabled, "unexpected proxy protocol enabled")
	require.NotNil(t, request.Syslog, "syslog should be set")
	require.Equal(t, 514, request.Syslog.Port, "unexpected syslog port")
	require.NotNil(t, request.Timeout, "timeout should be set")
	require.Equal(t, 30, request.Timeout.InactiveSec, "unexpected timeout")
	require.NotNil(t, request.MonitoringSuiteLog, "monitoring suite log should be set")
	require.True(t, request.MonitoringSuiteLog.Enabled, "unexpected monitoring suite log enabled")
	require.NotNil(t, request.StrictRule, "strict rule should be set")
	require.True(t, request.StrictRule.Enabled, "unexpected strict rule enabled")
}

func TestUpdateRequestFromFlagsKeepsOmittedFields(t *testing.T) {
	command := &UpdateCommand{
		ID:                        ptrInt64(123),
		Description:               ptrString(""),
		Plan:                      ptrInt(500),
		HealthCheckProtocol:       ptrString("tcp"),
		HealthCheckDelayLoop:      ptrInt(20),
		BindPorts:                 []string{"proxy-mode=http,port=8080"},
		Servers:                   []string{"ip-address=192.0.2.12,port=80,enabled=false"},
		StickySession:             ptrBool(false),
		Gzip:                      ptrBool(false),
		ProxyProtocol:             ptrBool(false),
		StrictRule:                ptrBool(false),
		MonitoringSuiteLogEnabled: ptrBool(false),
	}

	request, err := updateRequestFromFlags(command)
	require.NoError(t, err)
	require.EqualValues(t, 123, request.ID, "unexpected id")
	require.NotNil(t, request.Description, "explicit empty description should be kept")
	require.Equal(t, "", *request.Description, "unexpected description")
	require.NotNil(t, request.Plan, "plan should be set")
	require.EqualValues(t, 500, *request.Plan, "unexpected plan")
	require.NotNil(t, request.HealthCheck, "health check should be set")
	require.Equal(t, 20, request.HealthCheck.DelayLoop, "unexpected delay loop")
	require.Equal(t, types.ProxyLBProtocols.TCP, request.HealthCheck.Protocol, "unexpected protocol")
	require.NotNil(t, request.BindPorts, "bind ports should be set")
	require.Len(t, *request.BindPorts, 1, "unexpected bind ports")
	require.NotNil(t, request.Servers, "servers should be set")
	require.Len(t, *request.Servers, 1, "unexpected servers")
	require.False(t, (*request.Servers)[0].Enabled, "explicit false enabled should be kept")
	require.NotNil(t, request.StickySession, "sticky session should be set")
	require.False(t, request.StickySession.Enabled, "explicit false should be kept")
	require.Equal(t, "cookie", request.StickySession.Method, "method should stay cookie")
	require.False(t, request.Gzip.Enabled, "explicit false should be kept")
	require.False(t, request.ProxyProtocol.Enabled, "explicit false should be kept")
	require.False(t, request.StrictRule.Enabled, "explicit false should be kept")
	require.False(t, request.MonitoringSuiteLog.Enabled, "explicit false should be kept")

	// 省略した設定は nil のまま維持する
	require.Nil(t, request.Name, "omitted fields should stay nil")
	require.Nil(t, request.Tags, "omitted fields should stay nil")
	require.Nil(t, request.IconID, "omitted fields should stay nil")
	require.Nil(t, request.SorryServer, "omitted fields should stay nil")
	require.Nil(t, request.Rules, "omitted fields should stay nil")
	require.Nil(t, request.LetsEncrypt, "omitted fields should stay nil")
	require.Nil(t, request.BackendHttpKeepAlive, "omitted fields should stay nil")
	require.Nil(t, request.Syslog, "omitted fields should stay nil")
	// updateRequestFromFlags はタイムアウトを設定しない。既定値は finalizeUpdateRequest が適用する
	require.Nil(t, request.Timeout, "omitted fields should stay nil")
	require.Nil(t, request.OriginGuard, "omitted fields should stay nil")
}

func TestFinalizeCreateRequest(t *testing.T) {
	// ヘルスチェックは必須
	request := &proxylb.CreateRequest{Name: "web-lb", Plan: types.EProxyLBPlan(100)}
	err := finalizeCreateRequest(request, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--health-check-protocol")

	// JSON 経路のエラーメッセージは API のフィールド名で案内する
	err = finalizeCreateRequest(&proxylb.CreateRequest{Name: "web-lb", Plan: types.EProxyLBPlan(100)}, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API の HealthCheck")

	// 正常系: 未指定のタイムアウトに既定値が設定される
	request = &proxylb.CreateRequest{
		Name:        "web-lb",
		Plan:        types.EProxyLBPlan(100),
		HealthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.HTTP, DelayLoop: 10, Path: "/"},
	}
	require.NoError(t, finalizeCreateRequest(request, true))
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, request.Timeout, "unexpected timeout")

	// 指定済みのタイムアウトは維持される
	request = &proxylb.CreateRequest{
		Name:        "web-lb",
		Plan:        types.EProxyLBPlan(100),
		HealthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.TCP, DelayLoop: 10},
		Timeout:     &iaas.ProxyLBTimeout{InactiveSec: 600},
	}
	require.NoError(t, finalizeCreateRequest(request, true))
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 600}, request.Timeout, "unexpected timeout")
}

func TestFinalizeUpdateRequest(t *testing.T) {
	// ヘルスチェックは必須 (API がヘルスチェックの設定を常に要求するため)
	request := &proxylb.UpdateRequest{ID: types.ID(123)}
	err := finalizeUpdateRequest(request, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--health-check-protocol")

	request = &proxylb.UpdateRequest{
		ID:          types.ID(123),
		HealthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.TCP, DelayLoop: 20},
	}
	require.NoError(t, finalizeUpdateRequest(request, true))
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, request.Timeout, "unexpected timeout")
}

func TestRequireHealthCheck(t *testing.T) {
	tests := []struct {
		name        string
		healthCheck *iaas.ProxyLBHealthCheck
		contains    string
	}{
		{name: "nil", healthCheck: nil, contains: "ヘルスチェックの設定が必要です"},
		{name: "no protocol", healthCheck: &iaas.ProxyLBHealthCheck{DelayLoop: 10}, contains: "プロトコル"},
		{name: "unsupported protocol", healthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.EProxyLBHealthCheckProtocol("ping"), DelayLoop: 10}, contains: "http または tcp"},
		{name: "no delay loop", healthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.HTTP, Path: "/"}, contains: "監視間隔"},
		{name: "http without path", healthCheck: &iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.HTTP, DelayLoop: 10}, contains: "パス"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireHealthCheck(tt.healthCheck, true)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.contains)
			// フラグ経路のエラーメッセージにはフラグ名を含める
			require.Contains(t, err.Error(), "--health-check-")
		})
	}

	// 正常系: tcp はパスが不要
	require.NoError(t, requireHealthCheck(&iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.TCP, DelayLoop: 10}, true))
	// 正常系: http はパスが必要
	require.NoError(t, requireHealthCheck(&iaas.ProxyLBHealthCheck{Protocol: types.ProxyLBProtocols.HTTP, DelayLoop: 10, Path: "/"}, true))
}

func TestNormalizeTimeout(t *testing.T) {
	require.Equal(t, &iaas.ProxyLBTimeout{InactiveSec: 10}, normalizeTimeout(nil), "unexpected default")
	timeout := &iaas.ProxyLBTimeout{InactiveSec: 600}
	require.Same(t, timeout, normalizeTimeout(timeout), "existing timeout should stay")
}

func TestSetCertificatesRequestFromFlags(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "key.pem")
	intermediatePath := filepath.Join(dir, "intermediate.pem")
	additionalPath := filepath.Join(dir, "additional.json")
	require.NoError(t, os.WriteFile(certPath, []byte("-----BEGIN CERTIFICATE-----\nserver\n-----END CERTIFICATE-----\n"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n"), 0o600))
	require.NoError(t, os.WriteFile(intermediatePath, []byte("-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----\n"), 0o600))
	require.NoError(t, os.WriteFile(additionalPath,
		[]byte(`[{"ServerCertificate":"add-cert","IntermediateCertificate":"add-intermediate","PrivateKey":"add-key"}]`), 0o600))

	command := &SetCertificatesCommand{
		ID:                  ptrInt64(123),
		PrimaryCert:         ptrString("@" + certPath),
		PrimaryKey:          ptrString("@" + keyPath),
		PrimaryIntermediate: ptrString("@" + intermediatePath),
		AdditionalCerts:     ptrString("@" + additionalPath),
	}
	request, err := setCertificatesRequestFromFlags(command)
	require.NoError(t, err)
	require.EqualValues(t, 123, request.ID, "unexpected id")
	require.Equal(t, "-----BEGIN CERTIFICATE-----\nserver\n-----END CERTIFICATE-----\n", request.PrimaryCerts.ServerCertificate, "unexpected server certificate")
	require.Equal(t, "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n", request.PrimaryCerts.PrivateKey, "unexpected private key")
	require.Equal(t, "-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----\n", request.PrimaryCerts.IntermediateCertificate, "unexpected intermediate certificate")
	require.Len(t, request.AdditionalCerts, 1, "unexpected additional certs")
	require.Equal(t, "add-cert", request.AdditionalCerts[0].ServerCertificate, "unexpected additional server certificate")
	require.Equal(t, "add-key", request.AdditionalCerts[0].PrivateKey, "unexpected additional private key")
}

func TestSetCertificatesRequestFromFlagsErrors(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "key.pem")
	badJSONPath := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0o600))
	require.NoError(t, os.WriteFile(badJSONPath, []byte(`not json`), 0o600))

	for _, test := range []struct {
		name    string
		command *SetCertificatesCommand
	}{
		{name: "missing both", command: &SetCertificatesCommand{ID: ptrInt64(123)}},
		{name: "missing key", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryCert: ptrString("@" + certPath)}},
		{name: "missing cert", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryKey: ptrString("@" + keyPath)}},
		{name: "inline cert", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryCert: ptrString("inline"), PrimaryKey: ptrString("@" + keyPath)}},
		{name: "inline key", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryCert: ptrString("@" + certPath), PrimaryKey: ptrString("inline")}},
		{name: "empty @", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryCert: ptrString("@"), PrimaryKey: ptrString("@" + keyPath)}},
		{name: "missing file", command: &SetCertificatesCommand{ID: ptrInt64(123), PrimaryCert: ptrString("@/nonexistent/file.pem"), PrimaryKey: ptrString("@" + keyPath)}},
		{name: "invalid additional certs JSON", command: &SetCertificatesCommand{ID: ptrInt64(123),
			PrimaryCert: ptrString("@" + certPath), PrimaryKey: ptrString("@" + keyPath), AdditionalCerts: ptrString("@" + badJSONPath)}},
		{name: "inline additional certs", command: &SetCertificatesCommand{ID: ptrInt64(123),
			PrimaryCert: ptrString("@" + certPath), PrimaryKey: ptrString("@" + keyPath), AdditionalCerts: ptrString(`[{"PrivateKey":"inline"}]`)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := setCertificatesRequestFromFlags(test.command)
			require.Error(t, err, "should fail")
		})
	}
}

func TestBuildMonitorConnectionCondition(t *testing.T) {
	request := &proxylb.MonitorConnectionRequest{}
	command := &MonitorConnectionCommand{
		Start: ptrString("2026-01-01T00:00:00+09:00"),
		End:   ptrString("2026-01-01T01:00:00+09:00"),
	}
	require.NoError(t, buildMonitorConnectionCondition(command, request))
	expectedStart, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00+09:00")
	require.NoError(t, err)
	expectedEnd, err := time.Parse(time.RFC3339, "2026-01-01T01:00:00+09:00")
	require.NoError(t, err)
	require.Equal(t, expectedStart, request.Start, "unexpected start")
	require.Equal(t, expectedEnd, request.End, "unexpected end")

	command = &MonitorConnectionCommand{Start: ptrString("not-a-time")}
	err = buildMonitorConnectionCondition(command, request)
	require.Error(t, err, "should fail for invalid time")

	// フラグ未指定時は何も設定しない
	request = &proxylb.MonitorConnectionRequest{}
	require.NoError(t, buildMonitorConnectionCondition(&MonitorConnectionCommand{}, request))
	require.True(t, request.Start.IsZero(), "start should stay zero")
	require.True(t, request.End.IsZero(), "end should stay zero")
}

func TestBuildHealthCheckRequiresProtocolAndDelayLoop(t *testing.T) {
	_, set, err := buildHealthCheck(nil, nil, nil, nil)
	require.NoError(t, err)
	require.False(t, set, "no flags means no health check")

	_, set, err = buildHealthCheck(nil, ptrInt(10), nil, nil)
	require.Error(t, err, "missing protocol should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildHealthCheck(ptrString("http"), nil, nil, nil)
	require.Error(t, err, "missing delay-loop should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildHealthCheck(ptrString("ping"), ptrInt(10), nil, nil)
	require.Error(t, err, "unsupported protocol should fail")
	require.True(t, set, "set should be true")
}

func TestBuildSorryServerRequiresBothValues(t *testing.T) {
	_, set, err := buildSorryServer(ptrString("192.0.2.99"), nil)
	require.Error(t, err, "missing port should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildSorryServer(nil, ptrInt(80))
	require.Error(t, err, "missing ip should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildSorryServer(nil, nil)
	require.NoError(t, err)
	require.False(t, set, "no flags means no sorry server")
}

func TestBuildLetsEncryptRequiresEnabledAndCommonName(t *testing.T) {
	_, set, err := buildLetsEncrypt(nil, ptrString("example.com"), nil)
	require.Error(t, err, "missing enabled should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildLetsEncrypt(ptrBool(true), nil, nil)
	require.Error(t, err, "missing common name should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildLetsEncrypt(nil, nil, nil)
	require.NoError(t, err)
	require.False(t, set, "no flags means no lets encrypt")
}

func TestBuildSyslogRequiresBothValues(t *testing.T) {
	_, set, err := buildSyslog(ptrString("192.0.2.50"), nil)
	require.Error(t, err, "missing port should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildSyslog(nil, ptrInt(514))
	require.Error(t, err, "missing server should fail")
	require.True(t, set, "set should be true")

	_, set, err = buildSyslog(nil, nil)
	require.NoError(t, err)
	require.False(t, set, "no flags means no syslog")
}

func TestParsePortRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "65536", "abc", ""} {
		_, err := parsePort(value)
		require.Error(t, err, "parsePort(%q) should fail", value)
	}
	port, err := parsePort("65535")
	require.NoError(t, err)
	require.EqualValues(t, 65535, port, "unexpected port")
}

func ptrString(value string) *string { return &value }
func ptrInt(value int) *int          { return &value }
func ptrInt64(value int64) *int64    { return &value }
func ptrBool(value bool) *bool       { return &value }
func ptrStrings(values ...string) *[]string {
	if len(values) == 0 {
		values = []string{}
	}
	return &values
}
