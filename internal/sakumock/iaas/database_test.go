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

package iaas

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// mockDatabaseNaked はモックに登録する Database の naked 形式の内容です。
func mockDatabaseNaked() map[string]any {
	return map[string]any{
		"ID":           112700000001,
		"Name":         "db-01",
		"Availability": "available",
		"Instance":     map[string]any{"Status": "down"},
		"Remark": map[string]any{
			"Switch":  map[string]any{"ID": 112900500001},
			"Plan":    map[string]any{"ID": 10},
			"Network": map[string]any{"DefaultRoute": "192.168.0.1", "NetworkMaskLen": 24},
			"Servers": []map[string]any{{"IPAddress": "192.168.0.11"}},
		},
	}
}

func TestDatabaseOperations(t *testing.T) {
	server := NewTestServer(Config{Zones: []string{"is1b"}})
	t.Cleanup(server.Close)
	require.NoError(t, server.AddDatabase("is1b", "112700000001", mockDatabaseNaked()))
	ctx := context.Background()
	op := sdk.NewDatabaseOp(server)

	t.Run("find returns registered databases", func(t *testing.T) {
		result, err := op.Find(ctx, "is1b", &sdk.FindCondition{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Total)
		require.Len(t, result.Databases, 1)
		require.Equal(t, "db-01", result.Databases[0].Name)
	})

	t.Run("read returns the registered database", func(t *testing.T) {
		db, err := op.Read(ctx, "is1b", types.ID(112700000001))
		require.NoError(t, err)
		require.NotNil(t, db)
		require.Equal(t, "db-01", db.Name)
		require.Equal(t, types.ID(112900500001), db.SwitchID)
	})

	t.Run("read propagates not found", func(t *testing.T) {
		_, err := op.Read(ctx, "is1b", types.ID(999))
		require.Error(t, err)
	})

	t.Run("power operations and reset are accepted", func(t *testing.T) {
		require.NoError(t, op.Boot(ctx, "is1b", types.ID(112700000001)))
		require.NoError(t, op.Shutdown(ctx, "is1b", types.ID(112700000001), nil))
		require.NoError(t, op.Reset(ctx, "is1b", types.ID(112700000001)))
	})

	t.Run("power operations reject missing databases", func(t *testing.T) {
		require.Error(t, op.Boot(ctx, "is1b", types.ID(999)))
		require.Error(t, op.Shutdown(ctx, "is1b", types.ID(999), nil))
		require.Error(t, op.Reset(ctx, "is1b", types.ID(999)))
	})

	t.Run("monitor-cpu returns values", func(t *testing.T) {
		activity, err := op.MonitorCPU(ctx, "is1b", types.ID(112700000001), &sdk.MonitorCondition{})
		require.NoError(t, err)
		require.NotNil(t, activity)
		require.Len(t, activity.Values, 1)
		require.Equal(t, float64(10), activity.Values[0].CPUTime)
	})

	t.Run("monitor-database returns values", func(t *testing.T) {
		activity, err := op.MonitorDatabase(ctx, "is1b", types.ID(112700000001), &sdk.MonitorCondition{})
		require.NoError(t, err)
		require.NotNil(t, activity)
		require.Len(t, activity.Values, 1)
		require.Equal(t, float64(10), activity.Values[0].BinlogUsedSizeKiB)
	})

	t.Run("get-parameter returns settings and meta info", func(t *testing.T) {
		parameter, err := op.GetParameter(ctx, "is1b", types.ID(112700000001))
		require.NoError(t, err)
		require.NotNil(t, parameter)
		require.EqualValues(t, 100, parameter.Settings["max_connections"])
		require.Len(t, parameter.MetaInfo, 1)
		require.Equal(t, "max_connections", parameter.MetaInfo[0].Name)
	})

	t.Run("delete removes the database", func(t *testing.T) {
		require.NoError(t, op.Delete(ctx, "is1b", types.ID(112700000001)))
		_, err := op.Read(ctx, "is1b", types.ID(112700000001))
		require.Error(t, err, "deleted database should not remain")
	})
}
