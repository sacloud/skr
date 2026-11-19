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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/dns"
	"github.com/sacloud/skr/internal/dnsapi"
	"github.com/stretchr/testify/require"
)

type dnsAPITestDouble struct {
	findRequest   *dns.FindRequest
	readRequest   *dns.ReadRequest
	createRequest *dns.CreateRequest
	updateRequest *dns.UpdateRequest
	deleteRequest *dns.DeleteRequest
	err           error
}

func (api *dnsAPITestDouble) FindWithContext(_ context.Context, request *dns.FindRequest) ([]*iaas.DNS, error) {
	api.findRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.DNS{{ID: 123, Name: "example.com"}}, nil
}

func (api *dnsAPITestDouble) ReadWithContext(_ context.Context, request *dns.ReadRequest) (*iaas.DNS, error) {
	api.readRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.DNS{ID: request.ID, Name: "example.com", Records: iaas.DNSRecords{
		{Name: "www", Type: types.EDNSRecordType("A"), RData: "192.0.2.1", TTL: 3600},
	}}, nil
}

func (api *dnsAPITestDouble) CreateWithContext(_ context.Context, request *dns.CreateRequest) (*iaas.DNS, error) {
	api.createRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.DNS{ID: 123, Name: request.Name}, nil
}

func (api *dnsAPITestDouble) UpdateWithContext(_ context.Context, request *dns.UpdateRequest) (*iaas.DNS, error) {
	api.updateRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.DNS{ID: request.ID, Name: "example.com"}, nil
}

func (api *dnsAPITestDouble) DeleteWithContext(_ context.Context, request *dns.DeleteRequest) error {
	api.deleteRequest = request
	return api.err
}

func setDNSAPIFactory(commandLine *cli, factory dnsapi.APIFactory) {
	commandLine.DNSAPI = dnsapi.NewCommands(dnsapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, factory)
}

func TestRunDNSAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"dns-api", "--help"}, []string{"find", "read", "create", "update", "delete", "DNS ゾーン"}},
		{[]string{"dns-api", "create", "--help"}, []string{"--name", "--record", "--request", "@path.json",
			"name=レコード名,type=レコード種別,rdata=レコード値,ttl=秒", "A, AAAA, ALIAS, CNAME"}},
		{[]string{"dns-api", "update", "--help"}, []string{"--id", "--record", "一覧全体を置き換えます",
			"保持したいレコードも再指定", "ゾーン名 (Name) は変更できません", "--monitoring-suite-log-enabled"}},
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

func TestDNSCommandOperations(t *testing.T) {
	api := &dnsAPITestDouble{}
	commandLine := newCLI()
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr=%q", args, stderr.String())
		return stdout.Bytes()
	}

	listed := run("dns-api", "find", "--name", "example", "--tags", "web,production", "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	err := json.Unmarshal(listed, &projection)
	require.NoError(t, err)
	require.Len(t, projection, 1, "find output = %s", listed)
	require.JSONEq(t, `"example.com"`, string(projection[0]["Name"]), "find output = %s", listed)

	require.NotNil(t, api.findRequest, "find request = %+v", api.findRequest)
	require.Equal(t, []string{"example"}, api.findRequest.Names, "find request = %+v", api.findRequest)
	require.True(t, reflectStrings(api.findRequest.Tags, []string{"web", "production"}), "find request = %+v", api.findRequest)

	created := run("dns-api", "create", "--name", "example.com",
		"--record", "name=www,type=A,rdata=192.0.2.1,ttl=3600",
		"--record", "name=example.com,type=MX,rdata=10 mx.example.com.,ttl=3600")
	var createResult iaas.DNS
	err = json.Unmarshal(created, &createResult)
	require.NoError(t, err)
	require.EqualValues(t, 123, createResult.ID, "create output = %s", created)

	require.NotNil(t, api.createRequest, "create request = %+v", api.createRequest)
	require.Equal(t, "example.com", api.createRequest.Name, "create request = %+v", api.createRequest)
	require.Len(t, api.createRequest.Records, 2, "create request = %+v", api.createRequest)
	require.Equal(t, "www", api.createRequest.Records[0].Name, "create request = %+v", api.createRequest)
	require.Equal(t, types.EDNSRecordType("A"), api.createRequest.Records[0].Type, "create request = %+v", api.createRequest)
	require.Equal(t, "192.0.2.1", api.createRequest.Records[0].RData, "create request = %+v", api.createRequest)
	require.Equal(t, 3600, api.createRequest.Records[0].TTL, "create request = %+v", api.createRequest)
	// レコード値は字義どおり扱い、空白を含む MX の値もそのまま渡す
	require.Equal(t, "10 mx.example.com.", api.createRequest.Records[1].RData, "create request = %+v", api.createRequest)

	// レコードなしでも空配列を送る
	run("dns-api", "create", "--name", "empty.example.com")
	require.NotNil(t, api.createRequest, "create request = %+v", api.createRequest)
	require.Equal(t, "empty.example.com", api.createRequest.Name, "create request = %+v", api.createRequest)
	require.NotNil(t, api.createRequest.Records, "records should be non-nil empty: %+v", api.createRequest.Records)
	require.Empty(t, api.createRequest.Records, "records should be empty: %+v", api.createRequest.Records)

	read := run("dns-api", "read", "--id", "123")
	var readResult iaas.DNS
	err = json.Unmarshal(read, &readResult)
	require.NoError(t, err)
	require.EqualValues(t, 123, readResult.ID, "read output = %s", read)
	require.Len(t, readResult.Records, 1, "read output = %s", read)
	require.Equal(t, types.EDNSRecordType("A"), readResult.Records[0].Type, "read output = %s", read)

	// --record を指定するとレコード一覧全体を置き換える。未指定の項目は変更しない
	run("dns-api", "update", "--id", "123", "--description", "updated",
		"--record", "name=www,type=A,rdata=192.0.2.2,ttl=600",
		"--record", "name=example.com,type=TXT,rdata=skr-e2e-test,ttl=600",
		"--monitoring-suite-log-enabled=false")
	require.NotNil(t, api.updateRequest, "update request = %+v", api.updateRequest)
	require.EqualValues(t, 123, api.updateRequest.ID, "update request = %+v", api.updateRequest)
	require.NotNil(t, api.updateRequest.Description, "update request = %+v", api.updateRequest)
	require.Equal(t, "updated", *api.updateRequest.Description, "update request = %+v", api.updateRequest)
	require.Len(t, api.updateRequest.Records, 2, "update request = %+v", api.updateRequest)
	require.Equal(t, "192.0.2.2", api.updateRequest.Records[0].RData, "update request = %+v", api.updateRequest)
	require.False(t, api.updateRequest.MonitoringSuiteLog.Enabled, "update request = %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.Tags, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.IconID, "omitted update fields were populated: %+v", api.updateRequest)
	require.Zero(t, api.updateRequest.SettingsHash, "SettingsHash is set by the SDK service: %+v", api.updateRequest)

	run("dns-api", "delete", "--id", "123", "--fail-if-not-found")
	require.NotNil(t, api.deleteRequest, "delete request = %+v", api.deleteRequest)
	require.EqualValues(t, 123, api.deleteRequest.ID, "delete request = %+v", api.deleteRequest)
	require.True(t, api.deleteRequest.FailIfNotFound, "delete request = %+v", api.deleteRequest)

	// --request 経路でも同じ SDK リクエストになる
	run("dns-api", "find", "--request", `{"Names":["json-zone"],"Tags":["web"]}`)
	require.NotNil(t, api.findRequest, "find request = %+v", api.findRequest)
	require.Equal(t, []string{"json-zone"}, api.findRequest.Names, "find request = %+v", api.findRequest)
	require.True(t, reflectStrings(api.findRequest.Tags, []string{"web"}), "find request = %+v", api.findRequest)

	run("dns-api", "read", "--request", `{"ID":123}`)
	require.NotNil(t, api.readRequest, "read request = %+v", api.readRequest)
	require.EqualValues(t, 123, api.readRequest.ID, "read request = %+v", api.readRequest)
}

func TestDNSJSONInputAndMutualExclusion(t *testing.T) {
	api := &dnsAPITestDouble{}
	commandLine := newCLI()
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return api, nil })

	run := func(args ...string) (int, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		return code, stderr.String()
	}
	// カンマを含むレコード値は JSON 経路でのみ指定できる
	code, stderr := run("dns-api", "create", "--request",
		`{"Name":"json.example.com","Records":[{"Name":"example.com","Type":"TXT","RData":"a,b","TTL":3600}]}`)
	require.Equal(t, 0, code, "JSON create code=%d stderr=%q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.NotNil(t, api.createRequest, "JSON create request = %+v", api.createRequest)
	require.Equal(t, "json.example.com", api.createRequest.Name, "JSON create request = %+v", api.createRequest)
	require.Len(t, api.createRequest.Records, 1, "JSON create request = %+v", api.createRequest)
	require.Equal(t, "a,b", api.createRequest.Records[0].RData, "JSON create request = %+v", api.createRequest)

	for _, args := range [][]string{
		{"dns-api", "find", "--request", `{"Count":1}`, "--name", "x"},
		{"dns-api", "create", "--request", `{"Name":"x"}`, "--name", "x"},
		{"dns-api", "update", "--request", `{"ID":123}`, "--description", "x"},
		{"dns-api", "delete", "--request", `{"ID":123}`, "--fail-if-not-found=false"},
	} {
		code, stderr = run(args...)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stderr %q", args, code, stderr)
		require.Contains(t, stderr, "--request と個別フラグ", "missing expected text %q", "--request と個別フラグ")
	}

	for _, args := range [][]string{
		{"dns-api", "create"},
		{"dns-api", "read"},
		{"dns-api", "update", "--id", "123"},
		{"dns-api", "delete"},
	} {
		code, stderr = run(args...)
		require.EqualValues(t, 1, code, "run(%v) = code %d, stderr %q", args, code, stderr)
	}

	// --record の入力エラーは CLI で検出する
	for _, record := range []string{
		"name=www,type=A,rdata=192.0.2.1",                  // ttl 不足
		"name=www,type=DS,rdata=192.0.2.1,ttl=3600",        // 対応していないレコード種別
		"name=www,type=A,rdata=192.0.2.1,ttl=3600,extra=1", // 未対応のキー
		"name=www,type=A,rdata=192.0.2.1,ttl=1h",           // ttl は整数
	} {
		code, stderr = run("dns-api", "create", "--name", "example.com", "--record", record)
		require.EqualValues(t, 1, code, "run(--record %s) = code %d, stderr %q", record, code, stderr)
	}
}

func TestDNSCommandErrorsAreReturned(t *testing.T) {
	apiErr := errors.New("DNS API failure")
	api := &dnsAPITestDouble{err: apiErr}
	commandLine := newCLI()
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"dns-api", "find"}, &stdout, &stderr, commandLine)
	require.EqualValues(t, 1, code, "code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	require.Empty(t, stdout, "stdout should be empty")
	require.Equal(t, apiErr.Error()+"\n", stderr.String(), "stderr = %q", stderr.String())
}

type dnsAPICaller struct {
	method string
	uri    string
	body   map[string]any
}

func (caller *dnsAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
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
	return []byte(`{"CommonServiceItem":{"ID":321,"Name":"example.com","Provider":{"Class":"dns"},"Settings":{"DNS":{"ResourceRecordSets":[{"Name":"www","Type":"A","RData":"192.0.2.1","TTL":3600}]}},"Status":{"Zone":"is1a","NS":["ns1.dns.ne.jp","ns2.dns.ne.jp"]}}}`), nil
}

// TestDNSCLIUsesSDKRequestConversion は SDK のサービス層を経由して API に送られる
// wire body を検証します。ゾーン名、Provider.Class、レコードの変換を確認します。
func TestDNSCLIUsesSDKRequestConversion(t *testing.T) {
	caller := &dnsAPICaller{}
	commandLine := newCLI()
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return dns.New(caller), nil })

	var stdout, stderr bytes.Buffer
	args := []string{
		"dns-api", "create", "--name", "example.com",
		"--record", "name=www,type=A,rdata=192.0.2.1,ttl=3600",
	}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.Equal(t, "POST", caller.method, "SDK call = %s %s", caller.method, caller.uri)
	require.Contains(t, caller.uri, "/commonserviceitem", "missing expected text %q", "/commonserviceitem")
	item, ok := caller.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	provider, ok := item["Provider"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	require.Equal(t, "dns", provider["Class"], "SDK request body = %#v", caller.body)
	dnsSettings, ok := item["Settings"].(map[string]any)["DNS"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	records, ok := dnsSettings["ResourceRecordSets"].([]any)
	require.True(t, ok, "SDK request body = %#v", caller.body)
	require.Len(t, records, 1, "SDK request body = %#v", caller.body)
	record := records[0].(map[string]any)
	require.Equal(t, "example.com", item["Name"], "SDK request body = %#v", item)
	require.Equal(t, "www", record["Name"], "converted record = %#v", record)
	require.Equal(t, "A", record["Type"], "converted record = %#v", record)
	require.Equal(t, "192.0.2.1", record["RData"], "converted record = %#v", record)
	require.EqualValues(t, 3600, record["TTL"], "converted record = %#v", record)
	var result iaas.DNS
	err := json.Unmarshal(stdout.Bytes(), &result)
	require.NoError(t, err)
	require.EqualValues(t, 321, result.ID, "decoded SDK result = %+v", result)
	require.Equal(t, "example.com", result.Name, "unexpected value")
	require.Len(t, result.Records, 1, "decoded SDK result = %+v", result)
	require.Equal(t, []string{"ns1.dns.ne.jp", "ns2.dns.ne.jp"}, result.DNSNameServers, "decoded SDK result = %+v", result)

	// レコードなしの作成は空配列を送る
	emptyCaller := &dnsAPICaller{}
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return dns.New(emptyCaller), nil })
	stdout.Reset()
	stderr.Reset()
	args = []string{"dns-api", "create", "--name", "empty.example.com"}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	item, ok = emptyCaller.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", emptyCaller.body)
	dnsSettings, ok = item["Settings"].(map[string]any)["DNS"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", emptyCaller.body)
	records, ok = dnsSettings["ResourceRecordSets"].([]any)
	require.True(t, ok, "wire ResourceRecordSets should be an array: %#v", dnsSettings)
	require.Empty(t, records, "wire ResourceRecordSets = %#v", records)
}

// TestDNSJSONRequestRoundTrip は --request JSON から SDK が送信する wire body までの
// 変換 (round-trip) を検証します。update では SDK のサービス層が現在値とマージするため、
// --request に含まれない項目が現在値で維持されることを wire body で確認します。
func TestDNSJSONRequestRoundTrip(t *testing.T) {
	commandLine := newCLI()
	current := `{"CommonServiceItem":{"ID":123,"Name":"example.com","Description":"current-desc","Provider":{"Class":"dns"},"Settings":{"DNS":{"ResourceRecordSets":[{"Name":"www","Type":"A","RData":"192.0.2.1","TTL":3600}],"MonitoringSuiteLog":{"Enabled":true}}},"SettingsHash":"current-hash"},"Status":{"Zone":"is1a","NS":["ns1.dns.ne.jp","ns2.dns.ne.jp"]}}`
	caller := &dnsRoundTripCaller{readBody: []byte(current)}
	setDNSAPIFactory(commandLine, func() (dnsapi.API, error) { return dns.New(caller), nil })

	dir := t.TempDir()
	updatePath := filepath.Join(dir, "update.json")
	updateJSON := `{"ID":123,"Description":"json-updated","Records":[{"Name":"example.com","Type":"TXT","RData":"skr-e2e-test","TTL":600}]}`
	require.NoError(t, os.WriteFile(updatePath, []byte(updateJSON), 0o600))

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"dns-api", "update", "--request", "@" + updatePath}, &stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "update: stderr %q", stderr.String())
	require.Equal(t, "PUT", caller.updateMethod, "update URI = %s", caller.updateURI)

	item, ok := caller.updateBody["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.updateBody)
	dnsSettings, ok := item["Settings"].(map[string]any)["DNS"].(map[string]any)
	require.True(t, ok, "SDK request body = %#v", caller.updateBody)
	// --request で指定した項目が wire まで届く
	require.Equal(t, "json-updated", item["Description"], "wire item = %#v", item)
	records, ok := dnsSettings["ResourceRecordSets"].([]any)
	require.True(t, ok, "wire dnsSettings = %#v", dnsSettings)
	require.Len(t, records, 1, "wire dnsSettings = %#v", dnsSettings)
	record := records[0].(map[string]any)
	require.Equal(t, "example.com", record["Name"], "converted record = %#v", record)
	require.Equal(t, "TXT", record["Type"], "converted record = %#v", record)
	require.Equal(t, "skr-e2e-test", record["RData"], "converted record = %#v", record)
	require.EqualValues(t, 600, record["TTL"], "converted record = %#v", record)
	// --request に含まれない MonitoringSuiteLog は SDK が現在値を引き継ぐ
	require.Equal(t, "True", dnsSettings["MonitoringSuiteLog"].(map[string]any)["Enabled"], "wire dnsSettings = %#v", dnsSettings)
	// update 入力型に存在しない API フィールド (Status 系) は wire に現れない
	require.NotContains(t, item, "Status", "wire item = %#v", item)
}

// dnsRoundTripCaller は SDK が送信する wire body を記録し、Read 応答を固定値で返します。
type dnsRoundTripCaller struct {
	readBody     []byte
	updateMethod string
	updateURI    string
	updateBody   map[string]any
}

func (caller *dnsRoundTripCaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	if method == "GET" {
		return caller.readBody, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &caller.updateBody); err != nil {
		return nil, err
	}
	caller.updateMethod, caller.updateURI = method, uri
	return caller.readBody, nil
}
