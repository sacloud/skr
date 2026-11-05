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
	"context"
	"encoding/json"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/stretchr/testify/require"
)

func TestFindPassesSortToAPI(t *testing.T) {
	op := &packetFilterAPIFake{}
	api := &sdkAPI{op: op}
	sort := search.SortKeys{search.SortKeyDesc("CreatedAt")}
	var request FindRequest
	require.NoError(t, json.Unmarshal([]byte(`{"Zone":"is1b","Sort":[{"Key":"CreatedAt","Order":1}],"Count":10,"From":20}`), &request))

	_, err := api.FindWithContext(context.Background(), &request)

	require.NoError(t, err)
	require.NotNil(t, op.condition)
	require.Equal(t, sort, op.condition.Sort)
	require.Equal(t, 10, op.condition.Count)
	require.Equal(t, 20, op.condition.From)
}

type packetFilterAPIFake struct {
	iaas.PacketFilterAPI
	condition *iaas.FindCondition
}

func (api *packetFilterAPIFake) Find(_ context.Context, _ string, condition *iaas.FindCondition) (*iaas.PacketFilterFindResult, error) {
	api.condition = condition
	return &iaas.PacketFilterFindResult{}, nil
}
