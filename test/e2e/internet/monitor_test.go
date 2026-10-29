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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// monitorWindow は現在時刻を終端とする観測期間の長さです。
const monitorWindow = 10 * time.Minute

// monitorRouter は最近の期間のルータ＋スイッチのモニタ値を読み込み、
// 形式の整ったモニタデータが返ることを確認します。モニタリングは監視データの反映待ちを
// 必要としないため、作成直後でも実行できます。
func monitorRouter(t *testing.T, h *helper.Helper, id types.ID) {
	t.Helper()

	end := time.Now().UTC()
	request, err := json.Marshal(map[string]any{
		"Zone":  internetZone,
		"ID":    int64(id),
		"Start": end.Add(-monitorWindow).Format(time.RFC3339),
		"End":   end.Format(time.RFC3339),
	})
	require.NoError(t, err, "encode Router monitor request")
	output := h.Call(t, "ルータ＋スイッチのモニタリングを取得する",
		"iaas-api", "internet", "monitor-router", "--request", string(request))
	require.NoError(t, checkMonitorValues(output), "monitor-router should return well-formed monitor values")
}

// monitorValue はルータ＋スイッチのモニタ値の要素です。
type monitorValue struct {
	Time time.Time `json:"Time"`
	In   float64   `json:"In"`
	Out  float64   `json:"Out"`
}

// checkMonitorValues はモニタ値の応答が配列で、各要素が Time・In・Out を持つことを確認します。
func checkMonitorValues(output []byte) error {
	var values []monitorValue
	if err := json.Unmarshal(output, &values); err != nil {
		return fmt.Errorf("decode monitor values: %w", err)
	}
	if values == nil {
		return errors.New("expected a JSON array of monitor values")
	}
	for i, value := range values {
		if value.Time.IsZero() {
			return fmt.Errorf("monitor values[%d] has no Time", i)
		}
	}
	return nil
}

// TestCheckMonitorValues はモニタ値の応答の内容確認を確認します。
func TestCheckMonitorValues(t *testing.T) {
	require.NoError(t, checkMonitorValues([]byte(`[{"Time":"2026-01-01T00:00:00Z","In":0,"Out":0}]`)))
	require.NoError(t, checkMonitorValues([]byte(`[]`)))

	require.Error(t, checkMonitorValues([]byte(`[{"Time":"invalid","In":0,"Out":0}]`)),
		"an unparseable Time should be refused")
	require.Error(t, checkMonitorValues([]byte(`[{"In":0,"Out":0}]`)),
		"a missing Time should be refused")
	require.Error(t, checkMonitorValues([]byte(`{"Time":"2026-01-01T00:00:00Z"}`)),
		"a non-array response should be refused")
	require.Error(t, checkMonitorValues([]byte("{")),
		"invalid JSON should be refused")
}

// TestMonitorRouterRequestJSON はモニタリングのリクエスト JSON の内容を確認します。
func TestMonitorRouterRequestJSON(t *testing.T) {
	end := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	request, err := json.Marshal(map[string]any{
		"Zone":  internetZone,
		"ID":    int64(12),
		"Start": end.Add(-monitorWindow).Format(time.RFC3339),
		"End":   end.Format(time.RFC3339),
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(request, &decoded))
	require.Equal(t, internetZone, decoded["Zone"])
	require.Equal(t, float64(12), decoded["ID"])
	require.Equal(t, "2026-01-02T02:54:05Z", decoded["Start"])
	require.Equal(t, "2026-01-02T03:04:05Z", decoded["End"])
}
