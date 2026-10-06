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

func TestIaaSZoneFindHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "zone", "--help"}, want: []string{"find", "--query", "map(.Name)"}},
		{args: []string{"iaas-api", "zone", "find", "--help"}, want: []string{"--query", "map(.Name)", "--request", "FindRequest"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(test.args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d; stderr: %s", test.args, code, stderr.String())
		}
		for _, text := range test.want {
			if !strings.Contains(stdout.String(), text) {
				t.Errorf("run(%v) help does not contain %q: %s", test.args, text, stdout.String())
			}
		}
	}
}

func TestIaaSZoneFindWithLocalMockAndQuery(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{Zones: []string{"test-zone-a", "test-zone-b"}})
	t.Cleanup(server.Close)

	commandLine := newCLI()
	commandLine.IaaSAPI.Zone.SetFactory(func() (zoneapi.API, error) {
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

	queryOutput := runCommand("iaas-api", "zone", "find", "--query", "map(.Name)", "--output", "table")
	var names []string
	if err := json.Unmarshal([]byte(queryOutput), &names); err != nil {
		t.Fatalf("decode query output %q: %v", queryOutput, err)
	}
	if len(names) != 2 || names[0] != "test-zone-a" || names[1] != "test-zone-b" {
		t.Errorf("query output = %#v, want configured zone names", names)
	}
}
