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
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	iconSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
	"github.com/sacloud/skr/internal/apigen"
	"github.com/sacloud/skr/internal/iaas/iconapi"
	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

func TestIconGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-icon.json"))
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
	got, err := os.ReadFile(repositoryPath("internal/iaas/iconapi/icon_api_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated Icon commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-icon.json API_OUTPUT=internal/iaas/iconapi/icon_api_generated.go")
	}
}

func TestRunIaaSIconHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"icon"}},
		{args: []string{"iaas-api", "icon", "--help"}, want: []string{"find", "read", "create", "update", "delete"}},
		{args: []string{"iaas-api", "icon", "find", "--help"}, want: []string{"Names", "Tags", "Scope", "Sort", "--count", "--from", "省略"}},
		{args: []string{"iaas-api", "icon", "read", "--help"}, want: []string{"--id", "--request", "@path.json"}},
		{args: []string{"iaas-api", "icon", "create", "--help"}, want: []string{"--name", "--image-file", "PNG", "JPEG", "Base64", "--request", "併用できません"}},
		{args: []string{"iaas-api", "icon", "update", "--help"}, want: []string{"--id", "--name", "--tags", "省略した項目"}},
		{args: []string{"iaas-api", "icon", "delete", "--help"}, want: []string{"--id", "--fail-if-not-found", "出力しません"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) code = %d; stderr: %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help output does not contain %q", text)
				}
			}
		})
	}
}

func TestIaaSIconAPIWithLocalMock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		commandLine := newCLI()
		commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
			DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
		}, func() (iconapi.API, error) {
			return iconSDK.New(server), nil
		})
		if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
			t.Fatalf("run(%v) code = %d; stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr.String())
		}
		return stdout.Bytes()
	}

	imagePath := t.TempDir() + "/icon.png"
	file, err := os.Create(imagePath) //nolint:gosec // imagePath is constructed under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(file, imageData); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var created iaas.Icon
	if err := json.Unmarshal(runCommand("iaas-api", "icon", "create",
		"--name", "TEST-ICON-NAME", "--image-file", imagePath, "--tags", "test,tutorial"), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "TEST-ICON-NAME" || len(created.Tags) != 2 {
		t.Fatalf("create returned %+v, want icon fields", created)
	}
	var other iaas.Icon
	if err := json.Unmarshal(runCommand("iaas-api", "icon", "create",
		"--name", "OTHER-ICON", "--image-file", imagePath, "--tags", "other"), &other); err != nil {
		t.Fatal(err)
	}

	var found []*iaas.Icon
	if err := json.Unmarshal(runCommand("iaas-api", "icon", "find", "--request", `{"Names":["TEST-ICON-NAME"]}`), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("find returned %+v, want icon %s", found, created.ID)
	}
	for _, test := range []struct {
		name string
		args []string
		want []iaas.Icon
	}{
		{"tags", []string{"--request", `{"Tags":["test","tutorial"]}`}, []iaas.Icon{created}},
		{"no name match", []string{"--request", `{"Names":["MISSING"]}`}, nil},
		{"no tag match", []string{"--request", `{"Tags":["test","other"]}`}, nil},
		{"partial name", []string{"--request", `{"Names":["TEST","NAME"]}`}, []iaas.Icon{created}},
		{"page", []string{"--count", "1", "--from", "1"}, []iaas.Icon{other}},
		{"past end", []string{"--count", "1", "--from", "2"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"iaas-api", "icon", "find"}, test.args...)
			var items []iaas.Icon
			if err := json.Unmarshal(runCommand(args...), &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != len(test.want) {
				t.Fatalf("find returned %+v, want %+v", items, test.want)
			}
			for i := range items {
				if items[i].ID != test.want[i].ID {
					t.Fatalf("find returned %+v, want %+v", items, test.want)
				}
			}
		})
	}

	readArgs := []string{"iaas-api", "icon", "read", "--id", created.ID.String()}
	var read iaas.Icon
	if err := json.Unmarshal(runCommand(readArgs...), &read); err != nil {
		t.Fatal(err)
	}
	if read.ID != created.ID || read.Name != created.Name {
		t.Fatalf("read returned %+v, want icon %s", read, created.ID)
	}

	var updated iaas.Icon
	if err := json.Unmarshal(runCommand("iaas-api", "icon", "update", "--id", created.ID.String(),
		"--name", "UPDATED-ICON-NAME", "--tags", "updated,tutorial"), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "UPDATED-ICON-NAME" || len(updated.Tags) != 2 || !containsString(updated.Tags, "updated") || !containsString(updated.Tags, "tutorial") {
		t.Fatalf("update returned %+v, want updated name and tags", updated)
	}

	if output := runCommand("iaas-api", "icon", "delete", "--id", created.ID.String(), "--fail-if-not-found"); len(output) != 0 {
		t.Fatalf("delete output = %s, want empty", output)
	}
	runCommand("iaas-api", "icon", "delete", "--id", created.ID.String())
	commandLine := newCLI()
	commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (iconapi.API, error) { return iconSDK.New(server), nil })
	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"iaas-api", "icon", "delete", "--id", created.ID.String(), "--fail-if-not-found"},
		&stdout, &stderr, commandLine); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "not_found") {
		t.Fatalf("repeat delete code=%d stdout=%q stderr=%q; want not_found error", code, stdout.String(), stderr.String())
	}
	runCommand("iaas-api", "icon", "delete", "--id", other.ID.String())

	found = nil
	if err := json.Unmarshal(runCommand("iaas-api", "icon", "find", "--query", "map({ID,Name,Tags})"), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("find after delete returned %+v, want no icons", found)
	}
}

func TestIaaSIconCreateRequestFile(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	requestFile := t.TempDir() + "/request.json"
	if err := os.WriteFile(requestFile, []byte(`{"Name":"JSON-ICON","Image":"aW1hZ2U=","Tags":["json"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	commandLine := newCLI()
	commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (iconapi.API, error) { return iconSDK.New(server), nil })
	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"iaas-api", "icon", "create", "--request", "@" + requestFile}, &stdout, &stderr, commandLine); code != 0 {
		t.Fatalf("JSON create code = %d; stderr: %s", code, stderr.String())
	}
	var created iaas.Icon
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "JSON-ICON" || len(created.Tags) != 1 || created.Tags[0] != "json" {
		t.Fatalf("JSON create returned %+v", created)
	}
}

func TestIaaSIconReturnsSDKCreateValidationErrors(t *testing.T) {
	caller := &iconAPICaller{}
	commandLine := newCLI()
	commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (iconapi.API, error) { return iconSDK.New(caller), nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "icon", "create", "--request", `{"Name":"missing-image"}`}, &stdout, &stderr, commandLine)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Image") || !strings.Contains(stderr.String(), "required") {
		t.Fatalf("code=%d stdout=%q stderr=%q; want SDK Image validation error", code, stdout.String(), stderr.String())
	}
	if caller.calls != 0 {
		t.Fatalf("API calls = %d, want SDK validation before transport", caller.calls)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type iconAPICaller struct {
	calls int
}

func (caller *iconAPICaller) Do(context.Context, string, string, any) ([]byte, error) {
	caller.calls++
	return nil, errors.New("unexpected Icon API request")
}
