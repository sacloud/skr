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
	"context"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	apprunsdk "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	apprunmock "github.com/sacloud/sakumock/apprundedicated"
	"github.com/stretchr/testify/require"

	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

// kongContextForTest は、確認表示の書き込み先を差し替えた kong.Context を返します。
func kongContextForTest(t *testing.T, stdout, stderr *bytes.Buffer) *kong.Context {
	t.Helper()
	var commands Commands
	parser, err := kong.New(&commands, kong.Name("skr"), kong.Writers(stdout, stderr))
	require.NoError(t, err)
	ctx, err := parser.Parse([]string{"rollback", "--app", "any.json"})
	require.NoError(t, err)
	return ctx
}

// setupConfirmService は sakumock に対してアプリケーションを 1 つデプロイし、
// 確認処理のテストに使う Service と定義を返します。
func setupConfirmService(t *testing.T) (*apprundedicatedservice.Service, *apprundedicatedservice.ApplicationDefinition) {
	t.Helper()
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	var client saclient.Client
	require.NoError(t, client.SetEnviron([]string{
		"SAKURA_PROFILE_DIR=" + t.TempDir(),
		"SAKURA_ACCESS_TOKEN=dummy-token",
		"SAKURA_ACCESS_TOKEN_SECRET=dummy-secret",
		"SAKURA_ENDPOINTS_APPRUN_DEDICATED=" + server.TestURL() + "/",
	}))
	caller, err := apprunsdk.NewClient(&client)
	require.NoError(t, err)
	svc := apprundedicatedservice.New(caller)
	// Deploy は既存クラスタを対象とするため、SDK の公開操作でクラスタを作成します。
	_, err = apprunsdk.NewClusterOp(caller).Create(context.Background(), cluster.CreateParams{
		Name:               "confirm-cluster",
		ServicePrincipalID: "sp-000000000",
		Ports: []v1.ClusterLoadBalancerPort{
			{Port: 443, Protocol: v1.ClusterLoadBalancerPortProtocolHTTPS},
		},
	})
	require.NoError(t, err)
	def := &apprundedicatedservice.ApplicationDefinition{
		Cluster:     "confirm-cluster",
		Name:        "confirm-app",
		CPU:         1000,
		Memory:      1024,
		ScalingMode: v1.ScalingModeManual,
		FixedScale:  func() *int32 { v := int32(1); return &v }(),
		Image:       "ghcr.io/example/app:v1",
	}
	_, err = svc.Deploy(context.Background(), def, &apprundedicatedservice.DeployOptions{NoWait: true})
	require.NoError(t, err)
	return svc, def
}

func TestConfirmOperation(t *testing.T) {
	ctx := context.Background()

	t.Run("非端末の標準入力では --force が必要です", func(t *testing.T) {
		svc, def := setupConfirmService(t)
		var stdout, stderr bytes.Buffer
		runtime := Runtime{Stdin: strings.NewReader(""), IsTerminal: func() bool { return false }}
		err := confirmOperation(ctx, kongContextForTest(t, &stdout, &stderr), runtime, svc, def, false, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--force")
	})

	t.Run("標準入力から定義を読む場合も --force が必要です", func(t *testing.T) {
		svc, def := setupConfirmService(t)
		var stdout, stderr bytes.Buffer
		runtime := Runtime{Stdin: strings.NewReader("y\n"), IsTerminal: func() bool { return true }}
		err := confirmOperation(ctx, kongContextForTest(t, &stdout, &stderr), runtime, svc, def, false, true)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--force")
		// --force を指定した場合は確認せず続行します。
		require.NoError(t, confirmOperation(ctx, kongContextForTest(t, &stdout, &stderr), runtime, svc, def, true, true))
	})

	t.Run("n を入力すると中止します", func(t *testing.T) {
		svc, def := setupConfirmService(t)
		var stdout, stderr bytes.Buffer
		runtime := Runtime{Stdin: strings.NewReader("n\n"), IsTerminal: func() bool { return true }}
		err := confirmOperation(ctx, kongContextForTest(t, &stdout, &stderr), runtime, svc, def, false, false)
		require.ErrorIs(t, err, errAborted)
		// 確認前に対象の現在の状態を表示します。
		require.Contains(t, stderr.String(), "対象:")
		require.Contains(t, stderr.String(), "現在の状態:")
		require.Contains(t, stderr.String(), "続行しますか?")
	})

	t.Run("y を入力すると続行します", func(t *testing.T) {
		svc, def := setupConfirmService(t)
		var stdout, stderr bytes.Buffer
		runtime := Runtime{Stdin: strings.NewReader("y\n"), IsTerminal: func() bool { return true }}
		require.NoError(t, confirmOperation(ctx, kongContextForTest(t, &stdout, &stderr), runtime, svc, def, false, false))
		require.Contains(t, stderr.String(), "続行しますか?")
	})
}
