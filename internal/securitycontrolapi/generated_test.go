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

package securitycontrolapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestSecurityControlGeneratedCodeMatchesConfigs(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{"api/commands/security-control-activation.json", "internal/securitycontrolapi/security_control_activation_api_generated.go"},
		{"api/commands/security-control-automated-action.json", "internal/securitycontrolapi/security_control_automated_action_api_generated.go"},
		{"api/commands/security-control-evaluation-rule.json", "internal/securitycontrolapi/security_control_evaluation_rule_api_generated.go"},
	} {
		configData, err := os.ReadFile(filepath.Join("..", "..", test.config))
		require.NoError(t, err)
		config, err := apigen.DecodeConfig(configData, apigen.ModeOgen)
		require.NoError(t, err, "decode %s", test.config)
		want, err := apigen.Generate(config, apigen.ModeOgen)
		require.NoError(t, err, "generate %s", test.config)
		got, err := os.ReadFile(filepath.Join("..", "..", test.output))
		require.NoError(t, err)
		require.Equal(t, want, got, "generated SecurityControl API command is stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
	}
}
