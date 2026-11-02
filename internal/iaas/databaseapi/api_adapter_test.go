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

package databaseapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/stretchr/testify/require"
)

func TestFindWithContextBuildsCondition(t *testing.T) {
	op := &fakeDatabaseOp{findResult: &iaas.DatabaseFindResult{Databases: []*iaas.Database{{ID: 1}, {ID: 2}}}}
	api := &databaseAPI{op: op}

	found, err := api.FindWithContext(context.Background(), &FindRequest{
		Zone:  "is1c",
		Names: []string{"db-01"},
		Tags:  []string{"prod", "managed"},
		Count: 10,
		From:  5,
		Sort:  search.SortKeys{search.SortKeyAsc("Name")},
	})

	require.NoError(t, err)
	require.Len(t, found, 2)
	require.Equal(t, "is1c", op.findZone)
	condition := op.findCondition
	require.NotNil(t, condition)
	// 単一名の部分一致検索は Names の1要素に変換します
	require.Equal(t, search.AndEqual("db-01"), condition.Filter[search.Key("Name")])
	require.Equal(t, search.TagsAndEqual("prod", "managed"), condition.Filter[search.Key("Tags.Name")])
	require.Equal(t, 10, condition.Count)
	require.Equal(t, 5, condition.From)
	require.Equal(t, search.SortKeys{search.SortKeyAsc("Name")}, condition.Sort)
}

func TestFindWithContextOmittedConditions(t *testing.T) {
	// Names と Tags を省略したとき Filter に要素が入らないこと
	op := &fakeDatabaseOp{findResult: &iaas.DatabaseFindResult{}}
	api := &databaseAPI{op: op}

	_, err := api.FindWithContext(context.Background(), &FindRequest{Zone: "is1c"})

	require.NoError(t, err)
	condition := op.findCondition
	require.NotNil(t, condition)
	require.Empty(t, condition.Filter)
	require.Nil(t, condition.Sort)
	require.Zero(t, condition.Count)
	require.Zero(t, condition.From)
}

func TestFindWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeDatabaseOp{findErr: errors.New("find failed")}
	api := &databaseAPI{op: op}

	_, err := api.FindWithContext(context.Background(), &FindRequest{Zone: "is1c"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "find failed")
}

func TestMonitorCPUWithContextReturnsValues(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	op := &fakeDatabaseOp{cpuValues: &iaas.CPUTimeActivity{Values: []*iaas.MonitorCPUTimeValue{{CPUTime: 10}}}}
	api := &databaseAPI{op: op}

	values, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1c", ID: 123, Start: now, End: now.Add(time.Hour)})

	require.NoError(t, err)
	require.Len(t, values, 1)
	// 指定した取得期間がそのまま API の条件に渡ること
	require.Equal(t, now, op.monitorCondition.Start)
	require.Equal(t, now.Add(time.Hour), op.monitorCondition.End)
}

func TestMonitorCPUWithContextComplementsPeriod(t *testing.T) {
	// 期間の未指定は現在時刻と 1 時間前に補完されます
	op := &fakeDatabaseOp{cpuValues: &iaas.CPUTimeActivity{}}
	api := &databaseAPI{op: op}

	before := time.Now().Truncate(time.Second)
	_, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1c", ID: 123})
	after := time.Now().Truncate(time.Second)

	require.NoError(t, err)
	require.False(t, op.monitorCondition.Start.IsZero())
	require.False(t, op.monitorCondition.End.IsZero())
	require.GreaterOrEqual(t, op.monitorCondition.End.Unix(), before.Unix())
	require.LessOrEqual(t, op.monitorCondition.End.Unix(), after.Unix())
	require.Equal(t, -1*time.Hour, op.monitorCondition.Start.Sub(op.monitorCondition.End))
}

func TestMonitorCPUWithContextRejectsInvalidPeriod(t *testing.T) {
	// 開始が終了より後の場合はエラーになります
	op := &fakeDatabaseOp{cpuValues: &iaas.CPUTimeActivity{}}
	api := &databaseAPI{op: op}

	now := time.Now()
	_, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1c", ID: 123, Start: now.Add(time.Hour), End: now})

	require.Error(t, err)
	require.Contains(t, err.Error(), "is invalid")
}

func TestMonitorCPUWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeDatabaseOp{cpuErr: errors.New("monitor failed")}
	api := &databaseAPI{op: op}

	_, err := api.MonitorCPUWithContext(context.Background(), &MonitorCPURequest{Zone: "is1c", ID: 123})

	require.Error(t, err)
	require.Contains(t, err.Error(), "monitor failed")
}

func TestMonitorDatabaseWithContextReturnsValues(t *testing.T) {
	op := &fakeDatabaseOp{databaseValues: &iaas.DatabaseActivity{Values: []*iaas.MonitorDatabaseValue{{BinlogUsedSizeKiB: 10}}}}
	api := &databaseAPI{op: op}

	values, err := api.MonitorDatabaseWithContext(context.Background(), &MonitorDatabaseRequest{Zone: "is1c", ID: 123})

	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, "monitor-database", op.operations[len(op.operations)-1])
}

func TestMonitorDatabaseWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeDatabaseOp{databaseErr: errors.New("monitor failed")}
	api := &databaseAPI{op: op}

	_, err := api.MonitorDatabaseWithContext(context.Background(), &MonitorDatabaseRequest{Zone: "is1c", ID: 123})

	require.Error(t, err)
	require.Contains(t, err.Error(), "monitor failed")
}

func TestListParameterWithContextMapsSettings(t *testing.T) {
	// パラメータのメタ情報と現在の設定値を対応付けて返します
	op := &fakeDatabaseOp{parameters: &iaas.DatabaseParameter{
		MetaInfo: []*iaas.DatabaseParameterMeta{
			{Name: "max_connections", Label: "最大接続数"},
			{Name: "slow_query_log", Label: "スロークエリログ"},
		},
		Settings: map[string]any{"max_connections": 100},
	}}
	api := &databaseAPI{op: op}

	parameters, err := api.ListParameterWithContext(context.Background(), &ListParameterRequest{Zone: "is1c", ID: 123})

	require.NoError(t, err)
	require.Len(t, parameters, 2)
	require.Equal(t, "最大接続数", parameters[0].Key)
	require.Equal(t, 100, parameters[0].Value)
	require.Equal(t, "max_connections", parameters[0].Meta.Name)
	// 設定値に対応する項目がない場合 Value は nil になります
	require.Equal(t, "スロークエリログ", parameters[1].Key)
	require.Nil(t, parameters[1].Value)
}

func TestListParameterWithContextPropagatesAPIErrors(t *testing.T) {
	op := &fakeDatabaseOp{parameterErr: errors.New("parameter failed")}
	api := &databaseAPI{op: op}

	_, err := api.ListParameterWithContext(context.Background(), &ListParameterRequest{Zone: "is1c", ID: 123})

	require.Error(t, err)
	require.Contains(t, err.Error(), "parameter failed")
}
