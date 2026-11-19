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
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/internal/apigen"
	"github.com/stretchr/testify/require"
)

func TestLicenseInfoGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(filepath.Join("..", "..", "..", "api/commands/iaas-product-license.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err, "decode api/commands/iaas-product-license.json")
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err, "generate api/commands/iaas-product-license.json")
	got, err := os.ReadFile(filepath.Join("..", "..", "..", "internal/iaas/licenseinfoapi/licenseinfo_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated LicenseInfo commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-product-license.json API_OUTPUT=internal/iaas/licenseinfoapi/licenseinfo_api_generated.go")
}
