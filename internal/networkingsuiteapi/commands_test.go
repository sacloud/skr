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

package networkingsuiteapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/networking-suite/apis/v1"
)

const (
	testGroupSRN  = "srnv1:sakura-is1:sakura.networking-suite.subnet-group:1234567890"
	testSubnetSRN = "srnv1:sakura-is1b:sakura.networking-suite.subnet:2345678901"
)

func TestInterfaceConnectionMissingFlagsDoesNotSuggestUnsupportedRequest(t *testing.T) {
	command := InterfaceConnectionCreateCommand{}
	err := command.Run(&kong.Context{})
	if err == nil || !strings.Contains(err.Error(), "--interface-srn") || !strings.Contains(err.Error(), "--subnet-srn") {
		t.Fatalf("missing required flags were not reported: %v", err)
	}
	if strings.Contains(err.Error(), "--request") {
		t.Fatalf("required flag error suggests unsupported JSON input: %v", err)
	}
}

func TestNetworkingSuiteCommandsUseSDKRequests(t *testing.T) {
	const (
		connectionSRN = "srnv1:sakura-is1b:sakura.networking-suite.interface-connection:3456789012"
		addressSRN    = "srnv1:sakura-is1b:sakura.networking-suite.address:4567890123"
		interfaceSRN  = "srnv1:sakura-is1b:sakura.iaas.interface:5678901234"
	)
	subnet := map[string]any{
		"SRN": testSubnetSRN, "Name": "subnet", "Description": "subnet description",
		"IPv4AddressRangeCIDR": "10.0.0.0/24", "Zone": map[string]string{"Code": "is1b"},
		"SubnetGroup": map[string]string{"SRN": testGroupSRN},
	}
	connection := map[string]any{
		"SRN": connectionSRN, "EphemeralIPv4Address": "10.0.0.10",
	}
	address := map[string]any{
		"SRN": addressSRN, "AddressType": "EPHEMERAL_ADDRESS", "IPVersion": "IPv4",
		"IPAddress": "10.0.0.10", "Subnet": map[string]string{"SRN": testSubnetSRN},
	}
	var got []string
	var gotCreateSubnet, gotCreateConnection map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		got = append(got, request.Method+" "+request.URL.Path)
		switch request.Method + " " + request.URL.Path {
		case "GET /api/cloud/1.1/networking-suite/subnets":
			if request.URL.Query().Get("subnetGroupSRN") != testGroupSRN {
				t.Errorf("subnetGroupSRN = %q", request.URL.Query().Get("subnetGroupSRN"))
			}
			_, _ = fmt.Fprintf(writer, `{"Total":1,"From":0,"Count":1,"Subnets":[%s]}`, mustJSON(t, subnet))
		case "POST /api/cloud/1.1/networking-suite/subnets":
			if err := json.NewDecoder(request.Body).Decode(&gotCreateSubnet); err != nil {
				t.Errorf("decode subnet create body: %v", err)
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{"Subnet": subnet})
		case "GET /api/cloud/1.1/networking-suite/subnets/2345678901":
			_ = json.NewEncoder(writer).Encode(map[string]any{"Subnet": subnet})
		case "PUT /api/cloud/1.1/networking-suite/subnets/2345678901":
			_ = json.NewEncoder(writer).Encode(map[string]any{"Subnet": subnet})
		case "DELETE /api/cloud/1.1/networking-suite/subnets/2345678901",
			"DELETE /api/cloud/1.1/networking-suite/interface-connections/3456789012":
			writer.WriteHeader(http.StatusNoContent)
		case "POST /api/cloud/1.1/networking-suite/interface-connections":
			if err := json.NewDecoder(request.Body).Decode(&gotCreateConnection); err != nil {
				t.Errorf("decode interface connection create body: %v", err)
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{"InterfaceConnection": connection})
		case "GET /api/cloud/1.1/networking-suite/addresses":
			if request.URL.Query().Get("subnetSRN") != testSubnetSRN ||
				request.URL.Query().Get("interfaceConnectionSRN") != connectionSRN {
				t.Errorf("address query = %v", request.URL.Query())
			}
			_, _ = fmt.Fprintf(writer, `{"Total":1,"From":0,"Count":1,"Addresses":[%s]}`, mustJSON(t, address))
		case "GET /api/cloud/1.1/networking-suite/addresses/4567890123":
			_ = json.NewEncoder(writer).Encode(map[string]any{"Address": address})
		default:
			http.Error(writer, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	setTestClient(t, server)

	var outputs []any
	runtime := Runtime{
		NewClient:      func() (*v1.Client, error) { return NewClient(false) },
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput: func(_ *kong.Context, value any) error {
			outputs = append(outputs, value)
			return nil
		},
	}
	ctx := &kong.Context{}
	groupSRN := testGroupSRN
	if err := (&SubnetListCommand{SubnetGroupSRN: groupSRN, runtime: runtime}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	createSubnet := SubnetCreateCommand{
		Name: new("subnet"), Description: new("subnet description"),
		IPv4AddressRange: new("10.0.0.0/24"), Zone: new("is1b"), SubnetGroupSRN: &groupSRN, runtime: runtime,
	}
	if err := createSubnet.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&SubnetReadCommand{SRN: testSubnetSRN, runtime: runtime}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&SubnetUpdateCommand{
		SRN: testSubnetSRN, Name: new("subnet"), Description: new("updated"), runtime: runtime,
	}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&SubnetDeleteCommand{SRN: testSubnetSRN, runtime: runtime}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	ipAddress := "10.0.0.10"
	if err := (&InterfaceConnectionCreateCommand{
		InterfaceSRN: new(interfaceSRN), SubnetSRN: new(testSubnetSRN),
		EphemeralIPv4Address: &ipAddress, runtime: runtime,
	}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&InterfaceConnectionDeleteCommand{SRN: connectionSRN, runtime: runtime}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&AddressListCommand{
		SubnetSRN: testSubnetSRN, InterfaceConnectionSRN: new(connectionSRN), runtime: runtime,
	}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&AddressReadCommand{SRN: addressSRN, runtime: runtime}).Run(ctx); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(gotCreateSubnet, map[string]any{
		"Name": "subnet", "Description": "subnet description", "IPv4AddressRangeCIDR": "10.0.0.0/24",
		"Zone": map[string]any{"Code": "is1b"}, "SubnetGroup": map[string]any{"SRN": testGroupSRN},
	}) {
		t.Errorf("subnet create request = %#v", gotCreateSubnet)
	}
	if !reflect.DeepEqual(gotCreateConnection, map[string]any{
		"Interface": map[string]any{"SRN": interfaceSRN}, "Subnet": map[string]any{"SRN": testSubnetSRN},
		"EphemeralIPv4Address": ipAddress,
	}) {
		t.Errorf("interface connection create request = %#v", gotCreateConnection)
	}
	want := []string{
		"GET /api/cloud/1.1/networking-suite/subnets",
		"POST /api/cloud/1.1/networking-suite/subnets",
		"GET /api/cloud/1.1/networking-suite/subnets/2345678901",
		"PUT /api/cloud/1.1/networking-suite/subnets/2345678901",
		"DELETE /api/cloud/1.1/networking-suite/subnets/2345678901",
		"POST /api/cloud/1.1/networking-suite/interface-connections",
		"DELETE /api/cloud/1.1/networking-suite/interface-connections/3456789012",
		"GET /api/cloud/1.1/networking-suite/addresses",
		"GET /api/cloud/1.1/networking-suite/addresses/4567890123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %#v, want %#v", got, want)
	}
	if len(outputs) != 7 {
		t.Errorf("output count = %d, want 7", len(outputs))
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestSubnetGroupCommandsUseSDKRequests(t *testing.T) {
	var gotMethods []string
	var gotCreate, gotUpdate map[string]any
	item := map[string]any{
		"SRN": testGroupSRN, "Name": "test-group", "Description": "test description",
		"IPv4AddressRangeCIDR": "10.0.0.0/20", "Region": map[string]string{"Code": "is1"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		gotMethods = append(gotMethods, request.Method+" "+request.URL.Path)
		switch request.Method + " " + request.URL.Path {
		case "GET /api/cloud/1.1/networking-suite/subnet-groups":
			_, _ = writer.Write([]byte(`{"Total":1,"From":0,"Count":1,"SubnetGroups":[{"SRN":"srnv1:sakura-is1:sakura.networking-suite.subnet-group:1234567890","Name":"test-group","Description":"test description","IPv4AddressRangeCIDR":"10.0.0.0/20","Region":{"Code":"is1"}}]}`))
		case "POST /api/cloud/1.1/networking-suite/subnet-groups":
			if err := json.NewDecoder(request.Body).Decode(&gotCreate); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{"SubnetGroup": item})
		case "GET /api/cloud/1.1/networking-suite/subnet-groups/1234567890":
			_ = json.NewEncoder(writer).Encode(map[string]any{"SubnetGroup": item})
		case "PUT /api/cloud/1.1/networking-suite/subnet-groups/1234567890":
			if err := json.NewDecoder(request.Body).Decode(&gotUpdate); err != nil {
				t.Errorf("decode update body: %v", err)
			}
			updated := map[string]any{
				"SRN": testGroupSRN, "Name": "changed-group", "Description": "changed description",
				"IPv4AddressRangeCIDR": "10.0.0.0/20", "Region": map[string]string{"Code": "is1"},
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"SubnetGroup": updated})
		case "DELETE /api/cloud/1.1/networking-suite/subnet-groups/1234567890":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	setTestClient(t, server)

	var outputs []any
	runtime := Runtime{
		NewClient:      func() (*v1.Client, error) { return NewClient(false) },
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput: func(_ *kong.Context, value any) error {
			outputs = append(outputs, value)
			return nil
		},
	}
	commands := NewCommands(runtime)
	ctx := &kong.Context{}
	if err := commands.SubnetGroup.List.Run(ctx); err != nil {
		t.Fatal(err)
	}
	commands.SubnetGroup.Create = SubnetGroupCreateCommand{
		Name: new("test-group"), Description: new("test description"),
		IPv4AddressRange: new("10.0.0.0/20"), Region: new("is1"), runtime: runtime,
	}
	if err := commands.SubnetGroup.Create.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := commands.SubnetGroup.Read.Run(ctx); err == nil {
		t.Fatal("read accepted a missing SRN")
	}
	read := SubnetGroupReadCommand{SRN: testGroupSRN, runtime: runtime}
	if err := read.Run(ctx); err != nil {
		t.Fatal(err)
	}
	update := SubnetGroupUpdateCommand{
		SRN: testGroupSRN, Name: new("changed-group"), Description: new("changed description"), runtime: runtime,
	}
	if err := update.Run(ctx); err != nil {
		t.Fatal(err)
	}
	deleteCommand := SubnetGroupDeleteCommand{SRN: testGroupSRN, runtime: runtime}
	if err := deleteCommand.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(gotCreate, map[string]any{
		"Name": "test-group", "Description": "test description",
		"IPv4AddressRangeCIDR": "10.0.0.0/20", "Region": map[string]any{"Code": "is1"},
	}) {
		t.Errorf("create request = %#v", gotCreate)
	}
	if !reflect.DeepEqual(gotUpdate, map[string]any{"Name": "changed-group", "Description": "changed description"}) {
		t.Errorf("update request = %#v", gotUpdate)
	}
	wantMethods := []string{
		"GET /api/cloud/1.1/networking-suite/subnet-groups",
		"POST /api/cloud/1.1/networking-suite/subnet-groups",
		"GET /api/cloud/1.1/networking-suite/subnet-groups/1234567890",
		"PUT /api/cloud/1.1/networking-suite/subnet-groups/1234567890",
		"DELETE /api/cloud/1.1/networking-suite/subnet-groups/1234567890",
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Errorf("requests = %v, want %v", gotMethods, wantMethods)
	}
	if len(outputs) != 4 {
		t.Fatalf("output count = %d, want 4", len(outputs))
	}
}

func TestCreateRequestInputExclusiveAndValidated(t *testing.T) {
	for _, test := range []struct {
		name    string
		command SubnetGroupCreateCommand
		wantErr string
	}{
		{
			name:    "missing flags",
			command: SubnetGroupCreateCommand{},
			wantErr: "--name",
		},
		{
			name:    "request and flag",
			command: SubnetGroupCreateCommand{Request: new(`{}`), Name: new("name")},
			wantErr: "--request と個別フラグ",
		},
		{
			name:    "invalid json",
			command: SubnetGroupCreateCommand{Request: new(`{"Name":`)},
			wantErr: "リクエスト JSON",
		},
		{
			name:    "wrong field type",
			command: SubnetGroupCreateCommand{Request: new(`{"Name":12,"Description":"d","IPv4AddressRangeCIDR":"10.0.0.0/20","Region":{"Code":"is1"}}`)},
			wantErr: "リクエスト JSON",
		},
		{
			name:    "trailing json",
			command: SubnetGroupCreateCommand{Request: new(`{"Name":"n","Description":"d","IPv4AddressRangeCIDR":"10.0.0.0/20","Region":{"Code":"is1"}} {}`)},
			wantErr: "リクエスト JSON",
		},
		{
			name:    "missing request file",
			command: SubnetGroupCreateCommand{Request: new("@" + t.TempDir() + "/missing.json")},
			wantErr: "リクエストファイルを読み込めません",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.command.Run(&kong.Context{})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Run() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeRequestSourcesAndUnknownFields(t *testing.T) {
	var request v1.CreateSubnetGroup
	if err := decodeRequest(`{"Name":"inline","Description":"d","IPv4AddressRangeCIDR":"10.0.0.0/20","Region":{"Code":"is1"},"Unknown":"ignored"}`, &request); err != nil {
		t.Fatal(err)
	}

	if request.Name != "inline" {
		t.Fatalf("Name = %q", request.Name)
	}

	path := t.TempDir() + "/request.json"
	if err := os.WriteFile(path, []byte(`{"Name":"file","Description":"d","IPv4AddressRangeCIDR":"10.0.0.0/20","Region":{"Code":"is1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := decodeRequest("@"+path, &request); err != nil {
		t.Fatal(err)
	}
	if request.Name != "file" {
		t.Fatalf("file Name = %q", request.Name)
	}

	previousStdin := os.Stdin
	stdin, err := os.Open(path) // #nosec G304 -- パスは上記で t.TempDir 内に作成された。
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = stdin
	t.Cleanup(func() {
		os.Stdin = previousStdin
		_ = stdin.Close()
	})
	if err := decodeRequest("-", &request); err != nil {
		t.Fatal(err)
	}
	if request.Name != "file" {
		t.Fatalf("stdin Name = %q", request.Name)
	}
}

func TestDecodeRequestReportsStdinReadError(t *testing.T) {
	previousStdin := os.Stdin
	stdin := mustOpen(t, t.TempDir())
	os.Stdin = stdin
	t.Cleanup(func() {
		os.Stdin = previousStdin
		_ = stdin.Close()
	})
	var request v1.CreateSubnetGroup
	if err := decodeRequest("-", &request); err == nil || !strings.Contains(err.Error(), "標準入力からリクエストを読み込めません") {
		t.Fatalf("decodeRequest() error = %v, want stdin read error", err)
	}
}

func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- 呼び出し元は t.TempDir からのパスを渡す。
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestParseSRNRequiresExpectedResource(t *testing.T) {
	if _, err := parseSRN(testSubnetSRN, subnetGroupResource); err == nil || !strings.Contains(err.Error(), "リソース種別") {
		t.Fatalf("parseSRN() error = %v, want resource mismatch", err)
	}
	if _, err := parseSRN("bad-srn", subnetGroupResource); err == nil {
		t.Fatal("parseSRN accepted malformed SRN")
	}
}

func TestCommandErrorsAreReturned(t *testing.T) {
	sentinel := errors.New("client failure")
	command := SubnetGroupListCommand{runtime: Runtime{
		NewClient:      func() (*v1.Client, error) { return nil, sentinel },
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}}
	if err := command.Run(&kong.Context{}); !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want client error", err)
	}
}

func setTestClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	t.Setenv("SAKURA_ENDPOINTS_NETWORKING_SUITE", server.URL+"/api/cloud/1.1/")
}

func TestSDKClientBuildsWithEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	setTestClient(t, server)
	client, err := NewClient(false)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestRuntimeReturnsOutputError(t *testing.T) {
	sentinel := errors.New("write failed")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/cloud/1.1/networking-suite/subnet-groups" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"Total":0,"From":0,"Count":0,"SubnetGroups":[]}`)
	}))
	t.Cleanup(server.Close)
	setTestClient(t, server)
	command := SubnetGroupListCommand{runtime: Runtime{
		NewClient:      func() (*v1.Client, error) { return NewClient(false) },
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return sentinel },
	}}
	if err := command.Run(&kong.Context{}); !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want output error", err)
	}
}
