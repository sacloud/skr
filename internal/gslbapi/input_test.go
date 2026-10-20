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

package gslbapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

func TestParseDestinationServer(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *iaas.GSLBServer
		wantErr string
	}{
		{
			name:  "defaults",
			input: "ip-address=192.0.2.10",
			want: &iaas.GSLBServer{
				IPAddress: "192.0.2.10",
				Enabled:   types.StringTrue,
				Weight:    types.StringNumber(1),
			},
		},
		{
			name:  "explicit values",
			input: "ip-address=192.0.2.11,enabled=false,weight=100",
			want: &iaas.GSLBServer{
				IPAddress: "192.0.2.11",
				Enabled:   types.StringFalse,
				Weight:    types.StringNumber(100),
			},
		},
		{name: "missing ip", input: "enabled=true", wantErr: "ip-address is required"},
		{name: "empty value", input: "ip-address=", wantErr: "non-empty"},
		{name: "unknown key", input: "ip-address=192.0.2.10,host=example.test", wantErr: `unknown key "host"`},
		{name: "duplicate key", input: "ip-address=192.0.2.10,weight=2,weight=3", wantErr: `duplicate key "weight"`},
		{name: "invalid enabled", input: "ip-address=192.0.2.10,enabled=yes", wantErr: "enabled must be true or false"},
		{name: "invalid weight", input: "ip-address=192.0.2.10,weight=0", wantErr: "from 1 through 10000"},
		{name: "invalid syntax", input: "192.0.2.10", wantErr: "key=value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseDestinationServer(test.input)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("server = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCreateRequestFromFlags(t *testing.T) {
	command := &CreateCommand{
		Name:                    stringPointer("test-gslb"),
		Description:             stringPointer("description"),
		Tags:                    &[]string{"e2e", "managed"},
		IconID:                  int64Pointer(123),
		DelayLoop:               intPointer(20),
		Weighted:                boolPointer(false),
		SorryServer:             stringPointer("192.0.2.20"),
		HealthCheckProtocol:     stringPointer("http"),
		HealthCheckHostHeader:   stringPointer("example.test"),
		HealthCheckPath:         stringPointer("/health"),
		HealthCheckResponseCode: intPointer(200),
		HealthCheckPort:         intPointer(8080),
		DestinationServers:      []string{"ip-address=192.0.2.10,enabled=true,weight=5"},
	}
	got, err := createRequestFromFlags(command)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test-gslb" || got.Description != "description" ||
		!reflect.DeepEqual(got.Tags, types.Tags{"e2e", "managed"}) || got.IconID != 123 ||
		got.DelayLoop != 20 || got.Weighted != types.StringFalse || got.SorryServer != "192.0.2.20" {
		t.Fatalf("scalar request fields = %+v", got)
	}
	if got.HealthCheck == nil || got.HealthCheck.Protocol != types.GSLBHealthCheckProtocols.HTTP ||
		got.HealthCheck.HostHeader != "example.test" || got.HealthCheck.Path != "/health" ||
		got.HealthCheck.ResponseCode != types.StringNumber(200) || got.HealthCheck.Port != types.StringNumber(8080) {
		t.Fatalf("health check = %+v", got.HealthCheck)
	}
	if len(got.DestinationServers) != 1 || got.DestinationServers[0].IPAddress != "192.0.2.10" ||
		got.DestinationServers[0].Weight != types.StringNumber(5) {
		t.Fatalf("destination servers = %+v", got.DestinationServers)
	}
}

func TestBuildHealthCheckRequiresProtocolAndKnownValue(t *testing.T) {
	for _, test := range []struct {
		name    string
		proto   *string
		path    *string
		wantErr string
	}{
		{name: "protocol omitted", path: stringPointer("/health"), wantErr: "--health-check-protocol"},
		{name: "unsupported protocol", proto: stringPointer("icmp"), wantErr: "unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := buildHealthCheck(test.proto, nil, test.path, nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestUpdateRequestFromFlagsPreservesUnspecifiedFields(t *testing.T) {
	command := &UpdateCommand{
		ID:       int64Pointer(42),
		Weighted: boolPointer(false),
	}
	got, err := updateRequestFromFlags(command)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 42 || got.Weighted == nil || *got.Weighted != types.StringFalse {
		t.Fatalf("request = %+v", got)
	}
	if got.Name != nil || got.Description != nil || got.Tags != nil || got.DelayLoop != nil ||
		got.HealthCheck != nil || got.DestinationServers != nil || got.MonitoringSuiteLog != nil {
		t.Fatalf("unspecified fields were populated: %+v", got)
	}
}

func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }
func int64Pointer(value int64) *int64    { return &value }
func boolPointer(value bool) *bool       { return &value }
