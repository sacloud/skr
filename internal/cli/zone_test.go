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

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	"github.com/sacloud/skr/internal/apigen"
	zoneapi "github.com/sacloud/skr/internal/iaas/zoneapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

func TestZoneGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-zone.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := apigen.DecodeConfig(configData)
	if err != nil {
		t.Fatal(err)
	}
	want, err := apigen.Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(repositoryPath("internal/iaas/zoneapi/zone_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Zone commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-zone.json API_OUTPUT=internal/iaas/zoneapi/zone_api_generated.go")
	}
}

func TestIaaSZoneHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "zone", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "zone", "find", "--help"}, want: []string{"--query", "map(.Name)", "--request", "FindRequest"}},
		{args: []string{"iaas-api", "zone", "read", "--help"}, want: []string{"--id", "--request", "@path.json", "ZONE-ID", "併用できません"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(test.args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", test.args, code, stderr.String())
		}
		help := strings.Join(strings.Fields(stdout.String()), " ")
		for _, text := range test.want {
			if !strings.Contains(help, text) {
				t.Errorf("help for %v does not contain %q: %s", test.args, text, help)
			}
		}
	}
}

func TestIaaSZoneFindWithLocalMockAndQuery(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.String()
	}

	var zones []*iaas.Zone
	if err := json.Unmarshal([]byte(runCommand("iaas-api", "zone", "find")), &zones); err != nil {
		t.Fatal(err)
	}
	if len(zones) != 2 || zones[0].Name != "test-zone-a" || zones[1].Name != "test-zone-b" {
		t.Fatalf("zone find returned %#v, want configured mock zones", zones)
	}

	queryOutput := runCommand("iaas-api", "zone", "find", "--query", "map(.Name)")
	var names []string
	if err := json.Unmarshal([]byte(queryOutput), &names); err != nil {
		t.Fatalf("decode query output %q: %v", queryOutput, err)
	}
	if len(names) != 2 || names[0] != "test-zone-a" || names[1] != "test-zone-b" {
		t.Errorf("query output = %#v, want configured zone names", names)
	}
}

func TestIaaSZoneReadWithLocalMock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.String()
	}

	// --id フラグ経路でゾーンを参照する
	output := runCommand("iaas-api", "zone", "read", "--id", "2")
	if !strings.Contains(output, `"Name": "test-zone-b"`) {
		t.Fatalf("read --id output = %q, want test-zone-b", output)
	}

	// --query で必要な項目へ絞り込む。応答は単一オブジェクトのためオブジェクト構築式を使う
	projected := runCommand("iaas-api", "zone", "read", "--id", "1", "--query", "{ID,Name}")
	var item struct {
		ID   int64
		Name string
	}
	if err := json.Unmarshal([]byte(projected), &item); err != nil {
		t.Fatalf("decode projected output %q: %v", projected, err)
	}
	if item.ID != 1 || item.Name != "test-zone-a" {
		t.Fatalf("projected read = %#v, want ID 1 and test-zone-a", item)
	}

	// --request 経路でも同じ SDK リクエストになる
	jsonOutput := runCommand("iaas-api", "zone", "read", "--request", `{"ID":2}`)
	if !strings.Contains(jsonOutput, `"Name": "test-zone-b"`) {
		t.Fatalf("read --request output = %q, want test-zone-b", jsonOutput)
	}
}

func TestIaaSZoneRejectsInvalidReadRequestsBeforeCallingAPI(t *testing.T) {
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a"}})
	t.Cleanup(server.Close)
	for _, args := range [][]string{
		{"read"},
		{"read", "--id", "invalid"},
		{"read", "--id", "1", "--request", `{"ID":1}`},
		{"read", "--request", `{"ID":"invalid"}`},
		{"read", "--request", `{"ID":1} {}`},
		{"read", "--request", `{"Unknown":1}`},
		{"read", "--request", `not-json`},
	} {
		commandLine := newCLI()
		setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
			return zone.New(server), nil
		})
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "zone"}, args...), &stdout, &stderr, commandLine)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Errorf("%v: code %d stdout %q stderr %q; want validation error", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestIaaSZoneReadPropagatesAPIErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	setZoneAPIFactory(commandLine, func() (zoneapi.API, error) {
		return zone.New(server), nil
	})
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "zone", "read", "--id", "99"}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "not_found") {
		t.Fatalf("read error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
