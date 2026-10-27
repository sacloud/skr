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
	"github.com/stretchr/testify/require"
)

func TestIconGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-icon.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData)
	require.NoError(t, err)
	want, err := apigen.Generate(config)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/iconapi/icon_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Icon commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-icon.json API_OUTPUT=internal/iaas/iconapi/icon_api_generated.go")
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
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
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
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.EqualValues(t, 0, stderr.Len(), "run(%v) stderr = %q, want empty", args, stderr.String())
		return stdout.Bytes()
	}

	imagePath := t.TempDir() + "/icon.png"
	file, err := os.Create(imagePath) //nolint:gosec // imagePath は t.TempDir 内で構築される。
	require.NoError(t, err)
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(file, imageData); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	err = file.Close()
	require.NoError(t, err)
	var created iaas.Icon
	require.NoError(t, json.Unmarshal(runCommand("iaas-api", "icon", "create",
		"--name", "TEST-ICON-NAME", "--image-file", imagePath, "--tags", "test,tutorial"), &created))
	require.NotEqualValues(t, 0, created.ID, "create returned %+v, want icon fields", created)
	require.Equal(t, "TEST-ICON-NAME", created.Name, "unexpected value")
	require.Len(t, created.Tags, 2, "unexpected length")
	var other iaas.Icon
	require.NoError(t, json.Unmarshal(runCommand("iaas-api", "icon", "create",
		"--name", "OTHER-ICON", "--image-file", imagePath, "--tags", "other"), &other))

	var found []*iaas.Icon
	err2 := json.Unmarshal(runCommand("iaas-api", "icon", "find", "--request", `{"Names":["TEST-ICON-NAME"]}`), &found)
	require.NoError(t, err2)
	require.Len(t, found, 1, "find returned %+v, want icon %s", found, created.ID)
	require.Equal(t, created.ID, found[0].ID, "unexpected value")
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
			err3 := json.Unmarshal(runCommand(args...), &items)
			require.NoError(t, err3)
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
	err4 := json.Unmarshal(runCommand(readArgs...), &read)
	require.NoError(t, err4)
	require.Equal(t, created.ID, read.ID, "read returned %+v, want icon %s", read, created.ID)
	require.Equal(t, created.Name, read.Name, "unexpected value")

	var updated iaas.Icon
	require.NoError(t, json.Unmarshal(runCommand("iaas-api", "icon", "update", "--id", created.ID.String(),
		"--name", "UPDATED-ICON-NAME", "--tags", "updated,tutorial"), &updated))
	if updated.Name != "UPDATED-ICON-NAME" || len(updated.Tags) != 2 || !containsString(updated.Tags, "updated") || !containsString(updated.Tags, "tutorial") {
		t.Fatalf("update returned %+v, want updated name and tags", updated)
	}

	output := runCommand("iaas-api", "icon", "delete", "--id", created.ID.String(), "--fail-if-not-found")
	require.Len(t, output, 0, "delete output = %s, want empty", output)
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
	err5 := json.Unmarshal(runCommand("iaas-api", "icon", "find", "--query", "map({ID,Name,Tags})"), &found)
	require.NoError(t, err5)
	require.Len(t, found, 0, "find after delete returned %+v, want no icons", found)
}

func TestIaaSIconCreateRequestFile(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	requestFile := t.TempDir() + "/request.json"
	err := os.WriteFile(requestFile, []byte(`{"Name":"JSON-ICON","Image":"aW1hZ2U=","Tags":["json"]}`), 0o600)
	require.NoError(t, err)

	commandLine := newCLI()
	commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (iconapi.API, error) { return iconSDK.New(server), nil })
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runCLI([]string{"iaas-api", "icon", "create", "--request", "@" + requestFile}, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	var created iaas.Icon
	err2 := json.Unmarshal(stdout.Bytes(), &created)
	require.NoError(t, err2)
	require.Equal(t, "JSON-ICON", created.Name, "JSON create returned %+v", created)
	require.Len(t, created.Tags, 1, "unexpected length")
	require.Equal(t, "json", created.Tags[0], "unexpected value")
}

func TestIaaSIconReturnsSDKCreateValidationErrors(t *testing.T) {
	caller := &iconAPICaller{}
	commandLine := newCLI()
	commandLine.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (iconapi.API, error) { return iconSDK.New(caller), nil })
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "icon", "create", "--request", `{"Name":"missing-image"}`}, &stdout, &stderr, commandLine)
	require.EqualValues(t, 1, code, "code=%d stdout=%q stderr=%q; want SDK Image validation error", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), "Image", "missing expected text %q", "Image")
	require.Contains(t, stderr.String(), "required", "missing expected text %q", "required")
	require.EqualValues(t, 0, caller.calls, "API calls = %d, want SDK validation before transport", caller.calls)
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
