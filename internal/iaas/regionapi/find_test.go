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

package regionapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/region"
)

func TestFindCommandSupportsNoInputAndOptionalFlags(t *testing.T) {
	for _, test := range []struct {
		name  string
		count *int
		from  *int
		want  region.FindRequest
	}{
		{name: "no input", want: region.FindRequest{}},
		{name: "explicit zero count", count: intPointer(0), want: region.FindRequest{Count: 0}},
		{name: "optional flags", count: intPointer(10), from: intPointer(2), want: region.FindRequest{Count: 10, From: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &findAPITestDouble{}
			command := newFindCommand(api)
			command.Count = test.count
			command.From = test.from
			if err := command.Run(&kong.Context{}); err != nil {
				t.Fatal(err)
			}
			if api.request == nil || api.request.Count != test.want.Count || api.request.From != test.want.From {
				t.Fatalf("Find request = %+v, want %+v", api.request, test.want)
			}
		})
	}
}

func TestFindCommandDecodesJSONWithoutMergingAndRejectsFlagCombination(t *testing.T) {
	api := &findAPITestDouble{}
	command := newFindCommand(api)
	requestJSON := `{"Names":["region"],"Sort":[{"Key":"Name"}]}`
	command.Request = &requestJSON
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if api.request == nil || len(api.request.Names) != 1 || api.request.Names[0] != "region" ||
		len(api.request.Sort) != 1 || api.request.Sort[0].Key != "Name" {
		t.Fatalf("JSON Find request = %+v", api.request)
	}

	command = newFindCommand(api)
	command.Request = &requestJSON
	command.Count = intPointer(0)
	if err := command.Run(&kong.Context{}); err == nil {
		t.Fatal("expected --request and --count combination to fail")
	}
	if api.calls != 1 {
		t.Fatalf("API call count = %d, want 1", api.calls)
	}
}

func TestFindCommandReturnsDecodeAndOutputValidationErrors(t *testing.T) {
	decodeErr := errors.New("decode failure")
	command := newFindCommand(&findAPITestDouble{})
	command.runtime.DecodeRequest = func(string, any) error { return decodeErr }
	input := "invalid"
	command.Request = &input
	if err := command.Run(&kong.Context{}); !errors.Is(err, decodeErr) {
		t.Fatalf("decode error = %v, want %v", err, decodeErr)
	}

	validateErr := errors.New("query validation failure")
	command = newFindCommand(&findAPITestDouble{})
	command.runtime.ValidateOutput = func(*kong.Context) error { return validateErr }
	if err := command.Run(&kong.Context{}); !errors.Is(err, validateErr) {
		t.Fatalf("output validation error = %v, want %v", err, validateErr)
	}
}

func intPointer(value int) *int {
	return &value
}

func newFindCommand(api *findAPITestDouble) *FindCommand {
	return &FindCommand{
		factory: func() (API, error) { return api, nil },
		runtime: Runtime{
			DecodeRequest: func(input string, request any) error {
				return json.Unmarshal([]byte(input), request)
			},
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput:    func(*kong.Context, any) error { return nil },
		},
	}
}

type findAPITestDouble struct {
	request *region.FindRequest
	calls   int
}

func (api *findAPITestDouble) FindWithContext(_ context.Context, request *region.FindRequest) ([]*iaas.Region, error) {
	api.calls++
	api.request = request
	return nil, nil
}

func (*findAPITestDouble) ReadWithContext(context.Context, *region.ReadRequest) (*iaas.Region, error) {
	return nil, nil
}
