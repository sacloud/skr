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
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/sacloud/skr/internal/apigen"
	diskapi "github.com/sacloud/skr/internal/iaas/diskapi"
	internetapi "github.com/sacloud/skr/internal/iaas/internetapi"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
	"github.com/stretchr/testify/require"
)

func TestIaaSSingleNameSearch(t *testing.T) {
	for _, resource := range []string{"internet", "server", "disk"} {
		for _, test := range []struct {
			name string
			args []string
			want []string
			fail bool
		}{
			{"omitted", []string{"--zone", "is1b"}, nil, false},
			{"single", []string{"--zone", "is1b", "--name", "router"}, []string{"router"}, false},
			{"comma", []string{"--zone", "is1b", "--name", "router, 日本語 name"}, []string{"router, 日本語 name"}, false},
			{"empty", []string{"--zone", "is1b", "--name", ""}, []string{""}, false},
			{"json", []string{"--request", `{"Zone":"is1b","Names":["router"]}`}, []string{"router"}, false},
			{"multiple json", []string{"--request", `{"Zone":"is1b","Names":["one","two"]}`}, []string{"one", "two"}, false},
			{"exclusive", []string{"--request", `{"Zone":"is1b"}`, "--name", ""}, nil, true},
			{"missing zone", []string{"--name", "router"}, nil, true},
		} {
			t.Run(resource+"/"+test.name, func(t *testing.T) {
				commandLine := newCLI()
				internetAPI := &internetAPITestDouble{}
				serverAPI := &serverAPITestDouble{}
				diskAPI := &diskAPITestDouble{}
				setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return internetAPI, nil })
				setServerAPIFactory(commandLine, func() (serverapi.API, error) { return serverAPI, nil })
				setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return diskAPI, nil })
				var stdout, stderr bytes.Buffer
				code := runCLI(append([]string{"iaas-api", resource, "find"}, test.args...), &stdout, &stderr, commandLine)
				if (code != 0) != test.fail {
					t.Fatalf("code = %d, stderr = %s", code, &stderr)
				}
				called := false
				var names []string
				switch resource {
				case "internet":
					if internetAPI.findRequest != nil {
						called, names = true, internetAPI.findRequest.Names
					}
				case "server":
					if len(serverAPI.findRequests) != 0 {
						called, names = true, serverAPI.findRequests[0].Names
					}
				case "disk":
					if len(diskAPI.findRequests) != 0 {
						called, names = true, diskAPI.findRequests[0].Names
					}
				}
				require.NotEqual(t, test.fail, called, "called = %v, Names = %#v, want %#v", called, names, test.want)
				require.Equal(t, test.want, names, "unexpected value")
			})
		}
	}
}

type nameSearchCaller struct {
	resource string
	payload  []byte
}

func (caller *nameSearchCaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	if method != "GET" || !strings.HasSuffix(parsed.Path, "/"+caller.resource) {
		return nil, fmt.Errorf("unexpected request: %s %s", method, uri)
	}
	caller.payload, err = json.Marshal(body)
	if err != nil {
		return nil, err
	}
	key := map[string]string{"internet": "Internet", "server": "Servers", "disk": "Disks"}[caller.resource]
	return json.Marshal(map[string]any{"Total": 0, "Count": 0, key: []any{}})
}

func TestSingleNameSearchUsesSDKFilter(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	for _, resource := range []string{"internet", "server", "disk"} {
		t.Run(resource, func(t *testing.T) {
			caller := &nameSearchCaller{resource: resource}
			commandLine := newCLI()
			setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return internet.New(caller), nil })
			setServerAPIFactory(commandLine, func() (serverapi.API, error) { return server.New(caller), nil })
			setDiskAPIFactory(commandLine, func() (diskapi.API, error) { return disk.New(caller), nil })
			name := "router, 日本語 name"
			var stdout, stderr bytes.Buffer
			code := runCLI([]string{"iaas-api", resource, "find", "--zone", "is1b", "--name", name}, &stdout, &stderr, commandLine)
			require.Equal(t, 0, code, "code = %d, stderr = %s", code, &stderr)
			var payload struct {
				Filter map[string]json.RawMessage
			}
			err := json.Unmarshal(caller.payload, &payload)
			require.NoError(t, err)
			var encodedName string
			err2 := json.Unmarshal(payload.Filter["Name"], &encodedName)
			require.NoError(t, err2)
			decodedName, err := url.PathUnescape(encodedName)
			require.NoError(t, err)
			require.Equal(t, name, decodedName, "Name filter = %q, want %q; payload = %s", decodedName, name, caller.payload)
		})
	}
}

func TestInternetDeleteFlags(t *testing.T) {
	for _, test := range []struct {
		args                        []string
		force, failIfNotFound, fail bool
	}{
		{args: []string{"--zone", "is1b", "--id", "123"}},
		{args: []string{"--zone", "is1b", "--id", "123", "--force", "--fail-if-not-found"}, force: true, failIfNotFound: true},
		{args: []string{"--zone", "is1b", "--id", "123", "--force=false", "--fail-if-not-found=false"}},
		{args: []string{"--request", `{"Zone":"is1b","ID":123,"Force":true,"FailIfNotFound":true}`}, force: true, failIfNotFound: true},
		{args: []string{"--request", `{"Zone":"is1b","ID":123}`, "--force=false"}, fail: true},
		{args: []string{"--request", `{"Zone":"is1b","ID":123}`, "--fail-if-not-found=false"}, fail: true},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			commandLine := newCLI()
			api := &internetAPITestDouble{}
			setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return api, nil })
			var stdout, stderr bytes.Buffer
			code := runCLI(append([]string{"iaas-api", "internet", "delete"}, test.args...), &stdout, &stderr, commandLine)
			if (code != 0) != test.fail {
				t.Fatalf("code = %d, stderr = %s", code, &stderr)
			}
			if test.fail {
				require.Nil(t, api.deleteRequest, "invalid request reached API")
			} else if request := api.deleteRequest; request == nil || request.Force != test.force || request.FailIfNotFound != test.failIfNotFound {
				t.Fatalf("unexpected request: %+v", request)
			}
		})
	}
}

type internetAPITestDouble struct {
	internetapi.API
	findRequest          *internet.FindRequest
	createRequest        *internet.CreateRequest
	updateRequest        *internet.UpdateRequest
	deleteRequest        *internet.DeleteRequest
	addSubnetRequest     *internet.AddSubnetRequest
	deleteSubnetRequest  *internet.DeleteSubnetRequest
	monitorRouterRequest *internet.MonitorRouterRequest
	createErr            error
}

func (api *internetAPITestDouble) FindWithContext(_ context.Context, request *internet.FindRequest) ([]*iaas.Internet, error) {
	api.findRequest = request
	return []*iaas.Internet{}, nil
}

func (api *internetAPITestDouble) CreateWithContext(_ context.Context, request *internet.CreateRequest) (*iaas.Internet, error) {
	api.createRequest = request
	if api.createErr != nil {
		return nil, api.createErr
	}
	return &iaas.Internet{
		ID:             123,
		Name:           request.Name,
		Description:    request.Description,
		Tags:           request.Tags,
		NetworkMaskLen: request.NetworkMaskLen,
		BandWidthMbps:  request.BandWidthMbps,
	}, nil
}

func (api *internetAPITestDouble) UpdateWithContext(_ context.Context, request *internet.UpdateRequest) (*iaas.Internet, error) {
	api.updateRequest = request
	return &iaas.Internet{ID: request.ID}, nil
}

func (api *internetAPITestDouble) DeleteWithContext(_ context.Context, request *internet.DeleteRequest) error {
	api.deleteRequest = request
	return nil
}

func (api *internetAPITestDouble) AddSubnetWithContext(_ context.Context, request *internet.AddSubnetRequest) (*iaas.Subnet, error) {
	api.addSubnetRequest = request
	return &iaas.Subnet{ID: 456}, nil
}

func (api *internetAPITestDouble) DeleteSubnetWithContext(_ context.Context, request *internet.DeleteSubnetRequest) error {
	api.deleteSubnetRequest = request
	return nil
}

func (api *internetAPITestDouble) MonitorRouterWithContext(_ context.Context, request *internet.MonitorRouterRequest) ([]*iaas.MonitorRouterValue, error) {
	api.monitorRouterRequest = request
	return []*iaas.MonitorRouterValue{}, nil
}

func TestInternetGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-internet.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/internetapi/internet_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Internet commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-internet.json API_OUTPUT=internal/iaas/internetapi/internet_api_generated.go")
}

func TestIaaSInternetHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"internet"}},
		{args: []string{"iaas-api", "internet", "--help"}, want: []string{"find", "read", "create", "update", "delete", "list-subnet", "add-subnet", "update-subnet", "delete-subnet", "read-ipv6", "enable-ipv6", "disable-ipv6", "monitor-router"}},
		{args: []string{"iaas-api", "internet", "create", "--help"}, want: []string{"--zone", "--name", "--tags", "--network-mask-len", "--band-width-mbps", "課金", "EnableIPv6", "併用不可"}},
		{args: []string{"iaas-api", "internet", "update", "--help"}, want: []string{"--zone", "--id", "--name", "--tags", "BandWidthMbps", "EnableIPv6", "省略した項目"}},
		{args: []string{"iaas-api", "internet", "find", "--help"}, want: []string{"--name", "部分一致", "Names", "併用不可"}},
		{args: []string{"iaas-api", "internet", "delete", "--help"}, want: []string{"--force", "--fail-if-not-found", "NextHop", "サブネット", "IPv6", "--request"}},
		{args: []string{"iaas-api", "internet", "add-subnet", "--help"}, want: []string{"--network-mask-len", "--next-hop", "ネクストホップ", "@path.json"}},
		{args: []string{"iaas-api", "internet", "monitor-router", "--help"}, want: []string{"--zone", "--id", "--start", "--end", "RFC3339", "1時間前", "併用不可", "--request"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				require.Contains(t, help, expected, "help does not contain %q: %s", expected, help)
			}
		})
	}
}

func TestIaaSInternetCommandsBuildSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &internetAPITestDouble{}
	commandLine := newCLI()
	setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return api, nil })

	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "internet"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	_, stderr, code := run("create", "--zone", "test-zone", "--name", "router",
		"--description", "tutorial", "--tags", "edge,managed", "--network-mask-len", "28", "--band-width-mbps", "100")
	require.Equal(t, 0, code, "create flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	flagRequest := *api.createRequest
	if flagRequest.Zone != "test-zone" || flagRequest.Name != "router" ||
		flagRequest.NetworkMaskLen != 28 || flagRequest.BandWidthMbps != 100 ||
		!reflect.DeepEqual([]string(flagRequest.Tags), []string{"edge", "managed"}) {
		t.Fatalf("Create flag request = %+v", flagRequest)
	}

	_, stderr, code = run("create", "--request", `{"Zone":"test-zone","Name":"router","Description":"tutorial","Tags":["edge","managed"],"NetworkMaskLen":28,"BandWidthMbps":100}`)
	require.Equal(t, 0, code, "create JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, *api.createRequest, flagRequest, "Create JSON request = %+v, want same as flag request %+v", api.createRequest, flagRequest)

	_, stderr, code = run("find", "--zone", "test-zone", "--tags", "edge,managed", "--count", "10", "--from", "2")
	require.Equal(t, 0, code, "find flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if request := api.findRequest; request.Zone != "test-zone" || request.Count != 10 || request.From != 2 ||
		!reflect.DeepEqual(request.Tags, []string{"edge", "managed"}) {
		t.Fatalf("Find request = %+v", request)
	}

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Names":["router"],"SwitchIDs":["789"]}`)
	require.Equal(t, 0, code, "find JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if request := api.findRequest; !reflect.DeepEqual(request.Names, []string{"router"}) ||
		!reflect.DeepEqual(request.SwitchIDs, []string{"789"}) {
		t.Fatalf("Find JSON request = %+v", request)
	}

	_, stderr, code = run("update", "--zone", "test-zone", "--id", "123", "--name", "", "--tags", "updated")
	require.Equal(t, 0, code, "update flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if request := api.updateRequest; request.ID != 123 || request.Name == nil || *request.Name != "" ||
		request.Tags == nil || !reflect.DeepEqual([]string(*request.Tags), []string{"updated"}) ||
		request.Description != nil {
		t.Fatalf("Update request = %+v, want explicit empty Name, Tags, and omitted Description", request)
	}

	_, stderr, code = run("update", "--request", `{"Zone":"test-zone","ID":123,"EnableIPv6":true,"BandWidthMbps":200}`)
	require.Equal(t, 0, code, "update JSON helpers: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if request := api.updateRequest; request.EnableIPv6 == nil || !*request.EnableIPv6 ||
		request.BandWidthMbps == nil || *request.BandWidthMbps != 200 {
		t.Fatalf("Update JSON request = %+v, want bandwidth and IPv6 values", request)
	}

	_, stderr, code = run("add-subnet", "--zone", "test-zone", "--id", "123", "--network-mask-len", "28", "--next-hop", "192.0.2.1")
	require.Equal(t, 0, code, "add-subnet: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	if request := api.addSubnetRequest; request.Zone != "test-zone" || request.ID != 123 ||
		request.NetworkMaskLen != 28 || request.NextHop != "192.0.2.1" {
		t.Fatalf("AddSubnet request = %+v", request)
	}

	_, stderr, code = run("delete", "--request", `{"Zone":"test-zone","ID":123,"Force":true,"FailIfNotFound":true}`)
	require.Equal(t, 0, code, "delete JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request := api.deleteRequest
	require.True(t, request.Force, "Delete request = %+v, want explicit Force and FailIfNotFound", request)
	require.True(t, request.FailIfNotFound, "unexpected value")

	_, stderr, code = run("monitor-router", "--zone", "test-zone", "--id", "123",
		"--start", "2026-10-14T00:00:00Z", "--end", "2026-10-15T00:00:00Z")
	require.Equal(t, 0, code, "monitor-router flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	flagMonitorRequest := *api.monitorRouterRequest
	require.Equal(t, "test-zone", flagMonitorRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, flagMonitorRequest.ID, "unexpected value")
	require.Equal(t, time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), flagMonitorRequest.Start, "unexpected value")
	require.Equal(t, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), flagMonitorRequest.End, "unexpected value")

	_, stderr, code = run("monitor-router", "--request", `{"Zone":"test-zone","ID":123}`)
	require.Equal(t, 0, code, "monitor-router JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.monitorRouterRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.monitorRouterRequest.ID, "unexpected value")
	require.True(t, api.monitorRouterRequest.Start.IsZero(), "omitted Start should stay zero for the SDK default")
	require.True(t, api.monitorRouterRequest.End.IsZero(), "omitted End should stay zero for the SDK default")

	_, stderr, code = run("delete-subnet", "--request", `{"Zone":"test-zone","ID":123,"SubnetID":456}`)
	require.Equal(t, 0, code, "delete-subnet JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	request2 := api.deleteSubnetRequest
	require.EqualValues(t, 123, request2.ID, "DeleteSubnet request = %+v", request2)
	require.EqualValues(t, 456, request2.SubnetID, "unexpected value")
}

func TestIaaSInternetRejectsInvalidRequestsBeforeCallingAPI(t *testing.T) {
	api := &internetAPITestDouble{}
	for _, args := range [][]string{
		{"find", "--zone", "all"},
		{"find", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
		{"read", "--zone", "test-zone"},
		{"create", "--zone", "test-zone"},
		{"create", "--zone", "test-zone", "--name", "router"},
		{"create", "--zone", "test-zone", "--name", "router", "--network-mask-len", "28"},
		{"create", "--zone", "test-zone", "--name", "router", "--band-width-mbps", "100"},
		{"monitor-router"},
		{"monitor-router", "--zone", "test-zone"},
		{"monitor-router", "--zone", "test-zone", "--id", "123", "--start", "invalid"},
		{"monitor-router", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
		{"add-subnet", "--zone", "test-zone", "--id", "123", "--network-mask-len", "28"},
	} {
		commandLine := newCLI()
		setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return api, nil })
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "internet"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want an input error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	if api.findRequest != nil || api.createRequest != nil || api.updateRequest != nil ||
		api.deleteRequest != nil || api.addSubnetRequest != nil || api.deleteSubnetRequest != nil ||
		api.monitorRouterRequest != nil {
		t.Fatalf("invalid request reached API: %+v", api)
	}
}

func TestIaaSInternetPropagatesAPIErrors(t *testing.T) {
	apiFailure := errors.New("Internet API failure")
	api := &internetAPITestDouble{createErr: apiFailure}
	commandLine := newCLI()
	setInternetAPIFactory(commandLine, func() (internetapi.API, error) { return api, nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "internet", "create", "--zone", "test-zone", "--name", "router",
		"--network-mask-len", "28", "--band-width-mbps", "100"},
		&stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "create error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), apiFailure.Error(), "missing expected text %q", apiFailure.Error())
}
