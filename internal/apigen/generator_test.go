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
				CommandType:  "switchFindCommand",
				Help:         "Find `switches`.",
				Method:       "FindWithContext",
				RequestType:  "swytch.FindRequest",
				ResponseType: "[]string",
				Handwritten:  true,
			},
			{
				Name:         "Search",
				CommandType:  "switchSearchCommand",
				Help:         "Search with generated flags.",
				Method:       "SearchWithContext",
				RequestType:  "swytch.FindRequest",
				ResponseType: "[]string",
				Flags: []Flag{
					{Name: "zone", Field: "Zone", Type: "string", Help: "Target `zone`.", Required: true},
					{Name: "count", Field: "Count", Type: "int", Help: "Maximum results.", Pointer: true},
				},
			},
			{
				Name:             "Delete",
				CommandType:      "switchDeleteCommand",
				Help:             "Delete a switch.",
				Method:           "DeleteWithContext",
				RequestType:      "swytch.DeleteRequest",
				RequestValidator: "validateSwitchDeleteRequest",
			},
			{
				Name:         "Version",
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
		"c.runtime.WriteOutput(ctx, result)",
		"if err := c.runtime.ValidateOutput(ctx); err != nil",
		"return op.DeleteWithContext(context.Background(), request)",
		"ValidateRequest(\"validateSwitchDeleteRequest\", request)",
		"switchFindCommand",
		"c.Find.runtime = runtime",
		"op.SearchWithContext(context.Background(), request)",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	if strings.Contains(string(source), "type switchFindCommand struct") {
		t.Fatal("handwritten operation command was generated")
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
		`name:\"queue-name\" help:\"対象キュー名。\" required:\"\"`,
		`name:\"content\" help:\"本文。\" required:\"\"`,
		"op.Send(context.Background(), c.Content)",
		"c.factory(c.QueueName, c.APIKeyFile)",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source does not contain %q", want)
		}
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
				Flags:       []Flag{{Name: "zone", Field: "Zone", Type: "[]string", Help: "Zone."}},
			},
		},
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Validate() error = %v, want unsupported flag type error", err)
	}

	config.Operations[0].Flags[0].Type = "string"
	config.Operations = append(config.Operations, config.Operations[0])
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate operation") {
		t.Fatalf("Validate() error = %v, want duplicate operation error", err)
	}
}
