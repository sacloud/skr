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
	"net/http"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"

	iaasmock "github.com/sacloud/skr/internal/sakumock/iaas"
)

// TestDeleteWithContextHonorsFailIfNotFound は存在しない PacketFilter の削除が、モックの 404 を
// iaas.IsNotFoundError で分類できることを前提に FailIfNotFound に応じた挙動になることを確認します。
func TestDeleteWithContextHonorsFailIfNotFound(t *testing.T) {
	server := iaasmock.NewTestServer(iaasmock.Config{})
	t.Cleanup(server.Close)
	api := &sdkAPI{caller: server, op: iaas.NewPacketFilterOp(server)}
	ctx := context.Background()

	err := api.DeleteWithContext(ctx, &DeleteRequest{Zone: "is1b", ID: types.ID(9999), FailIfNotFound: true})
	require.Error(t, err)
	var apiErr iaas.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.ResponseCode(), "error response code")

	err = api.DeleteWithContext(ctx, &DeleteRequest{Zone: "is1b", ID: types.ID(9999), FailIfNotFound: false})
	require.NoError(t, err)
}
