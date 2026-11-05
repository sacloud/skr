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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
)

func createRequestFromFlags(command *CreateCommand) (*proxylb.CreateRequest, error) {
	request := &proxylb.CreateRequest{
		Name: *command.Name,
		Plan: types.EProxyLBPlan(*command.Plan),
	}
	if command.Description != nil {
		request.Description = *command.Description
	}
	if command.Tags != nil {
		request.Tags = types.Tags(*command.Tags)
	}
	if command.IconID != nil {
		request.IconID = types.ID(*command.IconID)
	}
	if command.Region != nil {
		if !slices.Contains(types.ProxyLBRegionStrings, *command.Region) {
			return nil, fmt.Errorf("unsupported --region; choose tk1, is1, or anycast")
		}
		request.Region = types.EProxyLBRegion(*command.Region)
	}
	if command.UseVIPFailover != nil {
		request.UseVIPFailover = *command.UseVIPFailover
	}

	healthCheck, set, err := buildHealthCheck(command.HealthCheckProtocol, command.HealthCheckDelayLoop,
		command.HealthCheckHost, command.HealthCheckPath)
	if err != nil {
		return nil, err
	}
	if set {
		request.HealthCheck = healthCheck
	}

	sorryServer, set, err := buildSorryServer(command.SorryServerIP, command.SorryServerPort)
	if err != nil {
		return nil, err
	}
	if set {
		request.SorryServer = sorryServer
	}

	bindPorts, err := parseBindPorts(command.BindPorts)
	if err != nil {
		return nil, err
	}
	request.BindPorts = bindPorts

	servers, err := parseServers(command.Servers)
	if err != nil {
		return nil, err
	}
	request.Servers = servers

	letsEncrypt, set, err := buildLetsEncrypt(command.LetsEncryptEnabled, command.LetsEncryptCommonName,
		command.LetsEncryptSubjectAltNames)
	if err != nil {
		return nil, err
	}
	if set {
		request.LetsEncrypt = letsEncrypt
	}

	if command.StickySession != nil {
		request.StickySession = &iaas.ProxyLBStickySession{Enabled: *command.StickySession, Method: "cookie"}
	}
	if command.Gzip != nil {
		request.Gzip = &iaas.ProxyLBGzip{Enabled: *command.Gzip}
	}
	if command.BackendHTTPKeepAlive != nil {
		if !slices.Contains(types.ProxyLBBackendHttpKeepAliveStrings, *command.BackendHTTPKeepAlive) {
			return nil, fmt.Errorf("unsupported --backend-http-keep-alive; choose safe or aggressive")
		}
		request.BackendHttpKeepAlive = &iaas.ProxyLBBackendHttpKeepAlive{
			Mode: types.EProxyLBBackendHttpKeepAlive(*command.BackendHTTPKeepAlive),
		}
	}
	if command.ProxyProtocol != nil {
		request.ProxyProtocol = &iaas.ProxyLBProxyProtocol{Enabled: *command.ProxyProtocol}
	}
	syslog, set, err := buildSyslog(command.SyslogServer, command.SyslogPort)
	if err != nil {
		return nil, err
	}
	if set {
		request.Syslog = syslog
	}
	if command.TimeoutInactiveSec != nil {
		request.Timeout = &iaas.ProxyLBTimeout{InactiveSec: *command.TimeoutInactiveSec}
	}
	if command.MonitoringSuiteLogEnabled != nil {
		request.MonitoringSuiteLog = &iaas.MonitoringSuiteLog{Enabled: *command.MonitoringSuiteLogEnabled}
	}
	if command.StrictRule != nil {
		request.StrictRule = &iaas.ProxyLBStrictRule{Enabled: *command.StrictRule}
	}
	return request, nil
}

func updateRequestFromFlags(command *UpdateCommand) (*proxylb.UpdateRequest, error) {
	request := &proxylb.UpdateRequest{ID: types.ID(*command.ID)}
	if command.Name != nil {
		request.Name = command.Name
	}
	if command.Description != nil {
		request.Description = command.Description
	}
	if command.Tags != nil {
		tags := types.Tags(*command.Tags)
		request.Tags = &tags
	}
	if command.IconID != nil {
		iconID := types.ID(*command.IconID)
		request.IconID = &iconID
	}
	if command.Plan != nil {
		plan := types.EProxyLBPlan(*command.Plan)
		request.Plan = &plan
	}

	healthCheck, set, err := buildHealthCheck(command.HealthCheckProtocol, command.HealthCheckDelayLoop,
		command.HealthCheckHost, command.HealthCheckPath)
	if err != nil {
		return nil, err
	}
	if set {
		request.HealthCheck = healthCheck
	}

	sorryServer, set, err := buildSorryServer(command.SorryServerIP, command.SorryServerPort)
	if err != nil {
		return nil, err
	}
	if set {
		request.SorryServer = sorryServer
	}

	if command.BindPorts != nil {
		bindPorts, err := parseBindPorts(command.BindPorts)
		if err != nil {
			return nil, err
		}
		request.BindPorts = &bindPorts
	}
	if command.Servers != nil {
		servers, err := parseServers(command.Servers)
		if err != nil {
			return nil, err
		}
		request.Servers = &servers
	}

	letsEncrypt, set, err := buildLetsEncrypt(command.LetsEncryptEnabled, command.LetsEncryptCommonName,
		command.LetsEncryptSubjectAltNames)
	if err != nil {
		return nil, err
	}
	if set {
		request.LetsEncrypt = letsEncrypt
	}

	if command.StickySession != nil {
		request.StickySession = &iaas.ProxyLBStickySession{Enabled: *command.StickySession, Method: "cookie"}
	}
	if command.Gzip != nil {
		request.Gzip = &iaas.ProxyLBGzip{Enabled: *command.Gzip}
	}
	if command.BackendHTTPKeepAlive != nil {
		if !slices.Contains(types.ProxyLBBackendHttpKeepAliveStrings, *command.BackendHTTPKeepAlive) {
			return nil, fmt.Errorf("unsupported --backend-http-keep-alive; choose safe or aggressive")
		}
		request.BackendHttpKeepAlive = &iaas.ProxyLBBackendHttpKeepAlive{
			Mode: types.EProxyLBBackendHttpKeepAlive(*command.BackendHTTPKeepAlive),
		}
	}
	if command.ProxyProtocol != nil {
		request.ProxyProtocol = &iaas.ProxyLBProxyProtocol{Enabled: *command.ProxyProtocol}
	}
	syslog, set, err := buildSyslog(command.SyslogServer, command.SyslogPort)
	if err != nil {
		return nil, err
	}
	if set {
		request.Syslog = syslog
	}
	if command.TimeoutInactiveSec != nil {
		request.Timeout = &iaas.ProxyLBTimeout{InactiveSec: *command.TimeoutInactiveSec}
	}
	if command.MonitoringSuiteLogEnabled != nil {
		request.MonitoringSuiteLog = &iaas.MonitoringSuiteLog{Enabled: *command.MonitoringSuiteLogEnabled}
	}
	if command.StrictRule != nil {
		request.StrictRule = &iaas.ProxyLBStrictRule{Enabled: *command.StrictRule}
	}
	return request, nil
}

// finalizeCreateRequest は API に送る前にヘルスチェックの必須項目を検証し、
// 未指定のタイムアウトに既定値を適用する。flags が true の場合はエラーメッセージに個別フラグ名を含める。
func finalizeCreateRequest(request *proxylb.CreateRequest, flags bool) error {
	if err := requireHealthCheck(request.HealthCheck, flags); err != nil {
		return err
	}
	request.Timeout = normalizeTimeout(request.Timeout)
	return nil
}

// finalizeUpdateRequest は API に送る前にヘルスチェックの必須項目を検証し、
// 未指定のタイムアウトに既定値を適用する。flags が true の場合はエラーメッセージに個別フラグ名を含める。
func finalizeUpdateRequest(request *proxylb.UpdateRequest, flags bool) error {
	if err := requireHealthCheck(request.HealthCheck, flags); err != nil {
		return err
	}
	request.Timeout = normalizeTimeout(request.Timeout)
	return nil
}

// requireHealthCheck は API が必須とするヘルスチェックの設定を検証する。
// プロトコルと監視間隔は常に必須で、プロトコルが http の場合は監視パスも必要。
func requireHealthCheck(hc *iaas.ProxyLBHealthCheck, flags bool) error {
	hint := " (API の HealthCheck: Protocol、DelayLoop、http の場合は Path)"
	if flags {
		hint = " (--health-check-protocol、--health-check-delay-loop、http の場合は --health-check-path)"
	}
	if hc == nil {
		return fmt.Errorf("ヘルスチェックの設定が必要です%s", hint)
	}
	if hc.Protocol == "" {
		return fmt.Errorf("ヘルスチェックのプロトコルが指定されていません%s", hint)
	}
	if !slices.Contains(types.ProxyLBProtocolStrings, string(hc.Protocol)) {
		return fmt.Errorf("ヘルスチェックのプロトコルは http または tcp で指定してください%s", hint)
	}
	if hc.DelayLoop == 0 {
		return fmt.Errorf("ヘルスチェックの監視間隔が指定されていません%s", hint)
	}
	if hc.Protocol == types.ProxyLBProtocols.HTTP && hc.Path == "" {
		return fmt.Errorf("プロトコルが http の場合はヘルスチェックのパスが必要です%s", hint)
	}
	return nil
}

// normalizeTimeout は未指定のタイムアウトに InactiveSec 10 を適用する。
// 空の Timeout オブジェクトは API に拒否されるため、常に有効な値を送る。
// 10 は SDK の ProxyLBTimeout.GetInactiveSec() が未指定時に返す値。
func normalizeTimeout(t *iaas.ProxyLBTimeout) *iaas.ProxyLBTimeout {
	if t == nil {
		return &iaas.ProxyLBTimeout{InactiveSec: 10}
	}
	return t
}

func buildHealthCheck(protocol *string, delayLoop *int, host, path *string) (*iaas.ProxyLBHealthCheck, bool, error) {
	set := protocol != nil || delayLoop != nil || host != nil || path != nil
	if !set {
		return nil, false, nil
	}
	if protocol == nil {
		return nil, true, fmt.Errorf("--health-check-protocol が必要です")
	}
	if !slices.Contains(types.ProxyLBProtocolStrings, *protocol) {
		return nil, true, fmt.Errorf("unsupported --health-check-protocol; choose http or tcp")
	}
	if delayLoop == nil {
		return nil, true, fmt.Errorf("--health-check-delay-loop が必要です")
	}

	result := &iaas.ProxyLBHealthCheck{
		Protocol:  types.EProxyLBHealthCheckProtocol(*protocol),
		DelayLoop: *delayLoop,
	}
	if host != nil {
		result.Host = *host
	}
	if path != nil {
		result.Path = *path
	}
	return result, true, nil
}

func buildSorryServer(ip *string, port *int) (*iaas.ProxyLBSorryServer, bool, error) {
	set := ip != nil || port != nil
	if !set {
		return nil, false, nil
	}
	if ip == nil {
		return nil, true, fmt.Errorf("--sorry-server-ip が必要です")
	}
	if port == nil {
		return nil, true, fmt.Errorf("--sorry-server-port が必要です")
	}
	return &iaas.ProxyLBSorryServer{IPAddress: *ip, Port: *port}, true, nil
}

func buildLetsEncrypt(enabled *bool, commonName *string, subjectAltNames *[]string) (*iaas.ProxyLBACMESetting, bool, error) {
	set := enabled != nil || commonName != nil || subjectAltNames != nil
	if !set {
		return nil, false, nil
	}
	if commonName == nil {
		return nil, true, fmt.Errorf("--lets-encrypt-common-name が必要です")
	}
	if enabled == nil {
		return nil, true, fmt.Errorf("--lets-encrypt-enabled が必要です")
	}
	result := &iaas.ProxyLBACMESetting{
		Enabled:    *enabled,
		CommonName: *commonName,
	}
	if subjectAltNames != nil {
		result.SubjectAltNames = *subjectAltNames
	}
	return result, true, nil
}

func buildSyslog(server *string, port *int) (*iaas.ProxyLBSyslog, bool, error) {
	set := server != nil || port != nil
	if !set {
		return nil, false, nil
	}
	if server == nil {
		return nil, true, fmt.Errorf("--syslog-server が必要です")
	}
	if port == nil {
		return nil, true, fmt.Errorf("--syslog-port が必要です")
	}
	return &iaas.ProxyLBSyslog{Server: *server, Port: *port}, true, nil
}

func parseBindPorts(inputs []string) ([]*iaas.ProxyLBBindPort, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]*iaas.ProxyLBBindPort, 0, len(inputs))
	for i, input := range inputs {
		bindPort, err := parseBindPort(input)
		if err != nil {
			return nil, fmt.Errorf("--bind-port %d: %w", i+1, err)
		}
		result = append(result, bindPort)
	}
	return result, nil
}

func parseBindPort(input string) (*iaas.ProxyLBBindPort, error) {
	fields := strings.Split(input, ",")
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("expected non-empty key=value entries")
		}
		switch key {
		case "proxy-mode", "port", "redirect-to-https", "support-http2", "ssl-policy":
		default:
			return nil, fmt.Errorf("unknown key %q", key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	mode, ok := values["proxy-mode"]
	if !ok {
		return nil, fmt.Errorf("proxy-mode is required")
	}
	if !slices.Contains(types.ProxyLBProxyModeStrings, mode) {
		return nil, fmt.Errorf("proxy-mode must be http, https, or tcp")
	}
	port, ok := values["port"]
	if !ok {
		return nil, fmt.Errorf("port is required")
	}
	portNumber, err := parsePort(port)
	if err != nil {
		return nil, err
	}

	bindPort := &iaas.ProxyLBBindPort{
		ProxyMode: types.EProxyLBProxyMode(mode),
		Port:      portNumber,
	}
	if value, exists := values["redirect-to-https"]; exists {
		if mode != "http" {
			return nil, fmt.Errorf("redirect-to-https is only valid with proxy-mode=http")
		}
		enabled, err := parseBool(value, "redirect-to-https")
		if err != nil {
			return nil, err
		}
		bindPort.RedirectToHTTPS = enabled
	}
	if value, exists := values["support-http2"]; exists {
		if mode != "https" {
			return nil, fmt.Errorf("support-http2 is only valid with proxy-mode=https")
		}
		enabled, err := parseBool(value, "support-http2")
		if err != nil {
			return nil, err
		}
		bindPort.SupportHTTP2 = enabled
	}
	if value, exists := values["ssl-policy"]; exists {
		if mode != "https" {
			return nil, fmt.Errorf("ssl-policy is only valid with proxy-mode=https")
		}
		bindPort.SSLPolicy = value
	}
	return bindPort, nil
}

func parseServers(inputs []string) ([]*iaas.ProxyLBServer, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]*iaas.ProxyLBServer, 0, len(inputs))
	for i, input := range inputs {
		server, err := parseServer(input)
		if err != nil {
			return nil, fmt.Errorf("--server %d: %w", i+1, err)
		}
		result = append(result, server)
	}
	return result, nil
}

func parseServer(input string) (*iaas.ProxyLBServer, error) {
	fields := strings.Split(input, ",")
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("expected non-empty key=value entries")
		}
		switch key {
		case "ip-address", "port", "server-group", "enabled", "proxy-ssl":
		default:
			return nil, fmt.Errorf("unknown key %q", key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	address, ok := values["ip-address"]
	if !ok {
		return nil, fmt.Errorf("ip-address is required")
	}
	port, ok := values["port"]
	if !ok {
		return nil, fmt.Errorf("port is required")
	}
	portNumber, err := parsePort(port)
	if err != nil {
		return nil, err
	}

	server := &iaas.ProxyLBServer{
		IPAddress: address,
		Port:      portNumber,
	}
	if value, exists := values["server-group"]; exists {
		server.ServerGroup = value
	}
	if value, exists := values["enabled"]; exists {
		enabled, err := parseBool(value, "enabled")
		if err != nil {
			return nil, err
		}
		server.Enabled = enabled
	}
	if value, exists := values["proxy-ssl"]; exists {
		enabled, err := parseBool(value, "proxy-ssl")
		if err != nil {
			return nil, err
		}
		server.ProxySSL = enabled
	}
	return server, nil
}

func setCertificatesRequestFromFlags(command *SetCertificatesCommand) (*proxylb.SetCertificatesRequest, error) {
	if command.PrimaryCert == nil {
		return nil, fmt.Errorf("--primary-cert と --primary-key が必要です")
	}
	if command.PrimaryKey == nil {
		return nil, fmt.Errorf("--primary-cert と --primary-key が必要です")
	}
	primaryCert, err := readProtectedFile(*command.PrimaryCert, "--primary-cert")
	if err != nil {
		return nil, err
	}
	privateKey, err := readProtectedFile(*command.PrimaryKey, "--primary-key")
	if err != nil {
		return nil, err
	}

	request := &proxylb.SetCertificatesRequest{
		ID: types.ID(*command.ID),
		PrimaryCerts: &iaas.ProxyLBPrimaryCert{
			ServerCertificate: primaryCert,
			PrivateKey:        privateKey,
		},
	}
	if command.PrimaryIntermediate != nil {
		intermediate, err := readProtectedFile(*command.PrimaryIntermediate, "--primary-intermediate")
		if err != nil {
			return nil, err
		}
		request.PrimaryCerts.IntermediateCertificate = intermediate
	}
	if command.AdditionalCerts != nil {
		additional, err := loadAdditionalCerts(*command.AdditionalCerts)
		if err != nil {
			return nil, err
		}
		request.AdditionalCerts = additional
	}
	return request, nil
}

func loadAdditionalCerts(input string) ([]*iaas.ProxyLBAdditionalCert, error) {
	var certs []*iaas.ProxyLBAdditionalCert
	if err := decodeProtectedJSON(input, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

// readProtectedFile は秘密情報を含む PEM を @path または -（標準入力）から読み込みます。
// 秘密情報を引数に直接指定させないため、直接指定は受け付けません。
func readProtectedFile(input, flag string) (string, error) {
	data, err := readProtectedBytes(input, flag)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodeProtectedJSON は秘密情報を含む可能性がある JSON を @path または -（標準入力）から読み込みます。
// 直接指定は受け付けません。
func decodeProtectedJSON(input string, destination any) error {
	data, err := readProtectedBytes(input, "--additional-certs")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("JSON を読み込めません: %w", err)
	}
	return nil
}

func readProtectedBytes(input, flag string) ([]byte, error) {
	switch {
	case input == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("%s: 標準入力を読み込めません: %w", flag, err)
		}
		return data, nil
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		if path == "" {
			return nil, fmt.Errorf("%s: ファイルパスが空です", flag)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- パスはユーザーが明示的に指定する。
		if err != nil {
			return nil, fmt.Errorf("%s: ファイルを読み込めません: %w", flag, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("%s は秘密情報を含むため、@path または -（標準入力）で指定してください", flag)
	}
}

func buildMonitorConnectionCondition(command *MonitorConnectionCommand, request *proxylb.MonitorConnectionRequest) error {
	if command.Start == nil && command.End == nil {
		return nil
	}
	start, err := parseTime(command.Start, "--start")
	if err != nil {
		return err
	}
	end, err := parseTime(command.End, "--end")
	if err != nil {
		return err
	}
	if start != nil {
		request.Start = *start
	}
	if end != nil {
		request.End = *end
	}
	return nil
}

func parseTime(value *string, flag string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, fmt.Errorf("%s は RFC3339 形式 (例: 2026-01-01T00:00:00Z) で指定してください: %w", flag, err)
	}
	return &parsed, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be an integer from 1 through 65535")
	}
	return port, nil
}

func parseBool(value, flag string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", flag)
	}
}
