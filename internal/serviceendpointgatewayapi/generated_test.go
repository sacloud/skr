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

package serviceendpointgatewayapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestServiceEndpointGatewayGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(filepath.Join("..", "..", "api/commands/service-endpoint-gateway.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeOgen)
	require.NoError(t, err, "decode api/commands/service-endpoint-gateway.json")
	want, err := apigen.Generate(config, apigen.ModeOgen)
	require.NoError(t, err, "generate api/commands/service-endpoint-gateway.json")
	got, err := os.ReadFile(filepath.Join("..", "..", "internal/serviceendpointgatewayapi/service_endpoint_gateway_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated ServiceEndpointGateway API command is stale; run make generate-api API_CONFIG=api/commands/service-endpoint-gateway.json API_OUTPUT=internal/serviceendpointgatewayapi/service_endpoint_gateway_api_generated.go")
}
