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
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	config := Config{
		Package:  "main",
		Resource: "switch",
		Imports: map[string]string{
			"swytch": "github.com/sacloud/sacloud-sdk-go/service/iaas/swytch",
		},
		Operations: []Operation{
			{
				Name:         "Find",
				CommandName:  "find",
				Help:         "Find `switches`.",
				Method:       "FindWithContext",
				RequestType:  "swytch.FindRequest",
				ResponseType: "[]string",
				Handwritten:  true,
			},
			{
				Name:         "Search",
				CommandName:  "search",
				Help:         "Search with generated flags.",
				Method:       "SearchWithContext",
				RequestType:  "swytch.FindRequest",
				ResponseType: "[]string",
				Flags: []Flag{
					{Name: "zone", Field: "Zone", Type: "string", Help: "Target `zone`.", Required: true},
					{Name: "count", Field: "Count", Type: "int", Help: "Maximum results.", Pointer: true},
					{Name: "tags", Field: "Tags", Type: "[]string", Help: "Tags.", Separator: ","},
				},
			},
			{
				Name:             "Delete",
				CommandName:      "delete",
				Help:             "Delete a switch.",
				Method:           "DeleteWithContext",
				RequestType:      "swytch.DeleteRequest",
				RequestValidator: "ValidateDeleteRequest",
			},
			{
				Name:         "Version",
				CommandName:  "version",
				Help:         "Print the API version.",
				Method:       "VersionWithContext",
				ResponseType: "string",
			},
		},
	}

	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "Generate() error")
	_, err = parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err, "generated source does not parse")

	for _, want := range []string{
		"FindWithContext(context.Context, *swytch.FindRequest) ([]string, error)",
		"SearchWithContext(context.Context, *swytch.FindRequest) ([]string, error)",
		"DeleteWithContext(context.Context, *swytch.DeleteRequest) error",
		"VersionWithContext(context.Context) (string, error)",
		"--request と個別フラグは併用できません",
		"if c.Count != nil",
		"request.Count = c.Count",
		"if c.Zone == nil",
		"request.Zone = *c.Zone",
		"Tags",
		"sep:\\\",\\\"",
		"request.Tags = *c.Tags",
		"c.runtime.WriteOutput(ctx, result)",
		"if err := c.runtime.ValidateOutput(ctx); err != nil",
		"return op.DeleteWithContext(context.Background(), request)",
		"if err := ValidateDeleteRequest(request); err != nil",
		"FindCommand",
		`name:\"find\"`,
		"commands.Find.runtime = runtime",
		"commands.Find.factory = factory",
		"op.SearchWithContext(context.Background(), request)",
	} {
		require.Contains(t, string(source), want, "generated source does not contain %q", want)
	}
	require.NotContains(t, string(source), "type FindCommand struct", "handwritten operation command was generated")
	require.NotContains(t, string(source), "ValidateDeleteRequest func(",
		"request validator is configured through Runtime instead of called directly")
	require.NotContains(t, string(source), "c.runtime.ValidateDeleteRequest",
		"request validator is configured through Runtime instead of called directly")
	for _, removed := range []string{"OutputType", "tableZones", "format,", "SetZoneFactory", "zoneFactory", "zones."} {
		require.NotContains(t, string(source), removed, "generated source contains removed output machinery %q", removed)
	}

	second, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "second Generate() error")
	require.Equal(t, string(source), string(second), "Generate() output is not deterministic")
}

func TestGenerateIaaSDerivesFixedTypeNames(t *testing.T) {
	config := Config{
		Package:  "exampleapi",
		Resource: "Example",
		Operations: []Operation{{
			Name:         "List",
			Help:         "一覧を表示します。",
			Method:       "List",
			RequestType:  "ListParams",
			ResponseType: "[]string",
		}},
	}
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	for _, want := range []string{
		"type Commands struct",
		"func NewCommands(runtime Runtime, factory APIFactory) Commands",
		"type API interface",
		"type APIFactory func() (API, error)",
		"type Runtime struct",
		"List ListCommand",
		"type ListCommand struct",
	} {
		require.Contains(t, string(source), want, "IaaS 生成コードに %q がありません", want)
	}
}

func TestGenerateOgenDerivesTypeNamesFromPrefix(t *testing.T) {
	config := Config{
		Package:       "scheduleapi",
		Resource:      "Schedule",
		CommandPrefix: "Schedule",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/schedule",
		},
		Operations: []Operation{{
			Name:         "List",
			Help:         "一覧を表示します。",
			Method:       "List",
			RequestType:  "example.ListParams",
			ResponseType: "[]example.Item",
		}},
	}
	source, err := Generate(config, ModeOgen)
	require.NoError(t, err)
	_, err = parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err, "generated source does not parse")
	for _, want := range []string{
		"type ScheduleCommands struct",
		"func NewScheduleCommands(runtime ScheduleRuntime, factory ScheduleAPIFactory) ScheduleCommands",
		"type ScheduleAPI interface",
		"type ScheduleAPIFactory func() (ScheduleAPI, error)",
		"type ScheduleRuntime struct",
		"List ScheduleListCommand",
		"type ScheduleListCommand struct",
	} {
		require.Contains(t, string(source), want, "Ogen 生成コードに %q がありません", want)
	}
}

func TestValidateOperationCommandNames(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		Operations: []Operation{
			{Name: "ReadIPv6", CommandName: "read-ipv6", Help: "IPv6 を読み取ります。", Method: "ReadIPv6"},
			{Name: "EnableIPv6", CommandName: "enable-ipv6", Help: "IPv6 を有効にします。", Method: "EnableIPv6"},
		},
	}
	require.NoError(t, config.Validate(ModeIaaS))
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	for _, want := range []string{`name:\"read-ipv6\"`, `name:\"enable-ipv6\"`} {
		require.Contains(t, string(source), want, "generated source does not contain %q", want)
	}

	config.Operations[1].CommandName = "read-ipv6"
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate command name")

	config.Operations[1].CommandName = ""
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "every operation or none")

	config.Operations[1].CommandName = "Enable-IPv6"
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "command_name must be kebab-case")
}

func TestGenerateHandwrittenRequestValidator(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		Operations: []Operation{{
			Name: "Boot", Help: "起動します。",
			Method: "Boot", RequestType: "BootRequest", RequestValidator: "ValidateBootRequest", Handwritten: true,
		}},
	}
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	text := string(source)
	require.Contains(t, text, "Boot BootCommand")
	require.NotContains(t, text, "type BootCommand struct", "handwritten operation command was generated")
}

func TestGenerateFileOnlyRequestInput(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		Operations: []Operation{{
			Name: "Create", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", RequestFileOnly: true,
		}},
	}
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	for _, want := range []string{
		`if *c.Request != "-" && (len(*c.Request) == 0 || (*c.Request)[0] != '@')`,
		"--request には @path.json または -（標準入力）を指定してください",
	} {
		require.Contains(t, string(source), want, "generated source does not contain %q", want)
	}
	config.Operations[0].RequestType = ""
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "request_file_only requires request_type")

	config.Operations[0].RequestType = "CreateRequest"
	for _, test := range []struct {
		name   string
		change func(*Operation)
	}{
		{"optional", func(op *Operation) { op.RequestOptional = true }},
		{"handwritten", func(op *Operation) { op.Handwritten = true }},
		{"flags", func(op *Operation) { op.Flags = []Flag{{Name: "name", Field: "Name", Type: "string"}} }},
		{"repeated flags", func(op *Operation) { op.RepeatedFlags = []RepeatedFlag{{Name: "item", Field: "Item"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copyConfig := config
			copyConfig.Operations = append([]Operation(nil), config.Operations...)
			test.change(&copyConfig.Operations[0])
			err := copyConfig.Validate(ModeIaaS)
			require.Error(t, err)
			require.Contains(t, err.Error(), "request_file_only cannot be combined")
		})
	}
}

func TestGenerateZoneValidation(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		Operations: []Operation{{
			Name: "Create", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", ValidateZone: true,
			RequestValidator: "ValidateCreateRequest", RequestTransform: "TransformCreateRequest",
			Flags: []Flag{{Name: "zone", Field: "Zone", Type: "string", Help: "対象ゾーンです。"}},
		}},
	}
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	text := string(source)
	require.Contains(t, text, `zones "github.com/sacloud/skr/internal/iaas/zones"`, "shared zone validator import is missing")
	zoneValidation := strings.Index(text, "if err := zones.ValidateSingleZone(request.Zone); err != nil")
	transform := strings.Index(text, "if err := c.runtime.TransformCreateRequest(c, request); err != nil")
	validator := strings.Index(text, "if err := ValidateCreateRequest(request); err != nil")
	factory := strings.Index(text, "op, err := c.factory(")
	require.NotEqual(t, -1, zoneValidation)
	require.NotEqual(t, -1, transform)
	require.NotEqual(t, -1, validator)
	require.NotEqual(t, -1, factory)
	require.Less(t, transform, zoneValidation, "transform must precede zone validation")
	require.Less(t, zoneValidation, validator, "zone validation must precede domain validation")
	require.Less(t, validator, factory, "domain validation must precede API setup")
	second, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.Equal(t, text, string(second), "second Generate() is not deterministic")

	config.Operations[0].ValidateZone = false
	source, err = Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.NotContains(t, string(source), "zones", "disabled zone validation added an import or validation call")
}

func TestValidateZoneSetting(t *testing.T) {
	base := `{"package":"exampleapi","resource":"Example","operations":[{"name":"Find","help":"検索します。","method":"Find","request_type":"FindRequest","validate_zone":true}]}`
	for _, test := range []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{name: "valid"},
		{name: "no request", change: func(c *Config) { c.Operations[0].RequestType = "" }, want: "requires a generated request operation"},
		{name: "handwritten", change: func(c *Config) { c.Operations[0].Handwritten = true }, want: "requires a generated request operation"},
		{name: "import collision", change: func(c *Config) { c.Imports = map[string]string{"zones": "example.test/zones"} }, want: "reserved import alias"},
		{name: "validator collision", change: func(c *Config) { c.Operations[0].RequestValidator = "zones" }, want: "conflicts with an import alias"},
		{name: "type alias collision", change: func(c *Config) { c.Imports = map[string]string{"Commands": "example.test/commands"} }, want: `generated type "Commands" conflicts with an import alias`},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := DecodeConfig([]byte(base), ModeIaaS)
			require.NoError(t, err)
			if test.change != nil {
				test.change(&config)
			}
			err = config.Validate(ModeIaaS)
			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.want)
			}
		})
	}
}

func TestGenerateOptionalRequest(t *testing.T) {
	config := Config{
		Package:  "exampleapi",
		Resource: "Example API",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		Operations: []Operation{
			{
				Name:            "List",
				Help:            "List resources.",
				Method:          "List",
				RequestType:     "example.ListParams",
				RequestByValue:  true,
				RequestOptional: true,
				ResponseType:    "[]example.Resource",
			},
		},
	}

	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "Generate() error")
	require.Contains(t, string(source), "result, err := op.List(context.Background(), *request)",
		"generated source does not invoke List with the zero-value request")
	require.NotContains(t, string(source), "--request または個別フラグが必要です",
		"generated source requires an optional request")
}

func TestGenerateValueRequestAndPositionalArguments(t *testing.T) {
	config := Config{
		Package:  "exampleapi",
		Resource: "Example API",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		Operations: []Operation{
			{
				Name:           "Update",
				Help:           "更新します。",
				Method:         "Update",
				RequestType:    "example.UpdateRequest",
				RequestByValue: true,
				ResponseType:   "*example.Item",
				Arguments: []Argument{
					{Name: "id", Field: "ID", Type: "string", Help: "更新対象の ID。"},
				},
			},
		},
	}

	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "Generate() error")
	_, err = parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err, "generated source does not parse")
	for _, want := range []string{
		"Update(context.Context, string, example.UpdateRequest) (*example.Item, error)",
		`arg:\"\" name:\"id\" help:\"更新対象の ID。\"`,
		"op.Update(context.Background(), c.ID, *request)",
	} {
		require.Contains(t, string(source), want, "generated source does not contain %q", want)
	}
}

func TestGenerateHandwrittenOnlyOperationsOmitFmtImport(t *testing.T) {
	config := Config{
		Package:  "zoneapi",
		Resource: "IaaS Zone",
		Imports: map[string]string{
			"iaas": "github.com/sacloud/sacloud-sdk-go/api/iaas",
			"zone": "github.com/sacloud/sacloud-sdk-go/service/iaas/zone",
		},
		Operations: []Operation{{
			Name:         "Find",
			Help:         "ゾーン一覧を取得します。",
			Method:       "FindWithContext",
			RequestType:  "zone.FindRequest",
			ResponseType: "[]*iaas.Zone",
			Handwritten:  true,
		}},
	}

	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "Generate() error")
	_, err = parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err, "generated source does not parse")
	require.NotContains(t, string(source), `"fmt"`, "generated source imports fmt without generated handlers")
}

func TestGenerateTimeFlag(t *testing.T) {
	baseConfig := Config{
		Package:  "internetapi",
		Resource: "IaaS Internet",
		Operations: []Operation{{
			Name:        "MonitorRouter",
			Help:        "取得します。",
			Method:      "MonitorRouterWithContext",
			RequestType: "MonitorRouterRequest",
			Flags: []Flag{
				{Name: "zone", Field: "Zone", Type: "string", Help: "Zone.", Required: true},
				{Name: "id", Field: "ID", Type: "int64", Help: "ID.", Required: true, Conversion: "types.ID"},
				{Name: "start", Field: "Start", Type: "time.Time", Help: "Start."},
				{Name: "end", Field: "End", Type: "time.Time", Help: "End."},
			},
		}},
	}

	t.Run("generates time flags and import", func(t *testing.T) {
		source, err := Generate(baseConfig, ModeIaaS)
		require.NoError(t, err)
		generated := string(source)
		require.Contains(t, generated, "\t\"time\"")
		require.Regexp(t, `Start\s+\*time\.Time`, generated)
		require.Regexp(t, `End\s+\*time\.Time`, generated)
		require.Contains(t, generated, "request.Start = *c.Start")
		require.Contains(t, generated, "request.End = *c.End")
	})

	t.Run("omits time import without time flags", func(t *testing.T) {
		config := baseConfig
		config.Operations = []Operation{{
			Name:        "MonitorRouter",
			Help:        "取得します。",
			Method:      "MonitorRouterWithContext",
			RequestType: "MonitorRouterRequest",
			Flags: []Flag{
				{Name: "zone", Field: "Zone", Type: "string", Help: "Zone.", Required: true},
			},
		}}
		source, err := Generate(config, ModeIaaS)
		require.NoError(t, err)
		require.NotContains(t, string(source), "\t\"time\"")
	})
}

func TestGenerateFactoryArgumentsAndNamedMethodArguments(t *testing.T) {
	config := Config{
		Package:  "exampleapi",
		Resource: "Example API",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		FactoryArguments: []Argument{
			{Name: "queue-name", Field: "QueueName", Type: "string", Help: "対象キュー名。", Required: true},
			{Name: "api-key-file", Field: "APIKeyFile", Type: "string", Help: "API キーファイル。", Required: true},
		},
		Operations: []Operation{{
			Name:         "Send",
			Help:         "送信します。",
			Method:       "Send",
			ResponseType: "*example.Item",
			Arguments: []Argument{
				{Name: "content", Field: "Content", Type: "string", Help: "本文。", Flag: true, Required: true},
			},
		}},
	}

	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err, "Generate() error")
	_, err = parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors)
	require.NoError(t, err, "generated source does not parse")
	for _, want := range []string{
		"type APIFactory func(QueueName string, APIKeyFile string) (API, error)",
		"func NewCommands(runtime Runtime, factory APIFactory) Commands",
		`name:\"queue-name\" help:\"対象キュー名。\" required:\"\"`,
		`name:\"content\" help:\"本文。\" required:\"\"`,
		"op.Send(context.Background(), c.Content)",
		"c.factory(c.QueueName, c.APIKeyFile)",
	} {
		require.Contains(t, string(source), want, "generated source does not contain %q", want)
	}
	require.NotContains(t, string(source), ".SetRuntime(", "generated source contains dependency setters")
	require.NotContains(t, string(source), ".SetFactory(", "generated source contains dependency setters")
}

func TestDecodeConfigRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, input := range []string{
		`{"package":"main","unexpected":true}`,
		`{"package":"main"} {}`,
		`{"operations":[{"zone_search":{"flag_field":"Zone","request_field":"Zone"}}]}`,
		// 生成型名の設定宣言は廃止済み (ADR 0067)。旧フィールドは未知フィールドとして拒否します。
		`{"package":"main","command_type":"Commands","api_type":"API","factory_type":"APIFactory","runtime_type":"Runtime","operations":[{"name":"List","help":"一覧。","method":"List","command_type":"ListCommand"}]}`,
	} {
		_, err := DecodeConfig([]byte(input), ModeIaaS)
		require.Error(t, err, "DecodeConfig(%q) succeeded; want error", input)
	}
}

func TestValidateRequestByValueRequiresRequestType(t *testing.T) {
	config := Config{
		Package:  "exampleapi",
		Resource: "Example API",
		Operations: []Operation{{
			Name:           "List",
			Help:           "一覧を表示します。",
			Method:         "List",
			RequestByValue: true,
		}},
	}
	err := config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "request_by_value requires request_type")
}

func TestValidateRejectsUnsupportedFlagsAndDuplicateNames(t *testing.T) {
	config := Config{
		Package:  "main",
		Resource: "switch",
		Operations: []Operation{
			{
				Name:        "Find",
				Help:        "Find.",
				Method:      "FindWithContext",
				RequestType: "FindRequest",
				Flags:       []Flag{{Name: "zone", Field: "Zone", Type: "[]int", Help: "Zone."}},
			},
		},
	}

	err := config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")

	config.Operations[0].Flags[0].Type = "string"
	config.Operations[0].Flags[0].Separator = ","
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "separator requires []string type")

	config.Operations[0].Flags[0].Separator = ""
	// Tags 以外の配列フラグを追加する場合は、カンマを要素値に使えない根拠を確認して ADR に記録するか、同一フラグの複数指定方式を検討します。
	config.Operations[0].Flags[0] = Flag{
		Name:      "names",
		Field:     "Names",
		Type:      "[]string",
		Help:      "Names.",
		Separator: ",",
	}
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "supported only for the tags flag")

	config.Operations[0].Flags[0] = Flag{
		Name:      "tags",
		Field:     "Tags",
		Type:      "[]string",
		Help:      "Tags.",
		Separator: ";",
	}
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "supported only for the tags flag")

	config.Operations[0].Flags[0] = Flag{
		Name:      "tags",
		Field:     "Tags",
		Type:      "[]string",
		Help:      "Tags.",
		Separator: ",",
	}
	require.NoError(t, config.Validate(ModeIaaS), "want valid tags array flag")

	config.Operations[0].Flags[0] = Flag{Name: "zone", Field: "Zone", Type: "string", Help: "Zone."}
	config.Operations = append(config.Operations, config.Operations[0])
	err = config.Validate(ModeIaaS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate operation")
}

func TestValidateModeSpecificPrefixRules(t *testing.T) {
	operations := []Operation{{
		Name: "List", Help: "一覧を表示します。", Method: "List", RequestType: "ListParams",
	}}

	t.Run("IaaS rejects command_prefix", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", CommandPrefix: "Switch", Operations: operations}
		err := config.Validate(ModeIaaS)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command_prefix is not supported for IaaS API generator configs")
	})
	t.Run("Ogen requires command_prefix", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", Operations: operations}
		err := config.Validate(ModeOgen)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command_prefix is required for Ogen API generator configs")
	})
	t.Run("Ogen rejects unexported command_prefix", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", CommandPrefix: "schedule", Operations: operations}
		err := config.Validate(ModeOgen)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command_prefix must be an exported Go identifier")
	})
	t.Run("Ogen rejects keyword command_prefix", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", CommandPrefix: "type", Operations: operations}
		err := config.Validate(ModeOgen)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command_prefix must be an exported Go identifier")
	})
	t.Run("Ogen rejects non-identifier command_prefix", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", CommandPrefix: "Schedule API", Operations: operations}
		err := config.Validate(ModeOgen)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command_prefix must be an exported Go identifier")
	})
	t.Run("unknown mode", func(t *testing.T) {
		config := Config{Package: "exampleapi", Resource: "Example", Operations: operations}
		err := config.Validate(Mode(99))
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown generator mode")
	})
}

func TestSingletonNameFlag(t *testing.T) {
	config := transformTestConfig()
	config.Operations[0].Name = "Find"
	config.Operations[0].Flags = []Flag{{Name: "name", Field: "Names", Type: "string", Help: "名前です。", Singleton: true}}
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.Contains(t, string(source), "request.Names = []string{*c.Names}", "singleton assignment missing")
	second, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.Equal(t, string(source), string(second), "non-deterministic generation")

	valid := config.Operations[0].Flags[0]
	for _, mutate := range []func(*Flag){
		func(f *Flag) { f.Name = "names" },
		func(f *Flag) { f.Field = "Other" },
		func(f *Flag) { f.Type = "int" },
		func(f *Flag) { f.Pointer = true },
		func(f *Flag) { f.Conversion = "string" },
		func(f *Flag) { f.Separator = "," },
	} {
		flag := valid
		mutate(&flag)
		config.Operations[0].Flags[0] = flag
		require.Error(t, config.Validate(ModeIaaS), "accepted invalid singleton flag: %+v", flag)
	}
	config.Operations[0].Flags[0] = valid
	config.Operations[0].Name = "Create"
	require.Error(t, config.Validate(ModeIaaS), "accepted singleton outside Find")
}

func transformTestConfig() Config {
	return Config{
		Package: "exampleapi", Resource: "Example",
		Operations: []Operation{{
			Name: "Create", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", RequestTransform: "TransformCreateRequest",
			RequestValidator: "ValidateCreateRequest",
			Flags:            []Flag{{Name: "name", Field: "Name", Type: "string", Required: true, Help: "名前です。"}},
			RepeatedFlags:    []RepeatedFlag{{Name: "item", Field: "Items", Help: "繰り返し指定します。"}},
		}},
	}
}

func TestGenerateRequestTransform(t *testing.T) {
	config := transformTestConfig()
	source, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	text := string(source)
	for _, want := range []string{
		"TransformCreateRequest func(*CreateCommand, *CreateRequest) error",
		"Items",
		"[]string",
		`sep:\"none\"`,
		"flagsSet = flagsSet || len(c.Items) != 0",
		"if c.runtime.TransformCreateRequest == nil",
		"if err := c.runtime.TransformCreateRequest(c, request); err != nil",
		"API リクエスト入力変換が設定されていません",
	} {
		require.Contains(t, text, want, "generated source does not contain %q", want)
	}
	assign := strings.Index(text, "request.Name = *c.Name")
	transform := strings.Index(text, "c.runtime.TransformCreateRequest(c, request)")
	validate := strings.Index(text, "if err := ValidateCreateRequest(request); err != nil")
	require.NotEqual(t, -1, assign)
	require.Greater(t, transform, assign, "transform must follow scalar assignment")
	require.Greater(t, validate, transform, "validation must follow transform")
	require.NotContains(t, text, "request.Items =", "repeated input must not be assigned directly to the API request")
	second, err := Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.Equal(t, text, string(second), "generation is not deterministic")

	config.Operations[0].Flags = nil
	source, err = Generate(config, ModeIaaS)
	require.NoError(t, err)
	require.NotContains(t, string(source), `return fmt.Errorf("--request が必要です")`,
		"repeated flags alone must provide the flag input path")
}

func TestValidateRequestTransform(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"missing transform", func(c *Config) { c.Operations[0].RequestTransform = "" }, "requires request_transform"},
		{"invalid transform", func(c *Config) { c.Operations[0].RequestTransform = "x.y" }, "exported Go identifier"},
		{"private transform", func(c *Config) { c.Operations[0].RequestTransform = "transform" }, "exported Go identifier"},
		{"reserved callback", func(c *Config) { c.Operations[0].RequestTransform = "DecodeRequest" }, "duplicate runtime field"},
		{"no request", func(c *Config) { c.Operations[0].RequestType = "" }, "requires a request_type"},
		{"request validator type collision", func(c *Config) { c.Operations[0].RequestValidator = "CreateCommand" }, "conflicts with a generated type"},
		{"request validator import collision", func(c *Config) {
			c.Imports = map[string]string{"ValidateCreateRequest": "example/sdk"}
		}, "conflicts with an import alias"},
		{"request validator transform collision", func(c *Config) {
			c.Operations[0].RequestValidator = "TransformCreateRequest"
		}, "conflicts with a generated Runtime field"},
		{"request validator c collision", func(c *Config) {
			c.Operations[0].RequestValidator = "c"
		}, "conflicts with a generated local name"},
		{"request validator ctx collision", func(c *Config) {
			c.Operations[0].RequestValidator = "ctx"
		}, "conflicts with a generated local name"},
		{"request validator flagsSet collision", func(c *Config) {
			c.Operations[0].RequestValidator = "flagsSet"
		}, "conflicts with a generated local name"},
		{"request validator request collision", func(c *Config) {
			c.Operations[0].RequestValidator = "request"
		}, "conflicts with a generated local name"},
		{"constructor conflicts with request validator", func(c *Config) {
			c.Operations[0].RequestValidator = "NewCommands"
		}, "constructor \"NewCommands\" conflicts with a request_validator"},
		{"constructor conflicts with import alias", func(c *Config) {
			c.Imports = map[string]string{"NewCommands": "example/sdk"}
		}, "constructor \"NewCommands\" conflicts with an import alias"},
		{"validator reused for different request types", func(c *Config) {
			op := c.Operations[0]
			op.Name, op.Method, op.RequestType, op.RequestTransform, op.RepeatedFlags =
				"Update", "Update", "UpdateRequest", "", nil
			c.Operations = append(c.Operations, op)
		}, "used with multiple request types"},
		{"invalid name", func(c *Config) { c.Operations[0].RepeatedFlags[0].Name = "Item" }, "kebab-case"},
		{"invalid field", func(c *Config) { c.Operations[0].RepeatedFlags[0].Field = "a.b" }, "field identifier"},
		{"method field", func(c *Config) { c.Operations[0].RepeatedFlags[0].Field = "Run" }, "field identifier"},
		{"empty help", func(c *Config) { c.Operations[0].RepeatedFlags[0].Help = "" }, "help must not be empty"},
		{"request name", func(c *Config) { c.Operations[0].RepeatedFlags[0].Name = "request" }, "duplicate repeated flag"},
		{"runtime field", func(c *Config) { c.Operations[0].RepeatedFlags[0].Field = "runtime" }, "duplicate repeated flag"},
		{"scalar name", func(c *Config) { c.Operations[0].RepeatedFlags[0].Name = "name" }, "duplicate repeated flag"},
		{"scalar field", func(c *Config) { c.Operations[0].RepeatedFlags[0].Field = "Name" }, "duplicate repeated flag"},
		{"duplicate input", func(c *Config) {
			c.Operations[0].RepeatedFlags = append(c.Operations[0].RepeatedFlags, c.Operations[0].RepeatedFlags[0])
		}, "duplicate repeated flag"},
		{"argument name", func(c *Config) {
			c.Operations[0].Arguments = []Argument{{Name: "item", Field: "ID", Type: "string", Help: "ID です。"}}
		}, "duplicate repeated flag"},
		{"factory field", func(c *Config) {
			c.FactoryArguments = []Argument{{Name: "tenant", Field: "Items", Type: "string", Help: "対象です。"}}
		}, "duplicate repeated flag"},
		{"duplicate callback", func(c *Config) {
			op := c.Operations[0]
			op.Name, op.Method = "Update", "Update"
			c.Operations = append(c.Operations, op)
		}, "duplicate runtime field"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := transformTestConfig()
			test.change(&config)
			err := config.Validate(ModeIaaS)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}
