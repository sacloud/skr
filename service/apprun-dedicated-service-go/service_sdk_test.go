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

package apprundedicated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	apprunsdk "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	apprundedicatedmock "github.com/sacloud/sakumock/apprundedicated"
	"github.com/stretchr/testify/require"
)

func TestServiceWithSDKAndSakumock(t *testing.T) {
	server := apprundedicatedmock.NewTestServer(apprundedicatedmock.Config{})
	t.Cleanup(server.Close)
	var client saclient.Client
	require.NoError(t, client.SetEnviron([]string{
		"SAKURA_PROFILE_DIR=" + t.TempDir(),
		"SAKURA_ACCESS_TOKEN=dummy-token",
		"SAKURA_ACCESS_TOKEN_SECRET=dummy-secret",
		"SAKURA_ENDPOINTS_APPRUN_DEDICATED=" + server.TestURL() + "/",
	}))
	var mu sync.Mutex
	var lastBody []byte
	require.NoError(t, client.SetWith(saclient.WithMiddleware(
		func(request *http.Request, pull func() (saclient.Middleware, bool)) (*http.Response, error) {
			// バージョン作成のリクエスト本文を記録します。
			if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/versions") {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				if err := request.Body.Close(); err != nil {
					return nil, err
				}
				request.Body = io.NopCloser(bytes.NewReader(body))
				mu.Lock()
				lastBody = body
				mu.Unlock()
			}
			next, ok := pull()
			if !ok {
				return nil, fmt.Errorf("HTTP middleware がありません")
			}
			return next(request, pull)
		},
	)))
	caller, err := apprunsdk.NewClient(&client)
	require.NoError(t, err)
	s := New(caller)
	ctx := t.Context()
	wire := func() map[string]json.RawMessage {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		var request map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(lastBody, &request))
		return request
	}

	// Service は既存クラスタを対象とするため、SDK の公開操作でクラスタを作成します。
	_, err = apprunsdk.NewClusterOp(caller).Create(ctx, cluster.CreateParams{
		Name:               "sdk-cluster",
		ServicePrincipalID: "sp-000000000",
		Ports: []v1.ClusterLoadBalancerPort{
			{Port: 443, Protocol: v1.ClusterLoadBalancerPortProtocolHTTPS},
		},
	})
	require.NoError(t, err)

	// 存在しないアプリケーションの deploy は、アプリケーションとバージョンを作成して有効化します。
	def := &ApplicationDefinition{
		Cluster:          "sdk-cluster",
		Name:             "sdk-app",
		CPU:              1000,
		Memory:           1024,
		ScalingMode:      v1.ScalingModeManual,
		FixedScale:       ptrInt32(1),
		Image:            "ghcr.io/example/app:v1",
		RegistryUsername: ptrString("user"),
		RegistryPassword: ptrString("p@ssw0rd-value"),
		Env:              []version.EnvironmentVariable{{Key: "API_TOKEN", Value: ptrString("token-value"), Secret: true}},
	}
	result, err := s.Deploy(ctx, def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.True(t, result.ApplicationCreated)
	require.Equal(t, v1.ApplicationVersionNumber(1), result.Version)

	// バージョン作成のリクエストは、パスワード値とアクション (new) を API に送ります。
	require.Equal(t, `"new"`, string(wire()["registryPasswordAction"]))
	require.Equal(t, `"p@ssw0rd-value"`, string(wire()["registryPassword"]))
	var envs []struct {
		Value *string `json:"value"`
	}
	require.NoError(t, json.Unmarshal(wire()["env"], &envs))
	require.Len(t, envs, 1)
	require.NotNil(t, envs[0].Value)
	require.Equal(t, "token-value", *envs[0].Value)

	// 2 回目の deploy は既存アプリケーションに新しいバージョンを作成します。
	// パスワードを指定しないため、アクションは keep で既存のパスワードを維持します。
	def.Image = "ghcr.io/example/app:v2"
	def.RegistryPassword = nil
	result, err = s.Deploy(ctx, def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.False(t, result.ApplicationCreated)
	require.Equal(t, v1.ApplicationVersionNumber(2), result.Version)
	require.Equal(t, `"keep"`, string(wire()["registryPasswordAction"]))
	// パスワードの値は送られません (明示的な null)。
	require.Equal(t, "null", string(wire()["registryPassword"]))

	// Read はアクティブバージョンの状態を返します。
	status, err := s.Status(ctx, def)
	require.NoError(t, err)
	require.Equal(t, int32(2), *status.Application.ActiveVersion)
	require.Equal(t, ptrInt32(1), status.Application.DesiredCount)
	require.NotNil(t, status.ActiveVersionDetail)
	require.Equal(t, "ghcr.io/example/app:v2", status.ActiveVersionDetail.Image)
	require.Equal(t, int64(1000), status.ActiveVersionDetail.CPU)

	// Diff はアクティブバージョンと定義の比較で、秘密情報を除いた上で一致します。
	def2, err := s.Init(ctx, "sdk-cluster", "sdk-app")
	require.NoError(t, err)
	diff, err := s.Diff(ctx, def2)
	require.NoError(t, err)
	require.Equal(t, diff.Local, diff.Remote)

	// Init はアクティブバージョンから定義を復元し、秘密情報は含めません。
	require.Equal(t, int64(1000), def2.CPU)
	require.Equal(t, int64(1024), def2.Memory)
	require.Equal(t, "ghcr.io/example/app:v2", def2.Image)
	require.Equal(t, ptrString("user"), def2.RegistryUsername)
	require.Nil(t, def2.RegistryPassword)

	// Rollback はアクティブバージョンより前のバージョンへ切り替えます。
	result2, err := s.Rollback(ctx, def2, &RollbackOptions{NoWait: true})
	require.NoError(t, err)
	require.Equal(t, v1.ApplicationVersionNumber(1), result2.Version)
	status, err = s.Status(ctx, def)
	require.NoError(t, err)
	require.Equal(t, int32(1), *status.Application.ActiveVersion)

	// Deactivate はアクティブバージョンを解除します。
	deactivated, err := s.Deactivate(ctx, def, &WaitOptions{NoWait: true})
	require.NoError(t, err)
	require.NotNil(t, deactivated.DeactivatedVersion)
	status, err = s.Status(ctx, def)
	require.NoError(t, err)
	require.Nil(t, status.Application.ActiveVersion)
	require.Nil(t, status.Application.DesiredCount)

	// Delete はアプリケーションを削除し、不在を確認します。
	deleted, err := s.Delete(ctx, def, &WaitOptions{NoWait: true})
	require.NoError(t, err)
	require.Equal(t, deactivated.ApplicationID, deleted.ApplicationID)
	_, err = s.Status(ctx, def)
	var notFound *ApplicationNotFoundError
	require.ErrorAs(t, err, &notFound)
}
