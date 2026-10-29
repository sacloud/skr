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

package packetfilterapi

import (
	"os"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	"github.com/stretchr/testify/require"
)

func TestTransformCreateRequestDecodesExpressions(t *testing.T) {
	request := &packetfilter.CreateRequest{}
	command := &CreateCommand{Expression: []string{`[{"Protocol":"tcp","DestinationPort":"443","Action":"allow"}]`}}
	require.NoError(t, TransformCreateRequest(command, request))
	require.Len(t, request.Expression, 1)
}

func TestTransformUpdateRequestDecodesExpressions(t *testing.T) {
	request := &packetfilter.UpdateRequest{}
	command := &UpdateCommand{Expression: []string{`[{"Protocol":"udp","DestinationPort":"53","Action":"allow"}]`}}
	require.NoError(t, TransformUpdateRequest(command, request))
	require.NotNil(t, request.Expression)
	require.Len(t, *request.Expression, 1)
}

func TestDecodeExpressions(t *testing.T) {
	path := t.TempDir() + "/rules.json"
	fileContent := `[{"Protocol":"udp","DestinationPort":"53","Action":"allow"}]`
	err := os.WriteFile(path, []byte(fileContent), 0o600)
	require.NoError(t, err)
	for _, input := range []string{
		`[{"Protocol":"tcp","DestinationPort":"443","Action":"allow"}]`,
		"@" + path,
	} {
		expressions, err := decodeExpressions([]string{input})
		require.NoError(t, err, "decodeExpressions(%q)", input)
		require.Len(t, expressions, 1, "decoded %q to %d expressions, want 1", input, len(expressions))
	}
	for _, inputs := range [][]string{
		{"{"},
		{`{"Protocol":"tcp"}`},
		{"[]", "[]"},
	} {
		_, err := decodeExpressions(inputs)
		require.Error(t, err, "decodeExpressions(%q) succeeded, want error", inputs)
	}
}

func TestDecodeExpressionsRejectsNonArray(t *testing.T) {
	_, err := decodeExpressions([]string{`{"Protocol":"tcp"}`})
	require.Error(t, err)
}
