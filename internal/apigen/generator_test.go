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
)

func TestGenerate(t *testing.T) {
	config := Config{
		Package:     "main",
		Resource:    "switch",
		CommandType: "switchCommands",
		APIType:     "switchAPI",
		FactoryType: "switchAPIFactory",
		RuntimeType: "switchAPIRuntime",
		Imports: map[string]string{
			"swytch": "github.com/sacloud/sacloud-sdk-go/service/iaas/swytch",
		},
		Operations: []Operation{
			{
				Name:         "Find",
				CommandName:  "find",
				CommandType:  "switchFindCommand",
				Help:         "Find `switches`.",
				Method:       "FindWithContext",
				RequestType:  "swytch.FindRequest",
				ResponseType: "[]string",
				Handwritten:  true,
			},
			{
				Name:         "Search",
				CommandName:  "search",
				CommandType:  "switchSearchCommand",
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
				CommandType:      "switchDeleteCommand",
				Help:             "Delete a switch.",
				Method:           "DeleteWithContext",
				RequestType:      "swytch.DeleteRequest",
				RequestValidator: "ValidateDeleteRequest",
			},
			{
				Name:         "Version",
				CommandName:  "version",
				CommandType:  "switchVersionCommand",
				Help:         "Print the API version.",
				Method:       "VersionWithContext",
				ResponseType: "string",
			},
		},
	}

	source, err := Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v", err)
	}

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
		"switchFindCommand",
		`name:\"find\"`,
		"commands.Find.runtime = runtime",
		"commands.Find.factory = factory",
		"op.SearchWithContext(context.Background(), request)",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	if strings.Contains(string(source), "type switchFindCommand struct") {
		t.Fatal("handwritten operation command was generated")
	}
	if strings.Contains(string(source), "ValidateDeleteRequest func(") ||
		strings.Contains(string(source), "c.runtime.ValidateDeleteRequest") {
		t.Fatal("request validator is configured through Runtime instead of called directly")
	}
	for _, removed := range []string{"OutputType", "tableZones", "format,", "SetZoneFactory", "zoneFactory", "zones."} {
		if strings.Contains(string(source), removed) {
			t.Errorf("generated source contains removed output machinery %q", removed)
		}
	}

	second, err := Generate(config)
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if string(source) != string(second) {
		t.Fatal("Generate() output is not deterministic")
	}
}

func TestValidateOperationCommandNames(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		CommandType: "Commands", APIType: "API", FactoryType: "APIFactory", RuntimeType: "Runtime",
		Operations: []Operation{
			{Name: "ReadIPv6", CommandName: "read-ipv6", CommandType: "ReadIPv6Command", Help: "IPv6 を読み取ります。", Method: "ReadIPv6"},
			{Name: "EnableIPv6", CommandName: "enable-ipv6", CommandType: "EnableIPv6Command", Help: "IPv6 を有効にします。", Method: "EnableIPv6"},
		},
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name:\"read-ipv6\"`, `name:\"enable-ipv6\"`} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}

	config.Operations[1].CommandName = "read-ipv6"
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate command name") {
		t.Fatalf("Validate() error = %v, want duplicate command name", err)
	}
	config.Operations[1].CommandName = ""
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "every operation or none") {
		t.Fatalf("Validate() error = %v, want all operations to specify command_name", err)
	}
	config.Operations[1].CommandName = "Enable-IPv6"
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "command_name must be kebab-case") {
		t.Fatalf("Validate() error = %v, want kebab-case command name", err)
	}
}

func TestGenerateHandwrittenRequestValidator(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		CommandType: "Commands", APIType: "API", FactoryType: "APIFactory", RuntimeType: "Runtime",
		Operations: []Operation{{
			Name: "Boot", CommandType: "BootCommand", Help: "起動します。",
			Method: "Boot", RequestType: "BootRequest", RequestValidator: "ValidateBootRequest", Handwritten: true,
		}},
	}
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"Boot BootCommand",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	if strings.Contains(text, "type BootCommand struct") {
		t.Fatal("handwritten operation command was generated")
	}
}

func TestGenerateFileOnlyRequestInput(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		CommandType: "Commands", APIType: "API", FactoryType: "APIFactory", RuntimeType: "Runtime",
		Operations: []Operation{{
			Name: "Create", CommandType: "CreateCommand", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", RequestFileOnly: true,
		}},
	}
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`if *c.Request != "-" && (len(*c.Request) == 0 || (*c.Request)[0] != '@')`,
		"--request には @path.json または -（標準入力）を指定してください",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	config.Operations[0].RequestType = ""
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "request_file_only requires request_type") {
		t.Fatalf("Validate() error = %v, want request_file_only request_type error", err)
	}
}

func TestGenerateZoneValidation(t *testing.T) {
	config := Config{
		Package: "exampleapi", Resource: "Example",
		CommandType: "Commands", APIType: "API", FactoryType: "APIFactory", RuntimeType: "Runtime",
		Operations: []Operation{{
			Name: "Create", CommandType: "CreateCommand", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", ValidateZone: true,
			RequestValidator: "ValidateCreateRequest", RequestTransform: "TransformCreateRequest",
			Flags: []Flag{{Name: "zone", Field: "Zone", Type: "string", Help: "対象ゾーンです。"}},
		}},
	}
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `zones "github.com/sacloud/skr/internal/iaas/zones"`) {
		t.Fatal("shared zone validator import is missing")
	}
	zoneValidation := strings.Index(text, "if err := zones.ValidateSingleZone(request.Zone); err != nil")
	transform := strings.Index(text, "if err := c.runtime.TransformCreateRequest(c, request); err != nil")
	validator := strings.Index(text, "if err := ValidateCreateRequest(request); err != nil")
	factory := strings.Index(text, "op, err := c.factory(")
	if zoneValidation < 0 || transform < 0 || validator < 0 || factory < 0 ||
		!(transform < zoneValidation && zoneValidation < validator && validator < factory) {
		t.Fatal("zone validation must follow request construction and precede domain validation and API setup")
	}
	second, err := Generate(config)
	if err != nil || string(second) != text {
		t.Fatalf("second Generate() is not deterministic: %v", err)
	}
	config.Operations[0].ValidateZone = false
	source, err = Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "zones") {
		t.Fatal("disabled zone validation added an import or validation call")
	}
}

func TestValidateZoneSetting(t *testing.T) {
	base := `{"package":"exampleapi","resource":"Example","command_type":"Commands","api_type":"API","factory_type":"APIFactory","runtime_type":"Runtime","operations":[{"name":"Find","command_type":"FindCommand","help":"検索します。","method":"Find","request_type":"FindRequest","validate_zone":true}]}`
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
		{name: "type collision", change: func(c *Config) { c.Operations[0].CommandType = "zones" }, want: "conflicts with an import alias"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := DecodeConfig([]byte(base))
			if err != nil {
				t.Fatal(err)
			}
			if test.change != nil {
				test.change(&config)
			}
			err = config.Validate()
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGenerateOptionalRequest(t *testing.T) {
	config := Config{
		Package:     "exampleapi",
		Resource:    "Example API",
		CommandType: "Commands",
		APIType:     "API",
		FactoryType: "APIFactory",
		RuntimeType: "Runtime",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		Operations: []Operation{
			{
				Name:            "List",
				CommandType:     "ListCommand",
				Help:            "List resources.",
				Method:          "List",
				RequestType:     "example.ListParams",
				RequestByValue:  true,
				RequestOptional: true,
				ResponseType:    "[]example.Resource",
			},
		},
	}

	source, err := Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(string(source), "result, err := op.List(context.Background(), *request)") {
		t.Fatalf("generated source does not invoke List with the zero-value request:\n%s", source)
	}
	if strings.Contains(string(source), "--request または個別フラグが必要です") {
		t.Fatalf("generated source requires an optional request:\n%s", source)
	}
}

func TestGenerateValueRequestAndPositionalArguments(t *testing.T) {
	config := Config{
		Package:     "exampleapi",
		Resource:    "Example API",
		CommandType: "Commands",
		APIType:     "API",
		FactoryType: "APIFactory",
		RuntimeType: "Runtime",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		Operations: []Operation{
			{
				Name:           "Update",
				CommandType:    "UpdateCommand",
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

	source, err := Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v", err)
	}
	for _, want := range []string{
		"Update(context.Context, string, example.UpdateRequest) (*example.Item, error)",
		`arg:\"\" name:\"id\" help:\"更新対象の ID。\"`,
		"op.Update(context.Background(), c.ID, *request)",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
}

func TestGenerateHandwrittenOnlyOperationsOmitFmtImport(t *testing.T) {
	config := Config{
		Package:     "zoneapi",
		Resource:    "IaaS Zone",
		CommandType: "Commands",
		APIType:     "API",
		FactoryType: "APIFactory",
		RuntimeType: "Runtime",
		Imports: map[string]string{
			"iaas": "github.com/sacloud/sacloud-sdk-go/api/iaas",
			"zone": "github.com/sacloud/sacloud-sdk-go/service/iaas/zone",
		},
		Operations: []Operation{{
			Name:         "Find",
			CommandType:  "FindCommand",
			Help:         "ゾーン一覧を取得します。",
			Method:       "FindWithContext",
			RequestType:  "zone.FindRequest",
			ResponseType: "[]*iaas.Zone",
			Handwritten:  true,
		}},
	}

	source, err := Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v", err)
	}
	if strings.Contains(string(source), `"fmt"`) {
		t.Fatal("generated source imports fmt without generated handlers")
	}
}

func TestGenerateFactoryArgumentsAndNamedMethodArguments(t *testing.T) {
	config := Config{
		Package:     "exampleapi",
		Resource:    "Example API",
		CommandType: "Commands",
		APIType:     "API",
		FactoryType: "APIFactory",
		RuntimeType: "Runtime",
		Imports: map[string]string{
			"example": "github.com/example/sdk/api/example",
		},
		FactoryArguments: []Argument{
			{Name: "queue-name", Field: "QueueName", Type: "string", Help: "対象キュー名。", Required: true},
			{Name: "api-key-file", Field: "APIKeyFile", Type: "string", Help: "API キーファイル。", Required: true},
		},
		Operations: []Operation{{
			Name:         "Send",
			CommandType:  "SendCommand",
			Help:         "送信します。",
			Method:       "Send",
			ResponseType: "*example.Item",
			Arguments: []Argument{
				{Name: "content", Field: "Content", Type: "string", Help: "本文。", Flag: true, Required: true},
			},
		}},
	}

	source, err := Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v", err)
	}
	for _, want := range []string{
		"type APIFactory func(QueueName string, APIKeyFile string) (API, error)",
		"func NewCommands(runtime Runtime, factory APIFactory) Commands",
		`name:\"queue-name\" help:\"対象キュー名。\" required:\"\"`,
		`name:\"content\" help:\"本文。\" required:\"\"`,
		"op.Send(context.Background(), c.Content)",
		"c.factory(c.QueueName, c.APIKeyFile)",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	if strings.Contains(string(source), ".SetRuntime(") || strings.Contains(string(source), ".SetFactory(") {
		t.Fatal("generated source contains dependency setters")
	}
}

func TestDecodeConfigRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, input := range []string{
		`{"package":"main","unexpected":true}`,
		`{"package":"main"} {}`,
		`{"operations":[{"zone_search":{"flag_field":"Zone","request_field":"Zone"}}]}`,
	} {
		if _, err := DecodeConfig([]byte(input)); err == nil {
			t.Errorf("DecodeConfig(%q) succeeded; want error", input)
		}
	}
}

func TestValidateRequestByValueRequiresRequestType(t *testing.T) {
	config := Config{
		Package:     "exampleapi",
		Resource:    "Example API",
		CommandType: "Commands",
		APIType:     "API",
		FactoryType: "APIFactory",
		RuntimeType: "Runtime",
		Operations: []Operation{{
			Name:           "List",
			CommandType:    "ListCommand",
			Help:           "一覧を表示します。",
			Method:         "List",
			RequestByValue: true,
		}},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "request_by_value requires request_type") {
		t.Fatalf("Validate() error = %v, want request_by_value error", err)
	}
}

func TestValidateRejectsUnsupportedFlagsAndDuplicateNames(t *testing.T) {
	config := Config{
		Package:     "main",
		Resource:    "switch",
		CommandType: "switchCommands",
		APIType:     "switchAPI",
		FactoryType: "switchAPIFactory",
		RuntimeType: "switchAPIRuntime",
		Operations: []Operation{
			{
				Name:        "Find",
				CommandType: "switchFindCommand",
				Help:        "Find.",
				Method:      "FindWithContext",
				RequestType: "FindRequest",
				Flags:       []Flag{{Name: "zone", Field: "Zone", Type: "[]int", Help: "Zone."}},
			},
		},
	}

	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Validate() error = %v, want unsupported flag type error", err)
	}

	config.Operations[0].Flags[0].Type = "string"
	config.Operations[0].Flags[0].Separator = ","
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "separator requires []string type") {
		t.Fatalf("Validate() error = %v, want separator type error", err)
	}
	config.Operations[0].Flags[0].Separator = ""
	// Tags 以外の配列フラグを追加する場合は、カンマを要素値に使えない根拠を確認して ADR に記録するか、同一フラグの複数指定方式を検討します。
	config.Operations[0].Flags[0] = Flag{
		Name:      "names",
		Field:     "Names",
		Type:      "[]string",
		Help:      "Names.",
		Separator: ",",
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "supported only for the tags flag") {
		t.Fatalf("Validate() error = %v, want unsupported array flag error", err)
	}
	config.Operations[0].Flags[0] = Flag{
		Name:      "tags",
		Field:     "Tags",
		Type:      "[]string",
		Help:      "Tags.",
		Separator: ";",
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "supported only for the tags flag") {
		t.Fatalf("Validate() error = %v, want unsupported tags separator error", err)
	}
	config.Operations[0].Flags[0] = Flag{
		Name:      "tags",
		Field:     "Tags",
		Type:      "[]string",
		Help:      "Tags.",
		Separator: ",",
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want valid tags array flag", err)
	}
	config.Operations[0].Flags[0] = Flag{Name: "zone", Field: "Zone", Type: "string", Help: "Zone."}
	config.Operations = append(config.Operations, config.Operations[0])
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate operation") {
		t.Fatalf("Validate() error = %v, want duplicate operation error", err)
	}
}

func TestSingletonNameFlag(t *testing.T) {
	config := transformTestConfig()
	config.Operations[0].Name = "Find"
	config.Operations[0].Flags = []Flag{{Name: "name", Field: "Names", Type: "string", Help: "名前です。", Singleton: true}}
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "request.Names = []string{*c.Names}") {
		t.Fatalf("singleton assignment missing: %s", source)
	}
	second, err := Generate(config)
	if err != nil || string(source) != string(second) {
		t.Fatalf("non-deterministic generation: %v", err)
	}
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
		if err := config.Validate(); err == nil {
			t.Fatalf("accepted invalid singleton flag: %+v", flag)
		}
	}
	config.Operations[0].Flags[0] = valid
	config.Operations[0].Name = "Create"
	if err := config.Validate(); err == nil {
		t.Fatal("accepted singleton outside Find")
	}
}

func transformTestConfig() Config {
	return Config{
		Package: "exampleapi", Resource: "Example",
		CommandType: "Commands", APIType: "API", FactoryType: "APIFactory", RuntimeType: "Runtime",
		Operations: []Operation{{
			Name: "Create", CommandType: "CreateCommand", Help: "作成します。",
			Method: "Create", RequestType: "CreateRequest", RequestTransform: "TransformCreateRequest",
			RequestValidator: "ValidateCreateRequest",
			Flags:            []Flag{{Name: "name", Field: "Name", Type: "string", Required: true, Help: "名前です。"}},
			RepeatedFlags:    []RepeatedFlag{{Name: "item", Field: "Items", Help: "繰り返し指定します。"}},
		}},
	}
}

func TestGenerateRequestTransform(t *testing.T) {
	config := transformTestConfig()
	source, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
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
		if !strings.Contains(text, want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	assign := strings.Index(text, "request.Name = *c.Name")
	transform := strings.Index(text, "c.runtime.TransformCreateRequest(c, request)")
	validate := strings.Index(text, "if err := ValidateCreateRequest(request); err != nil")
	if assign < 0 || transform <= assign || validate <= transform {
		t.Fatal("expected scalar assignment, transform, then validation")
	}
	if strings.Contains(text, "request.Items =") {
		t.Fatal("repeated input must not be assigned directly to the API request")
	}
	second, err := Generate(config)
	if err != nil || string(second) != text {
		t.Fatalf("generation is not deterministic: %v", err)
	}

	config.Operations[0].Flags = nil
	source, err = Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), `return fmt.Errorf("--request が必要です")`) {
		t.Fatal("repeated flags alone must provide the flag input path")
	}
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
		{"constructor conflicts with top-level type", func(c *Config) {
			c.APIType = "NewCommands"
		}, "constructor \"NewCommands\" conflicts with a generated type name"},
		{"constructor conflicts with operation type", func(c *Config) {
			c.Operations[0].CommandType = "NewCommands"
		}, "constructor \"NewCommands\" conflicts with a generated type name"},
		{"constructor conflicts with request validator", func(c *Config) {
			c.Operations[0].RequestValidator = "NewCommands"
		}, "constructor \"NewCommands\" conflicts with a request_validator"},
		{"constructor conflicts with import alias", func(c *Config) {
			c.Imports = map[string]string{"NewCommands": "example/sdk"}
		}, "constructor \"NewCommands\" conflicts with an import alias"},
		{"validator reused for different request types", func(c *Config) {
			op := c.Operations[0]
			op.Name, op.CommandType, op.Method, op.RequestType, op.RequestTransform, op.RepeatedFlags =
				"Update", "UpdateCommand", "Update", "UpdateRequest", "", nil
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
			op.Name, op.CommandType, op.Method = "Update", "UpdateCommand", "Update"
			c.Operations = append(c.Operations, op)
		}, "duplicate runtime field"},
		{"runtime type collision", func(c *Config) { c.Operations[0].CommandType = c.RuntimeType }, "duplicate generated type"},
		{"top level type collision", func(c *Config) { c.RuntimeType = c.APIType }, "duplicate generated type"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := transformTestConfig()
			test.change(&config)
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}
