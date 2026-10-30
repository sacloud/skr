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

package apigen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeWiringConfigJSON は配線生成のテスト用に設定 JSON を一時ディレクトリへ書き込みます。
func writeWiringConfigJSON(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func sampleWiringConfigJSON(wiringTarget, sdkFactory string) string {
	return `{
  "package": "exampleapi",
  "resource": "Example",
  "imports": {
    "example": "github.com/sacloud/sacloud-sdk-go/service/iaas/example"
  },
  "wiring_target": "` + wiringTarget + `",
  "sdk_factory": "` + sdkFactory + `",
  "client_factory": "iaasclient.New",
  "operations": [
    {
      "name": "Find",
      "help": "検索します。",
      "method": "FindWithContext",
      "request_type": "example.FindRequest",
      "response_type": "[]string"
    }
  ]
}
`
}

func handwrittenWiringConfigJSON() string {
	return `{
  "package": "exampleapi",
  "resource": "Example",
  "wiring": "handwritten",
  "operations": [
    {
      "name": "Find",
      "help": "検索します。",
      "method": "FindWithContext",
      "request_type": "example.FindRequest",
      "response_type": "[]string"
    }
  ]
}
`
}

func sampleOgenWiringConfigJSON(wiringTarget, sdkFactory string) string {
	return `{
  "package": "exampleapi",
  "resource": "Example",
  "command_prefix": "Example",
  "wiring_target": "` + wiringTarget + `",
  "sdk_factory": "` + sdkFactory + `",
  "operations": [
    {
      "name": "Find",
      "help": "検索します。",
      "method": "FindWithContext",
      "request_type": "example.FindRequest",
      "response_type": "[]string"
    }
  ]
}
`
}

func TestValidateWiring(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*Config)
		wantErr string
	}{
		{
			name:   "no wiring fields",
			change: func(c *Config) {},
		},
		{
			name: "handwritten",
			change: func(c *Config) {
				c.Wiring = "handwritten"
			},
		},
		{
			name: "wired",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
				c.SDKFactory = "example.New"
			},
		},
		{
			name: "invalid wiring value",
			change: func(c *Config) {
				c.Wiring = "generated"
			},
			wantErr: `wiring must be "handwritten"`,
		},
		{
			name: "handwritten with wiring fields",
			change: func(c *Config) {
				c.Wiring = "handwritten"
				c.WiringTarget = "IaaSAPI.Example"
			},
			wantErr: "wiring handwritten cannot be combined",
		},
		{
			name: "missing wiring_target",
			change: func(c *Config) {
				c.SDKFactory = "example.New"
			},
			wantErr: "wiring_target is required for generated wiring",
		},
		{
			name: "missing sdk_factory",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
			},
			wantErr: "sdk_factory is required for generated wiring",
		},
		{
			name: "unexported wiring_target segment",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.example"
				c.SDKFactory = "example.New"
			},
			wantErr: "wiring_target segment \"example\" must be exported",
		},
		{
			name: "unresolved sdk_factory qualifier",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
				c.SDKFactory = "unknownpkg.New"
			},
			wantErr: `sdk_factory qualifier "unknownpkg" does not resolve to an import`,
		},
		{
			name: "invalid sdk_factory form",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
				c.SDKFactory = "example.NewSDK.API"
			},
			wantErr: "sdk_factory must be <package>.<Name> form",
		},
		{
			name: "client_factory resolves via mode-specific import",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
				c.SDKFactory = "example.New"
				c.ClientFactory = "iaasclient.New"
			},
		},
		{
			name: "factory_arguments with client_factory",
			change: func(c *Config) {
				c.WiringTarget = "IaaSAPI.Example"
				c.SDKFactory = "example.New"
				c.ClientFactory = "iaasclient.New"
				c.FactoryArguments = []Argument{{
					Name:  "example-id",
					Field: "ExampleID",
					Type:  "int",
					Help:  "対象 ID。",
				}}
			},
			wantErr: "factory_arguments cannot be combined with client_factory",
		},
		{
			name: "decoder resolves via config imports",
			change: func(c *Config) {
				c.WiringTarget = "EventbusAPI.Example"
				c.SDKFactory = "exampleapi.NewExampleAPI"
				c.Decoder = "exampleapi.DecodeRequest"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				Package:  "exampleapi",
				Resource: "Example",
				Imports: map[string]string{
					"example":    "github.com/sacloud/sacloud-sdk-go/service/iaas/example",
					"exampleapi": "github.com/sacloud/skr/internal/exampleapi",
				},
				Operations: []Operation{{
					Name:         "Find",
					Help:         "検索します。",
					Method:       "FindWithContext",
					RequestType:  "example.FindRequest",
					ResponseType: "[]string",
				}},
			}
			test.change(&config)
			err := config.Validate(ModeIaaS)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestGenerateWiring(t *testing.T) {
	path := writeWiringConfigJSON(t, "iaas-example.json", sampleWiringConfigJSON("IaaSAPI.Example", "example.New"))
	got, err := GenerateWiring([]string{path}, ModeIaaS)
	require.NoError(t, err)
	for _, want := range []string{
		"// Code generated by cmd/apigen-wiring; DO NOT EDIT.",
		"func (c *cli) initIaaSAPI() {",
		`c.IaaSAPI.Example = exampleapi.NewCommands(exampleapi.Runtime{`,
		"DecodeRequest:  decodeRequest,",
		"client, err := iaasclient.New(c.Trace)",
		"return example.New(client), nil",
	} {
		require.Contains(t, string(got), want, "generated wiring does not contain %q", want)
	}

	// 再生成が決定的であること
	second, err := GenerateWiring([]string{path}, ModeIaaS)
	require.NoError(t, err)
	require.Equal(t, string(got), string(second), "wiring generation is not deterministic")
}

func TestGenerateWiringSkipsHandwritten(t *testing.T) {
	handwritten := writeWiringConfigJSON(t, "iaas-handwritten.json", handwrittenWiringConfigJSON())
	_, err := GenerateWiring([]string{handwritten}, ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no wirable configs are provided")
}

func TestGenerateWiringRejectsMixedRoots(t *testing.T) {
	first := writeWiringConfigJSON(t, "iaas-first.json", sampleWiringConfigJSON("IaaSAPI.Example", "example.New"))
	second := writeWiringConfigJSON(t, "iaas-second.json", sampleWiringConfigJSON("OtherAPI.Example", "example.New"))
	_, err := GenerateWiring([]string{first, second}, ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), `wiring_target root "OtherAPI" does not match "IaaSAPI"`)
}

func TestGenerateWiringRejectsMissingWiringTarget(t *testing.T) {
	content := strings.Replace(sampleWiringConfigJSON("IaaSAPI.Example", "example.New"), `"wiring_target": "IaaSAPI.Example",`, ``, 1)
	path := writeWiringConfigJSON(t, "iaas-example.json", content)
	_, err := GenerateWiring([]string{path}, ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wiring_target is required for generated wiring")
}

func TestGenerateWiringDirectFactory(t *testing.T) {
	// client_factory を持たない Ogen 系設定は sdk_factory を c.Trace で直接呼び出します
	content := strings.Replace(sampleOgenWiringConfigJSON("ExampleAPI.Example", "exampleapi.NewExampleAPI"), `"client_factory": "iaasclient.New",`, ``, 1)
	path := writeWiringConfigJSON(t, "example.json", content)
	got, err := GenerateWiring([]string{path}, ModeOgen)
	require.NoError(t, err)
	require.Contains(t, string(got), "return exampleapi.NewExampleAPI(c.Trace)")
	require.NotContains(t, string(got), "iaasclient")
}

func TestGenerateWiringFactoryArguments(t *testing.T) {
	// factory_arguments がある設定は、factory_arguments を引数に取る factory クロージャを生成します。
	// sdk_factory は c.Trace に続けて factory_arguments を受け取ります。
	content := strings.Replace(sampleOgenWiringConfigJSON("ExampleAPI.Example", "exampleapi.NewExampleAPI"), `"client_factory": "iaasclient.New",`, ``, 1)
	content = strings.Replace(content, `"operations": [`, `"factory_arguments": [
    {"name": "user-id", "field": "UserID", "type": "int", "help": "対象ユーザー ID。"}
  ],
  "operations": [`, 1)
	path := writeWiringConfigJSON(t, "example.json", content)
	got, err := GenerateWiring([]string{path}, ModeOgen)
	require.NoError(t, err)
	require.Contains(t, string(got), "func(UserID int) (exampleapi.ExampleAPI, error) {")
	require.Contains(t, string(got), "return exampleapi.NewExampleAPI(c.Trace, UserID)")

	// 再生成が決定的であること
	second, err := GenerateWiring([]string{path}, ModeOgen)
	require.NoError(t, err)
	require.Equal(t, string(got), string(second), "wiring generation is not deterministic")
}

func TestGenerateWiringFile(t *testing.T) {
	path := writeWiringConfigJSON(t, "iaas-example.json", sampleWiringConfigJSON("IaaSAPI.Example", "example.New"))
	output := filepath.Join(t.TempDir(), "example_wiring_generated.go")
	require.NoError(t, GenerateWiringFile([]string{path}, output, ModeIaaS))
	written, err := os.ReadFile(output) // #nosec G304 -- The output path is provided by the test.
	require.NoError(t, err)
	require.Contains(t, string(written), "func (c *cli) initIaaSAPI() {")
}

func TestGenerateWiringUnknownMode(t *testing.T) {
	path := writeWiringConfigJSON(t, "iaas-example.json", sampleWiringConfigJSON("IaaSAPI.Example", "example.New"))
	_, err := GenerateWiring([]string{path}, Mode(99))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown generator mode: 99")
}
