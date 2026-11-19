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

package secretmanagerapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestSecretManagerGeneratedCodeMatchesConfigs(t *testing.T) {
	for _, test := range []struct {
		config string
		output string
	}{
		{"api/commands/secretmanager-secret.json", "internal/secretmanagerapi/secretmanager_secret_api_generated.go"},
		{"api/commands/secretmanager-vault.json", "internal/secretmanagerapi/secretmanager_vault_api_generated.go"},
	} {
		configData, err := os.ReadFile(filepath.Join("..", "..", test.config))
		require.NoError(t, err)
		config, err := apigen.DecodeConfig(configData, apigen.ModeOgen)
		require.NoError(t, err, "decode %s", test.config)
		want, err := apigen.Generate(config, apigen.ModeOgen)
		require.NoError(t, err, "generate %s", test.config)
		got, err := os.ReadFile(filepath.Join("..", "..", test.output))
		require.NoError(t, err)
		require.Equal(t, want, got, "generated SecretManager API command is stale; run make generate-api API_CONFIG=%s API_OUTPUT=%s", test.config, test.output)
	}
}
