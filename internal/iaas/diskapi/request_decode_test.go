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

package diskapi

import (
	"encoding/json"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

func TestCreateRejectsUnsupportedJSONFieldsBeforeCallingAPI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		field string
	}{
		{name: "OSType", input: `{"Zone":"is1b","OSType":"ubuntu"}`, field: "OSType"},
		{name: "EditParameter", input: `{"Zone":"is1b","EditParameter":{}}`, field: "EditParameter"},
		{name: "NoWait", input: `{"Zone":"is1b","NoWait":true}`, field: "NoWait"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factoryCalls := 0
			command := &CreateCommand{
				Request: &tt.input,
				runtime: Runtime{
					DecodeRequest: func(input string, target any) error {
						return json.Unmarshal([]byte(input), target)
					},
				},
				factory: func() (API, error) {
					factoryCalls++
					return nil, nil
				},
			}

			err := command.Run(&kong.Context{})

			require.ErrorContains(t, err, tt.field)
			require.Zero(t, factoryCalls)
		})
	}
}

func TestUpdateRejectsUnsupportedJSONFieldsBeforeCallingAPI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		field string
	}{
		{name: "EditParameter", input: `{"Zone":"is1b","ID":123,"EditParameter":{}}`, field: "EditParameter"},
		{name: "NoWait", input: `{"Zone":"is1b","ID":123,"NoWait":true}`, field: "NoWait"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factoryCalls := 0
			command := &UpdateCommand{
				Request: &tt.input,
				runtime: Runtime{
					DecodeRequest: func(input string, target any) error {
						return json.Unmarshal([]byte(input), target)
					},
				},
				factory: func() (API, error) {
					factoryCalls++
					return nil, nil
				},
			}

			err := command.Run(&kong.Context{})

			require.ErrorContains(t, err, tt.field)
			require.Zero(t, factoryCalls)
		})
	}
}
