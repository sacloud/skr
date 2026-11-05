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
	"github.com/stretchr/testify/require"
)

func TestFindCommandSupportsNoInputAndOptionalFlags(t *testing.T) {
	for _, test := range []struct {
		name  string
		count *int
		from  *int
		want  FindRequest
	}{
		{name: "no input", want: FindRequest{}},
		{name: "explicit zero count", count: intPointer(0), want: FindRequest{Count: 0}},
		{name: "optional flags", count: intPointer(10), from: intPointer(2), want: FindRequest{Count: 10, From: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &findAPITestDouble{}
			command := newFindCommand(api)
			command.Count = test.count
			command.From = test.from
			require.NoError(t, command.Run(&kong.Context{}))
			require.NotNil(t, api.request)
			require.Equal(t, test.want.Count, api.request.Count)
			require.Equal(t, test.want.From, api.request.From)
		})
	}
}

func TestFindCommandDecodesJSONWithoutMergingAndRejectsFlagCombination(t *testing.T) {
	api := &findAPITestDouble{}
	command := newFindCommand(api)
	requestJSON := `{"Names":["region"],"Sort":[{"Key":"Name"}]}`
	command.Request = &requestJSON
	require.NoError(t, command.Run(&kong.Context{}))
	require.NotNil(t, api.request)
	require.Equal(t, []string{"region"}, api.request.Names)
	require.Len(t, api.request.Sort, 1)
	require.Equal(t, "Name", api.request.Sort[0].Key)

	command = newFindCommand(api)
	command.Request = &requestJSON
	command.Count = intPointer(0)
	require.Error(t, command.Run(&kong.Context{}))
	require.Equal(t, 1, api.calls)
}

func TestFindCommandReturnsDecodeAndOutputValidationErrors(t *testing.T) {
	decodeErr := errors.New("decode failure")
	command := newFindCommand(&findAPITestDouble{})
	command.runtime.DecodeRequest = func(string, any) error { return decodeErr }
	input := "invalid"
	command.Request = &input
	require.ErrorIs(t, command.Run(&kong.Context{}), decodeErr)

	validateErr := errors.New("query validation failure")
	command = newFindCommand(&findAPITestDouble{})
	command.runtime.ValidateOutput = func(*kong.Context) error { return validateErr }
	require.ErrorIs(t, command.Run(&kong.Context{}), validateErr)
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
	request *FindRequest
	calls   int
}

func (api *findAPITestDouble) FindWithContext(_ context.Context, request *FindRequest) ([]*iaas.Region, error) {
	api.calls++
	api.request = request
	return nil, nil
}

func (*findAPITestDouble) ReadWithContext(context.Context, *ReadRequest) (*iaas.Region, error) {
	return nil, nil
}
