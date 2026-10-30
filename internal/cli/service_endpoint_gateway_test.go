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

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	segmock "github.com/sacloud/sakumock/seg"
)

func TestRunServiceEndpointGatewayAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"service-endpoint-gateway-api", "--help"},
			want: []string{"appliance", "スイッチ", "閉域接続", "manual.sakura.ad.jp/cloud/network/switch/seg.html"},
		},
		{
			args: []string{"service-endpoint-gateway-api", "appliance", "--help"},
			want: []string{"list", "read", "create", "update", "apply", "delete", "read-interface", "read-power-status", "power-on", "shutdown", "reset"},
		},
		{
			args: []string{"service-endpoint-gateway-api", "appliance", "create", "--help"},
			want: []string{"--switch-id", "--network-mask-len", "--server-ip", "--request", "@path.json", "個別フラグと併用できません", "ルータ＋スイッチ"},
		},
		{
			args: []string{"service-endpoint-gateway-api", "appliance", "update", "--help"},
			want: []string{"--request", "全体置換", "EnabledServices", "apply"},
		},
		{
			args: []string{"service-endpoint-gateway-api", "appliance", "read-interface", "--help"},
			want: []string{"appliance-id", "interface-id"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				require.Contains(t, help, expected, "help does not contain %q:\n%s", expected, help)
			}
		})
	}
}

func TestRunServiceEndpointGatewayAPIRejectsCreateWithoutInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"service-endpoint-gateway-api", "appliance", "create"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request または個別フラグが必要です", "error = %q", stderr.String())
}

func TestRunServiceEndpointGatewayAPIRejectsMissingSwitchID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"service-endpoint-gateway-api", "appliance", "create", "--network-mask-len", "24"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--switch-id が必要です", "error = %q", stderr.String())
}

func TestRunServiceEndpointGatewayAPIRejectsMissingNetworkMaskLen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"service-endpoint-gateway-api", "appliance", "create", "--switch-id", "112900500001"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--network-mask-len が必要です", "error = %q", stderr.String())
}

func TestRunServiceEndpointGatewayAPIRejectsCreateWithRequestAndFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	request := `{"Appliance":{"Remark":{"Switch":{"ID":"112900500001"},"Network":{"NetworkMaskLen":24},"Servers":[]}}}`
	require.NotEqual(t, 0, run([]string{"service-endpoint-gateway-api", "appliance", "create", "--request", request, "--switch-id", "112900500001"}, &stdout, &stderr), "run should fail")
	require.Contains(t, stderr.String(), "--request と個別フラグは併用できません", "error = %q", stderr.String())
}

func TestServiceEndpointGatewayAPIWithSakumock(t *testing.T) {
	// ゾーンを設定すると SDK がエンドポイント上書きを無視するため、SAKURA_ZONE は未設定のままにします
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := segmock.NewTestServer(segmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_SERVICE_ENDPOINT_GATEWAY", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr = %q", args, stderr.String())
		return stdout.Bytes()
	}

	// 作成 (個別フラグ)
	// create の応答は API のレスポンスボディ (Appliance 配下) をそのまま出力します
	var created struct {
		Appliance struct {
			ID           string `json:"ID"`
			Class        string `json:"Class"`
			Availability string `json:"Availability"`
			Remark       struct {
				Switch struct {
					ID string `json:"ID"`
				} `json:"Switch"`
				Network struct {
					NetworkMaskLen int `json:"NetworkMaskLen"`
				} `json:"Network"`
				Servers []struct {
					IPAddress string `json:"IPAddress"`
				} `json:"Servers"`
			} `json:"Remark"`
		} `json:"Appliance"`
	}
	err := json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "create", "--switch-id", "112900500001", "--network-mask-len", "24", "--server-ip", "192.168.2.100"), &created)
	require.NoError(t, err)
	require.NotEmpty(t, created.Appliance.ID, "created appliance = %+v", created)
	require.Equal(t, "serviceendpointgateway", created.Appliance.Class, "unexpected value")
	require.Equal(t, "available", created.Appliance.Availability, "unexpected value")
	require.Equal(t, "112900500001", created.Appliance.Remark.Switch.ID, "unexpected value")
	require.Equal(t, 24, created.Appliance.Remark.Network.NetworkMaskLen, "unexpected value")
	require.Len(t, created.Appliance.Remark.Servers, 1, "created appliance = %+v", created)
	require.Equal(t, "192.168.2.100", created.Appliance.Remark.Servers[0].IPAddress, "unexpected value")
	applianceID := created.Appliance.ID

	// Read は作成内容を返します
	var read struct {
		Appliance struct {
			ID           string `json:"ID"`
			Availability string `json:"Availability"`
			Interfaces   []struct {
				Switch struct {
					ID    string `json:"ID"`
					Scope string `json:"Scope"`
				} `json:"Switch"`
			} `json:"Interfaces"`
		} `json:"Appliance"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read", applianceID), &read))
	require.Equal(t, applianceID, read.Appliance.ID, "unexpected value")
	require.NotEmpty(t, read.Appliance.Interfaces, "read appliance = %+v", read)

	// List に作成したゲートウェイが含まれること
	var listed struct {
		Appliances []struct {
			ID string `json:"ID"`
		} `json:"Appliances"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "list"), &listed))
	var found bool
	for _, appliance := range listed.Appliances {
		if appliance.ID == applianceID {
			found = true
		}
	}
	require.True(t, found, "appliance list = %+v, want created appliance %s", listed, applianceID)

	// read-interface (利用側スイッチのインターフェースを参照。応答は Interface 配下)
	var userInterface struct {
		Interface struct {
			Switch struct {
				Scope string `json:"Scope"`
			} `json:"Switch"`
		} `json:"Interface"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read-interface", applianceID, "112900500001"), &userInterface))
	require.Equal(t, "user", userInterface.Interface.Switch.Scope, "unexpected value")

	// 電源状態の取得 (応答は Instance 配下)
	var powerStatus struct {
		Instance struct {
			Status string `json:"Status"`
		} `json:"Instance"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read-power-status", applianceID), &powerStatus))
	require.Equal(t, "up", powerStatus.Instance.Status, "unexpected value")

	// 電源操作 (shutdown → power-on。shutdown の応答本文は出力しません)
	runCommand("service-endpoint-gateway-api", "appliance", "shutdown", applianceID)
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read-power-status", applianceID), &powerStatus))
	require.Equal(t, "down", powerStatus.Instance.Status, "unexpected value")
	var poweredOn struct {
		Success bool `json:"Success"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "power-on", applianceID), &poweredOn))
	require.True(t, poweredOn.Success, "power-on response = %+v", poweredOn)
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read-power-status", applianceID), &powerStatus))
	require.Equal(t, "up", powerStatus.Instance.Status, "unexpected value")

	// update は Settings を全体置換し、apply で反映します
	settingsJSON := `{"Appliance":{"Settings":{"ServiceEndpointGateway":{"EnabledServices":[{"Type":"ObjectStorage","Config":{"Endpoints":["s3.isk01.sakurastorage.jp"]}}],"MonitoringSuite":{"Enabled":"True"}}}}}`
	var updated struct {
		Appliance struct {
			ID       string `json:"ID"`
			Settings struct {
				ServiceEndpointGateway struct {
					EnabledServices []struct {
						Type string `json:"Type"`
					} `json:"EnabledServices"`
					MonitoringSuite struct {
						Enabled string `json:"Enabled"`
					} `json:"MonitoringSuite"`
				} `json:"ServiceEndpointGateway"`
			} `json:"Settings"`
		} `json:"Appliance"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "update", applianceID, "--request", settingsJSON), &updated))
	require.Equal(t, applianceID, updated.Appliance.ID, "unexpected value")
	require.Len(t, updated.Appliance.Settings.ServiceEndpointGateway.EnabledServices, 1, "updated appliance = %+v", updated)
	require.Equal(t, "ObjectStorage", updated.Appliance.Settings.ServiceEndpointGateway.EnabledServices[0].Type, "unexpected value")
	require.Equal(t, "True", updated.Appliance.Settings.ServiceEndpointGateway.MonitoringSuite.Enabled, "unexpected value")

	// apply (応答本文は出力しません。SettingsHash は read で確認できます)
	runCommand("service-endpoint-gateway-api", "appliance", "apply", applianceID)
	var applied struct {
		Appliance struct {
			ID           string  `json:"ID"`
			SettingsHash *string `json:"SettingsHash"`
		} `json:"Appliance"`
	}
	require.NoError(t, json.Unmarshal(runCommand("service-endpoint-gateway-api", "appliance", "read", applianceID), &applied))
	require.Equal(t, applianceID, applied.Appliance.ID, "unexpected value")
	require.NotNil(t, applied.Appliance.SettingsHash, "settings should be applied: %+v", applied)

	// 削除と不在確認
	runCommand("service-endpoint-gateway-api", "appliance", "delete", applianceID)
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"service-endpoint-gateway-api", "appliance", "read", applianceID}, &stdout, &stderr), "read after delete should fail")
}
