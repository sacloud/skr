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

package licenseinfoapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alecthomas/kong"
)

func TestFindCommandPassesOptionalPaginationFlags(t *testing.T) {
	for _, test := range []struct {
		name  string
		count *int
		from  *int
		want  FindRequest
	}{
		{name: "no input", want: FindRequest{}},
		{name: "nonzero pagination", count: intPointer(25), from: intPointer(10), want: FindRequest{Count: 25, From: 10}},
		{name: "explicit zero count", count: intPointer(0), want: FindRequest{Count: 0}},
		{name: "explicit zero from", from: intPointer(0), want: FindRequest{From: 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &findCommandAPITestDouble{}
			command := newFindCommandForTest(api)
			command.Count = test.count
			command.From = test.from
			if err := command.Run(&kong.Context{}); err != nil {
				t.Fatal(err)
			}
			if api.request == nil || api.request.Count != test.want.Count || api.request.From != test.want.From {
				t.Fatalf("Find request = %+v, want %+v", api.request, test.want)
			}
			if api.calls != 1 {
				t.Fatalf("API calls = %d, want 1", api.calls)
			}
		})
	}
}

func TestFindCommandRejectsRequestAndPaginationFlags(t *testing.T) {
	for _, test := range []struct {
		name  string
		count *int
		from  *int
	}{
		{name: "count", count: intPointer(0)},
		{name: "from", from: intPointer(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &findCommandAPITestDouble{}
			command := newFindCommandForTest(api)
			request := `{"Filter":{"Name":"license"}}`
			command.Request = &request
			command.Count = test.count
			command.From = test.from
			if err := command.Run(&kong.Context{}); err == nil {
				t.Fatal("expected --request and pagination flags to fail")
			}
			if api.calls != 0 {
				t.Fatalf("API calls = %d, want 0", api.calls)
			}
		})
	}
}

func TestFindCommandDoesNotCallAPIAfterDecodeOrOutputValidationFailure(t *testing.T) {
	decodeErr := errors.New("decode failure")
	validateErr := errors.New("output validation failure")
	for _, test := range []struct {
		name      string
		configure func(*FindCommand)
		wantErr   error
	}{
		{
			name: "decode failure",
			configure: func(command *FindCommand) {
				request := "invalid"
				command.Request = &request
				command.runtime.DecodeRequest = func(string, any) error { return decodeErr }
			},
			wantErr: decodeErr,
		},
		{
			name: "output validation failure",
			configure: func(command *FindCommand) {
				command.runtime.ValidateOutput = func(*kong.Context) error { return validateErr }
			},
			wantErr: validateErr,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &findCommandAPITestDouble{}
			command := newFindCommandForTest(api)
			test.configure(command)
			if err := command.Run(&kong.Context{}); !errors.Is(err, test.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, test.wantErr)
			}
			if api.calls != 0 {
				t.Fatalf("API calls = %d, want 0", api.calls)
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
}

func newFindCommandForTest(api *findCommandAPITestDouble) *FindCommand {
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

type findCommandAPITestDouble struct {
	request *FindRequest
	calls   int
}

func (api *findCommandAPITestDouble) FindWithContext(_ context.Context, request *FindRequest) (*FindResult, error) {
	api.calls++
	api.request = request
	return &FindResult{}, nil
}

var _ API = (*findCommandAPITestDouble)(nil)
