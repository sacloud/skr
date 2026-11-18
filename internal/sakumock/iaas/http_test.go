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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPBridgeRoundTrip(t *testing.T) {
	server := NewTestServer(Config{Zones: []string{"is1a"}})
	httpServer := NewTestHTTPServer(server)
	t.Cleanup(httpServer.Close)

	caller, err := NewRemoteCaller(httpServer.URL)
	require.NoError(t, err)
	ctx := context.Background()

	var created struct {
		Switch struct {
			ID          json.Number `json:"ID"`
			Name        string      `json:"Name"`
			Description string      `json:"Description"`
		} `json:"Switch"`
	}
	data, err := caller.Do(ctx, "POST", "/api/cloud/1.1/is1a/switch", map[string]any{
		"Switch": map[string]any{"Name": "eventbus-switch-x1y2", "Description": "bridge test"},
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &created))
	require.Equal(t, "eventbus-switch-x1y2", created.Switch.Name)
	require.NotEmpty(t, created.Switch.ID.String())

	// 別の caller からも同じ状態を参照できることを確認します。skr は
	// コマンドごとにプロセスを起動するため、状態はサーバー側で共有されます。
	other, err := NewRemoteCaller(httpServer.URL)
	require.NoError(t, err)
	data, err = other.Do(ctx, "GET", "/api/cloud/1.1/is1a/switch/"+created.Switch.ID.String(), nil)
	require.NoError(t, err)
	var read struct {
		Switch struct {
			ID          json.Number `json:"ID"`
			Description string      `json:"Description"`
		} `json:"Switch"`
	}
	require.NoError(t, json.Unmarshal(data, &read))
	require.Equal(t, created.Switch.ID.String(), read.Switch.ID.String())
	require.Equal(t, "bridge test", read.Switch.Description)

	// 存在しないリソースはエラーとして伝わります。
	_, err = caller.Do(ctx, "GET", "/api/cloud/1.1/is1a/switch/999999", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// 削除は本文を持たない 200 で応答します。
	data, err = caller.Do(ctx, "DELETE", "/api/cloud/1.1/is1a/switch/"+created.Switch.ID.String(), nil)
	require.NoError(t, err)
	require.Empty(t, data)

	_, err = caller.Do(ctx, "GET", "/api/cloud/1.1/is1a/switch/"+created.Switch.ID.String(), nil)
	require.Error(t, err)
}

func TestRemoteCallerValidation(t *testing.T) {
	_, err := NewRemoteCaller("ftp://example.invalid")
	require.Error(t, err)

	caller, err := NewRemoteCaller("http://127.0.0.1:1/")
	require.NoError(t, err)
	_, err = caller.Do(context.Background(), "GET", "/api/cloud/1.1/is1a/switch", nil)
	require.Error(t, err)
}
