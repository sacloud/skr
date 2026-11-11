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
	"encoding/json"
	"errors"
	"testing"
	"time"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/stretchr/testify/require"
)

// jsonString は map をキー順に整列した JSON 文字列へ変換します。出力内容の確認に使います。
func jsonString(t *testing.T, m map[string]any) string {
	t.Helper()
	data, err := json.Marshal(m)
	require.NoError(t, err)
	return string(data)
}

func TestStatus(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.activeVersion = ptrInt32(1)
	app.desiredCount = ptrInt32(2)
	app.versions[1] = &version.VersionDetail{Version: 1, Image: "ghcr.io/example/app:v1", CPU: 1000, Memory: 1024, ActiveNodeCount: 2, Created: 1700000000}
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	status, err := s.Status(t.Context(), def)
	require.NoError(t, err)
	require.Equal(t, "c1", status.Cluster)
	require.Equal(t, "app1", status.Application.Name)
	require.Equal(t, app.applicationID, status.Application.ID)
	require.NotNil(t, status.Application.ActiveVersion)
	require.Equal(t, int32(1), *status.Application.ActiveVersion)
	require.NotNil(t, status.Application.DesiredCount)
	require.Equal(t, int32(2), *status.Application.DesiredCount)
	require.NotNil(t, status.ActiveVersionDetail)
	require.Equal(t, v1.ApplicationVersionNumber(1), status.ActiveVersionDetail.Version)
	require.Equal(t, "ghcr.io/example/app:v1", status.ActiveVersionDetail.Image)
	require.Equal(t, int64(1000), status.ActiveVersionDetail.CPU)
	require.Equal(t, int64(1024), status.ActiveVersionDetail.Memory)
	require.Equal(t, int64(2), status.ActiveVersionDetail.ActiveNodeCount)
}

func TestStatusNotActive(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	// アクティブバージョンがなくても参照できます。
	status, err := s.Status(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Nil(t, status.Application.ActiveVersion)
	require.Nil(t, status.ActiveVersionDetail)
	require.Nil(t, status.Application.DesiredCount)
}

func TestStatusNotFound(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	_, err := s.Status(t.Context(), manualDef("c1", "app1"))
	require.Error(t, err)
	var notFound *ApplicationNotFoundError
	require.ErrorAs(t, err, &notFound)
	require.Equal(t, "c1", notFound.Cluster)
	require.Equal(t, "app1", notFound.Name)
	require.Contains(t, err.Error(), `クラスタ "c1" にアプリケーション "app1" が見つかりません`)
}

func TestStatusReadError(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	f.readErr = errors.New("参照エラー")
	s := newTestService(t, f)

	_, err := s.Status(t.Context(), manualDef("c1", "app1"))
	require.Error(t, err)
	var notFound *ApplicationNotFoundError
	require.NotErrorAs(t, err, &notFound)
}

func TestVersions(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.versions[1] = &version.VersionDetail{Version: 1, Image: "ghcr.io/example/app:v1", ActiveNodeCount: 1, Created: 1700000000}
	app.versions[2] = &version.VersionDetail{Version: 2, Image: "ghcr.io/example/app:v2", ActiveNodeCount: 3, Created: 1700000100}
	app.activeVersion = ptrInt32(2)
	s := newTestService(t, f)

	versions, err := s.Versions(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// アクティブバージョンが先頭に来ます。
	require.Equal(t, v1.ApplicationVersionNumber(2), versions[0].Version)
	require.True(t, versions[0].Active)
	require.Equal(t, v1.ApplicationVersionNumber(1), versions[1].Version)
	require.False(t, versions[1].Active)
	require.Equal(t, "ghcr.io/example/app:v2", versions[0].Image)
	require.Equal(t, int64(3), versions[0].ActiveNodeCount)
	require.Equal(t, time.Unix(1700000100, 0).UTC(), versions[0].Created)
}

func TestVersionsNotFound(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	_, err := s.Versions(t.Context(), manualDef("c1", "app1"))
	var notFound *ApplicationNotFoundError
	require.ErrorAs(t, err, &notFound)
}

func TestContainers(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.containers = []application.Placement{runningPlacement(1)}
	s := newTestService(t, f)

	placements, err := s.Containers(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Len(t, placements, 1)
	require.Equal(t, "container-1", placements[0].ContainersStats.Containers[0].ID)
}

func TestContainersNotFound(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	_, err := s.Containers(t.Context(), manualDef("c1", "app1"))
	var notFound *ApplicationNotFoundError
	require.ErrorAs(t, err, &notFound)
}

func TestRenderRoundTrip(t *testing.T) {
	f := newFakeSDK(t)
	s := newTestService(t, f)

	// 定義から Render した内容を ParseDefinition で読み直せます。
	def := manualDef("c1", "app1")
	def.Cmd = []string{"serve", "--port", "8080"}
	def.RegistryUsername = ptrString("user")
	def.RegistryPassword = ptrString("secret")
	def.ExposedPorts = []version.ExposedPort{
		{
			TargetPort:       8080,
			LoadBalancerPort: ptrPort(80),
			UseLetsEncrypt:   true,
			Host:             []string{"app.example.com"},
			HealthCheck:      &v1.HealthCheck{Path: "/health", IntervalSeconds: 30, TimeoutSeconds: 5},
		},
	}
	def.Env = []version.EnvironmentVariable{
		{Key: "LOG_LEVEL", Value: ptrString("info")},
		{Key: "API_TOKEN", Value: ptrString("secret-token"), Secret: true},
	}
	rendered, err := s.Render(def)
	require.NoError(t, err)
	require.NotContains(t, jsonString(t, rendered), "registryPassword")

	parsed, err := ParseDefinition([]byte(jsonString(t, rendered)))
	require.NoError(t, err)
	require.Equal(t, "c1", parsed.Cluster)
	require.Equal(t, "app1", parsed.Name)
	require.Equal(t, int64(1000), parsed.CPU)
	require.Equal(t, int64(1024), parsed.Memory)
	require.Equal(t, v1.ScalingModeManual, parsed.ScalingMode)
	require.Equal(t, ptrInt32(2), parsed.FixedScale)
	require.Equal(t, "ghcr.io/example/app:v1", parsed.Image)
	require.Equal(t, []string{"serve", "--port", "8080"}, parsed.Cmd)
	require.Equal(t, ptrString("user"), parsed.RegistryUsername)
	require.Nil(t, parsed.RegistryPassword)
	require.Len(t, parsed.ExposedPorts, 1)
	require.Equal(t, ptrPort(80), parsed.ExposedPorts[0].LoadBalancerPort)
	require.Len(t, parsed.Env, 2)
	require.Equal(t, "info", *parsed.Env[0].Value)
	// secret=true の値は出力に含めません。
	require.Nil(t, parsed.Env[1].Value)
	require.True(t, parsed.Env[1].Secret)

	// 元の定義は書き換わりません。
	require.NotNil(t, def.RegistryPassword)
	require.Equal(t, "secret-token", *def.Env[1].Value)
}

func TestRenderOmitsEmptyFields(t *testing.T) {
	f := newFakeSDK(t)
	s := newTestService(t, f)

	rendered, err := s.Render(manualDef("c1", "app1"))
	require.NoError(t, err)
	out := jsonString(t, rendered)
	require.NotContains(t, out, "cmd")
	require.NotContains(t, out, "env")
	require.NotContains(t, out, "exposedPorts")
	require.NotContains(t, out, "registryUsername")
	require.Contains(t, out, `"fixedScale":2`)
}

func TestDiff(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	def.Cmd = []string{"serve"}

	// ローカルの定義だけの場合、リモート側は識別子のみになります。
	diff, err := s.Diff(t.Context(), def)
	require.NoError(t, err)
	require.Contains(t, jsonString(t, diff.Local), "ghcr.io/example/app:v1")
	require.Equal(t, map[string]any{"cluster": "c1", "name": "app1"}, diff.Remote)

	// 同じ定義をデプロイすると差分はありません。
	_, err = s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	diff, err = s.Diff(t.Context(), def)
	require.NoError(t, err)
	require.Equal(t, diff.Local, diff.Remote)
}

func TestDiffMaskSecrets(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	def.RegistryPassword = ptrString("p@ssw0rd-value")
	def.Env = []version.EnvironmentVariable{{Key: "API_TOKEN", Value: ptrString("token-value"), Secret: true}}
	_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)

	// ローカルとリモートの両方から秘密情報が除かれます。
	diff, err := s.Diff(t.Context(), def)
	require.NoError(t, err)
	local := jsonString(t, diff.Local)
	require.NotContains(t, local, "p@ssw0rd-value")
	require.NotContains(t, local, "token-value")
	require.NotContains(t, local, "registryPassword")
	require.Equal(t, diff.Local, diff.Remote)

	// 元の定義は書き換わりません。
	require.NotNil(t, def.RegistryPassword)
	require.Equal(t, "p@ssw0rd-value", *def.RegistryPassword)
	require.Equal(t, "token-value", *def.Env[0].Value)
}

func TestDiffInactiveApplication(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	// 非活性のアプリケーションはリモート側に識別子だけを返します。
	diff, err := s.Diff(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"cluster": "c1", "name": "app1"}, diff.Remote)
}

func TestDiffNotFound(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	diff, err := s.Diff(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"cluster": "c1", "name": "app1"}, diff.Remote)
}

func TestDiffMissingActiveVersion(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.activeVersion = ptrInt32(1)
	s := newTestService(t, f)

	// アクティブバージョンが削除済みのときはエラーになります。
	_, err := s.Diff(t.Context(), manualDef("c1", "app1"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "アクティブバージョン 1 の情報取得に失敗しました")
}

func TestInit(t *testing.T) {
	t.Run("既存のアプリケーションから定義を作る", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.versions[1] = &version.VersionDetail{
			Version:          1,
			CPU:              500,
			Memory:           1024,
			ScalingMode:      v1.ScalingModeManual,
			FixedScale:       ptrInt32(1),
			Image:            "ghcr.io/example/app:v1",
			RegistryUsername: ptrString("user"),
			RegistryPassword: ptrString("masked"),
		}
		app.activeVersion = ptrInt32(1)
		s := newTestService(t, f)

		def, err := s.Init(t.Context(), "c1", "app1")
		require.NoError(t, err)
		require.Equal(t, "c1", def.Cluster)
		require.Equal(t, "app1", def.Name)
		require.Equal(t, int64(500), def.CPU)
		require.Equal(t, int64(1024), def.Memory)
		require.Equal(t, v1.ScalingModeManual, def.ScalingMode)
		require.Equal(t, ptrInt32(1), def.FixedScale)
		require.Equal(t, "ghcr.io/example/app:v1", def.Image)
		require.Equal(t, ptrString("user"), def.RegistryUsername)
		// 秘密情報は復元しません。
		require.Nil(t, def.RegistryPassword)
	})

	t.Run("アクティブバージョンを優先する", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.versions[1] = &version.VersionDetail{Version: 1, CPU: 500, Memory: 1024, ScalingMode: v1.ScalingModeManual, FixedScale: ptrInt32(1), Image: "ghcr.io/example/app:v1"}
		app.versions[2] = &version.VersionDetail{Version: 2, CPU: 1000, Memory: 2048, ScalingMode: v1.ScalingModeManual, FixedScale: ptrInt32(2), Image: "ghcr.io/example/app:v2"}
		app.activeVersion = ptrInt32(1)
		s := newTestService(t, f)

		def, err := s.Init(t.Context(), "c1", "app1")
		require.NoError(t, err)
		require.Equal(t, int64(500), def.CPU)
		require.Equal(t, "ghcr.io/example/app:v1", def.Image)
	})

	t.Run("アクティブバージョンがないときは最新を使う", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.versions[1] = &version.VersionDetail{Version: 1, CPU: 500, Memory: 1024, ScalingMode: v1.ScalingModeManual, FixedScale: ptrInt32(1), Image: "ghcr.io/example/app:v1"}
		app.versions[2] = &version.VersionDetail{Version: 2, CPU: 1000, Memory: 2048, ScalingMode: v1.ScalingModeManual, FixedScale: ptrInt32(2), Image: "ghcr.io/example/app:v2"}
		s := newTestService(t, f)

		def, err := s.Init(t.Context(), "c1", "app1")
		require.NoError(t, err)
		require.Equal(t, int64(1000), def.CPU)
		require.Equal(t, "ghcr.io/example/app:v2", def.Image)
	})

	t.Run("バージョンがないときは識別子だけの定義", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		def, err := s.Init(t.Context(), "c1", "app1")
		require.NoError(t, err)
		require.Equal(t, &ApplicationDefinition{Cluster: "c1", Name: "app1"}, def)
		require.Error(t, def.ValidateForDeploy())
	})

	t.Run("アプリケーションがないときはエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)

		_, err := s.Init(t.Context(), "c1", "app1")
		var notFound *ApplicationNotFoundError
		require.ErrorAs(t, err, &notFound)
	})
}

func TestFindApplicationSkipsOtherCluster(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addCluster(t, "c2")
	// 同名のアプリケーションが別クラスタに先にあっても、対象クラスタのものを見つけられます。
	f.addApp(t, "c2", "app1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	status, err := s.Status(t.Context(), manualDef("c1", "app1"))
	require.NoError(t, err)
	require.Equal(t, "c1", status.Cluster)
}
