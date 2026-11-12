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
	"context"
	"testing"
	"time"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/stretchr/testify/require"
)

// newTestService は fake と fake の時計を使った Service を返します。
func newTestService(t *testing.T, f *fakeSDK) *Service {
	t.Helper()
	return &Service{
		applicationAPI:         f.app,
		clusterAPI:             f.cluster,
		newVersionAPI:          f.newVersionAPI,
		newAutoScalingGroupAPI: f.newAutoScalingGroupAPI,
		newWorkerNodeAPI:       f.newWorkerNodeAPI,
		clock:                  newFakeClock(),
	}
}

// manualDef は検証済みの manual 方式の定義を返します。
func manualDef(clusterName, appName string) *ApplicationDefinition {
	return &ApplicationDefinition{
		Cluster:     clusterName,
		Name:        appName,
		CPU:         1000,
		Memory:      1024,
		ScalingMode: v1.ScalingModeManual,
		FixedScale:  ptrInt32(2),
		Image:       "ghcr.io/example/app:v1",
	}
}

// runWithTicks は op を goroutine で実行し、完了するまで時計を進めて tick を供給します。
// 途中で状態が変化する待機系のテストに使います。
func runWithTicks(t *testing.T, s *Service, op func() (any, error)) (any, error) {
	t.Helper()
	clock, ok := s.clock.(*fakeClock)
	require.True(t, ok, "テスト用の時計が設定されていません")
	done := make(chan error, 1)
	var result any
	go func() {
		v, err := op()
		result = v
		done <- err
	}()
	for i := 0; i < 1000; i++ {
		select {
		case err := <-done:
			return result, err
		case <-time.After(time.Millisecond):
			clock.Advance(DefaultPollInterval)
		}
	}
	t.Fatal("操作が完了しません")
	return nil, nil
}

func TestDeployCreatesApplicationAndVersion(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)
	def := manualDef("c1", "app1")

	result, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.True(t, result.ApplicationCreated)
	require.Equal(t, "c1", result.Cluster)
	require.Equal(t, "app1", result.Application)
	require.NotEmpty(t, result.ApplicationID)
	require.Equal(t, v1.ApplicationVersionNumber(1), result.Version)

	// アプリケーションの作成と有効化の順序を確認します。
	require.Len(t, f.createApplicationCalls, 1)
	require.Equal(t, "app1", f.createApplicationCalls[0].name)
	require.Len(t, f.updateApplicationCalls, 1)
	require.NotNil(t, f.updateApplicationCalls[0].toVersion)
	require.Equal(t, int32(1), *f.updateApplicationCalls[0].toVersion)

	// 初回デプロイの registryPasswordAction は remove です。
	require.Len(t, f.createVersionCalls, 1)
	require.Equal(t, v1.RegistryPasswordActionRemove, f.createVersionCalls[0].RegistryPasswordAction)
	require.Equal(t, int64(1000), f.createVersionCalls[0].CPU)
	require.Equal(t, int64(1024), f.createVersionCalls[0].Memory)
	require.Equal(t, v1.ScalingModeManual, f.createVersionCalls[0].ScalingMode)
	require.Equal(t, ptrInt32(2), f.createVersionCalls[0].FixedScale)
	require.Equal(t, "ghcr.io/example/app:v1", f.createVersionCalls[0].Image)
}

func TestDeployExistingApplication(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	// アクティブバージョンがない既存アプリケーションへの再デプロイは remove です。
	def := manualDef("c1", "app1")
	result, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.False(t, result.ApplicationCreated)
	require.Len(t, f.createApplicationCalls, 0)
	require.Len(t, f.createVersionCalls, 1)
	require.Equal(t, v1.RegistryPasswordActionRemove, f.createVersionCalls[0].RegistryPasswordAction)
	require.Equal(t, v1.ApplicationVersionNumber(1), result.Version)
}

func TestDeployRegistryPasswordAction(t *testing.T) {
	tests := []struct {
		name          string
		password      *string
		activeVersion *int32
		want          v1.RegistryPasswordAction
	}{
		{"パスワードがある場合は new", ptrString("secret"), ptrInt32(3), v1.RegistryPasswordActionNew},
		{"パスワードが空の場合はアクティブバージョンで決まる", ptrString(""), ptrInt32(3), v1.RegistryPasswordActionKeep},
		{"パスワードなしでアクティブバージョンあり", nil, ptrInt32(3), v1.RegistryPasswordActionKeep},
		{"パスワードなしで初回デプロイ", nil, nil, v1.RegistryPasswordActionRemove},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeSDK(t)
			f.addCluster(t, "c1")
			app := f.addApp(t, "c1", "app1")
			app.activeVersion = tc.activeVersion
			s := newTestService(t, f)

			def := manualDef("c1", "app1")
			def.RegistryPassword = tc.password
			_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
			require.NoError(t, err)
			require.Len(t, f.createVersionCalls, 1)
			require.Equal(t, tc.want, f.createVersionCalls[0].RegistryPasswordAction)
			// パスワードはバージョン作成リクエストにだけ現れます。
			if tc.password != nil && *tc.password != "" {
				require.Equal(t, tc.password, f.createVersionCalls[0].RegistryPassword)
			}
		})
	}
}

func TestDeployWithWait(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.containers = twoRunningPlacements(1)
	s := newTestService(t, f)

	// 最初の確認でコンテナが running なら、追加の待ち時間なしで完了します。
	def := manualDef("c1", "app1")
	result, err := s.Deploy(t.Context(), def, &DeployOptions{})
	require.NoError(t, err)
	require.Equal(t, v1.ApplicationVersionNumber(1), result.Version)
}

func TestDeployWaitTimeout(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.containers = nil
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	_, err := runWithTicks(t, s, func() (any, error) {
		return s.Deploy(t.Context(), def, &DeployOptions{Timeout: DefaultPollInterval * 2})
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "タイムアウト")
	require.Contains(t, err.Error(), "有効化は完了しています")
}

func TestDeployWaitRetriesReadErrors(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	app.containers = twoRunningPlacements(1)
	f.containersErrCount = 2
	s := newTestService(t, f)

	// 参照エラーは打ち切らず、期限まで再試行します。
	def := manualDef("c1", "app1")
	_, err := runWithTicks(t, s, func() (any, error) {
		return s.Deploy(t.Context(), def, &DeployOptions{Timeout: time.Minute})
	})
	require.NoError(t, err)
	require.Equal(t, 3, f.containersCalls)
}

func TestDeployClusterNotFound(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "other")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), `クラスタ "c1" が見つかりません`)
	require.Len(t, f.createApplicationCalls, 0)
}

func TestDeploySameNameInOtherCluster(t *testing.T) {
	f := newFakeSDK(t)
	c1 := f.addCluster(t, "c1")
	f.addCluster(t, "c2")
	app := f.addApp(t, "c2", "app1")
	s := newTestService(t, f)

	// 同名のアプリケーションが別クラスタにあっても、対象クラスタには新規作成します。
	def := manualDef("c1", "app1")
	result, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.True(t, result.ApplicationCreated)
	require.Len(t, f.createApplicationCalls, 1)
	require.Equal(t, c1.clusterID, f.createApplicationCalls[0].clusterID)
	require.NotEqual(t, app.applicationID, result.ApplicationID)
}

func TestDeployValidationErrorsBeforeAPICall(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	def.CPU = 0
	_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cpu")
	require.Len(t, f.createApplicationCalls, 0)
	require.Len(t, f.createVersionCalls, 0)
}

// pagingService はページサイズを一時的に変更した Service を返します。
func pagingService(t *testing.T, f *fakeSDK, pageSize int64) *Service {
	t.Helper()
	old := listPageSize
	listPageSize = pageSize
	t.Cleanup(func() { listPageSize = old })
	s := newTestService(t, f)
	return s
}

func TestDeployFindsAcrossPages(t *testing.T) {
	f := newFakeSDK(t)
	for i := 0; i < 3; i++ {
		f.addCluster(t, "c0")
	}
	f.addCluster(t, "c1")
	for i := 0; i < 3; i++ {
		f.addApp(t, "c1", "app0")
	}
	app := f.addApp(t, "c1", "app1")
	s := pagingService(t, f, 1)

	// クラスタとアプリケーションがページングの末尾にあっても見つけられます。
	def := manualDef("c1", "app1")
	result, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
	require.NoError(t, err)
	require.False(t, result.ApplicationCreated)
	require.Equal(t, app.applicationID, result.ApplicationID)
}

// readMutatingAppAPI は Read の応答を加工する application.ApplicationAPI です。
// 応答の不整合を再現するために使います。
type readMutatingAppAPI struct {
	application.ApplicationAPI
	mutate func(*application.ApplicationDetail)
}

func (m *readMutatingAppAPI) Read(ctx context.Context, id v1.ApplicationID) (*application.ApplicationDetail, error) {
	detail, err := m.ApplicationAPI.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	m.mutate(detail)
	return detail, nil
}

func TestFindApplicationResponseMismatch(t *testing.T) {
	def := manualDef("c1", "app1")

	// 応答の不整合は後続の変更 API が別のリソースを変更する恐れがあるため、変更に進みません。
	t.Run("応答の ID が一致しなければエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := &Service{
			applicationAPI: &readMutatingAppAPI{ApplicationAPI: f.app, mutate: func(d *application.ApplicationDetail) {
				d.ApplicationID = v1.ApplicationID{0xFF}
			}},
			clusterAPI:    f.cluster,
			newVersionAPI: f.newVersionAPI,
			clock:         newFakeClock(),
		}
		_, err := s.Status(t.Context(), def)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ID が一致しません")
	})

	t.Run("応答の名前が一致しなければエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := &Service{
			applicationAPI: &readMutatingAppAPI{ApplicationAPI: f.app, mutate: func(d *application.ApplicationDetail) {
				d.Name = "other"
			}},
			clusterAPI:    f.cluster,
			newVersionAPI: f.newVersionAPI,
			clock:         newFakeClock(),
		}
		_, err := s.Status(t.Context(), def)
		require.Error(t, err)
		require.Contains(t, err.Error(), "名前が一致しません")
	})

	t.Run("作成後の応答の所属クラスタが一致しなければエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := &Service{
			applicationAPI: &readMutatingAppAPI{ApplicationAPI: f.app, mutate: func(d *application.ApplicationDetail) {
				d.ClusterName = "other-cluster"
			}},
			clusterAPI:    f.cluster,
			newVersionAPI: f.newVersionAPI,
			clock:         newFakeClock(),
		}
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "所属クラスタが一致しません")
		// バージョンの作成に進みません。
		require.Len(t, f.createVersionCalls, 0)
	})
}

func TestRollbackFromActiveVersion(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	// バージョン 1〜3 を作り、バージョン 3 をアクティブにします。
	def := manualDef("c1", "app1")
	for i := 0; i < 3; i++ {
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)
	}
	require.Len(t, f.updateApplicationCalls, 3)

	result, err := s.Rollback(t.Context(), def, &RollbackOptions{NoWait: true})
	require.NoError(t, err)
	require.False(t, result.ApplicationCreated)
	require.Equal(t, v1.ApplicationVersionNumber(2), result.Version)
	require.NotNil(t, app.activeVersion)
	require.Equal(t, int32(2), *app.activeVersion)
}

func TestRollbackInactiveApplication(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	for i := 0; i < 2; i++ {
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)
	}
	_, err := s.Deactivate(t.Context(), def, &WaitOptions{NoWait: true})
	require.NoError(t, err)

	result, err := s.Rollback(t.Context(), def, &RollbackOptions{NoWait: true})
	require.NoError(t, err)
	// 非活性のアプリケーションの再開は最新のバージョンです。
	require.Equal(t, v1.ApplicationVersionNumber(2), result.Version)
}

func TestRollbackWithTarget(t *testing.T) {
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	f.addApp(t, "c1", "app1")
	s := newTestService(t, f)

	def := manualDef("c1", "app1")
	for i := 0; i < 3; i++ {
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)
	}

	result, err := s.Rollback(t.Context(), def, &RollbackOptions{Target: ptrVersion(1), NoWait: true})
	require.NoError(t, err)
	require.Equal(t, v1.ApplicationVersionNumber(1), result.Version)
}

func TestRollbackErrors(t *testing.T) {
	def := manualDef("c1", "app1")

	t.Run("アクティブバージョンへの rollback はエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)
		for i := 0; i < 2; i++ {
			_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
			require.NoError(t, err)
		}
		before := len(f.updateApplicationCalls)
		_, err := s.Rollback(t.Context(), def, &RollbackOptions{Target: ptrVersion(2), NoWait: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "すでにアクティブ")
		require.Len(t, f.updateApplicationCalls, before)
	})

	t.Run("存在しないバージョンへの rollback はエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)
		before := len(f.updateApplicationCalls)
		_, err = s.Rollback(t.Context(), def, &RollbackOptions{Target: ptrVersion(9), NoWait: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "バージョン 9 が見つかりません")
		require.Len(t, f.updateApplicationCalls, before)
	})

	t.Run("前のバージョンがない場合はエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)
		_, err = s.Rollback(t.Context(), def, &RollbackOptions{NoWait: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "より前のバージョンがありません")
	})

	t.Run("バージョンがないアプリケーションはエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)
		_, err := s.Rollback(t.Context(), def, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "バージョンが 1 つもない")
	})

	t.Run("アプリケーションが見つからない場合はエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)
		_, err := s.Rollback(t.Context(), def, &RollbackOptions{NoWait: true})
		require.Error(t, err)
		var notFound *ApplicationNotFoundError
		require.ErrorAs(t, err, &notFound)
	})
}

func TestDeactivate(t *testing.T) {
	t.Run("アクティブバージョンを解除して停止を待つ", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		def := manualDef("c1", "app1")
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)

		result, err := s.Deactivate(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.DeactivatedVersion)
		require.Equal(t, int32(1), *result.DeactivatedVersion)
		require.Nil(t, app.activeVersion)
		require.NotNil(t, app.desiredCount)
		require.Equal(t, int32(0), *app.desiredCount)
	})

	t.Run("すでに非活性なら変更しない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		def := manualDef("c1", "app1")
		result, err := s.Deactivate(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)
		require.Nil(t, result.DeactivatedVersion)
		require.Len(t, f.updateApplicationCalls, 0)
	})

	t.Run("DesiredCount の null も停止完了として扱う", func(t *testing.T) {
		f := newFakeSDK(t)
		f.deactivatedDesiredCount = nil
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		def := manualDef("c1", "app1")
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)

		_, err = s.Deactivate(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)
	})

	t.Run("対象が別クラスタなら変更しない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addCluster(t, "c2")
		f.addApp(t, "c2", "app1")
		s := newTestService(t, f)

		def := manualDef("c1", "app1")
		_, err := s.Deactivate(t.Context(), def, &WaitOptions{})
		require.Error(t, err)
		var notFound *ApplicationNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Len(t, f.updateApplicationCalls, 0)
	})
}

func TestDelete(t *testing.T) {
	def := manualDef("c1", "app1")

	t.Run("非活性化して削除する", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)

		result, err := s.Delete(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)
		require.Equal(t, app.applicationID, result.ApplicationID)
		// 解除と削除が行われます。
		require.Len(t, f.updateApplicationCalls, 2)
		require.NotNil(t, f.updateApplicationCalls[0].toVersion)
		require.Nil(t, f.updateApplicationCalls[1].toVersion)
		require.Len(t, f.deleteApplicationCalls, 1)
	})

	t.Run("非活性のアプリケーションを削除する", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		_, err := s.Delete(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)
		require.Len(t, f.updateApplicationCalls, 0)
		require.Len(t, f.deleteApplicationCalls, 1)
	})

	t.Run("削除を再試行する", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.deleteFailures = 2
		s := newTestService(t, f)

		_, err := runWithTicks(t, s, func() (any, error) {
			return s.Delete(t.Context(), def, &WaitOptions{Timeout: time.Minute})
		})
		require.NoError(t, err)
		require.Len(t, f.deleteApplicationCalls, 1)
	})

	t.Run("削除後の不在を確認する", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		_, err := s.Delete(t.Context(), def, &WaitOptions{})
		require.NoError(t, err)

		// 削除後の参照は見つかりません。
		_, err = s.Status(t.Context(), def)
		require.Error(t, err)
		var notFound *ApplicationNotFoundError
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("タイムアウトしたらエラー", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.deleteFailures = 1000
		s := newTestService(t, f)

		_, err := runWithTicks(t, s, func() (any, error) {
			return s.Delete(t.Context(), def, &WaitOptions{Timeout: DefaultPollInterval * 2})
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "アプリケーションの削除に失敗しました")
	})

	t.Run("削除済みを示すエラーが返っても不在確認で完了する", func(t *testing.T) {
		// 削除 API が成功した後も参照が遅延で存在を返すと、次の削除は既に削除済みを
		// 示すエラーで失敗します。不在確認が NotFound を返せば成功として扱います。
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		app.lingeringReads = 1
		s := newTestService(t, f)

		_, err := runWithTicks(t, s, func() (any, error) {
			return s.Delete(t.Context(), def, &WaitOptions{Timeout: time.Minute})
		})
		require.NoError(t, err)
		require.Len(t, f.deleteApplicationCalls, 1)
	})
}

func TestNilDefinition(t *testing.T) {
	f := newFakeSDK(t)
	s := newTestService(t, f)
	_, err := s.Deploy(t.Context(), nil, nil)
	require.Error(t, err)
	_, err = s.Rollback(t.Context(), nil, nil)
	require.Error(t, err)
	_, err = s.Deactivate(t.Context(), nil, nil)
	require.Error(t, err)
	_, err = s.Delete(t.Context(), nil, nil)
	require.Error(t, err)
	_, err = s.Status(t.Context(), nil)
	require.Error(t, err)
	_, err = s.Versions(t.Context(), nil)
	require.Error(t, err)
	_, err = s.Containers(t.Context(), nil)
	require.Error(t, err)
	_, err = s.Render(nil)
	require.Error(t, err)
	_, err = s.Diff(t.Context(), nil)
	require.Error(t, err)
	_, err = s.Init(t.Context(), "", "")
	require.Error(t, err)
}

func TestContextCancelBeforeMutation(t *testing.T) {
	def := manualDef("c1", "app1")
	t.Run("deploy は変更 API を呼び出さない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		// すでにキャンセル済みの context では変更 API を呼び出しません。
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := s.Deploy(ctx, def, &DeployOptions{NoWait: true})
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, f.createApplicationCalls, 0)
		require.Len(t, f.createVersionCalls, 0)
		require.Len(t, f.updateApplicationCalls, 0)
	})

	t.Run("deactivate は変更 API を呼び出さない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		// 活性なアプリケーションに対する解除がキャンセルで打ち切られます。
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = s.Deactivate(ctx, def, &WaitOptions{NoWait: true})
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, f.updateApplicationCalls, 1) // deploy の有効化だけ
		_ = app
	})

	t.Run("delete は変更 API を呼び出さない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		app := f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		// 活性なアプリケーションの削除が解除の前で打ち切られます。
		_, err := s.Deploy(t.Context(), def, &DeployOptions{NoWait: true})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = s.Delete(ctx, def, &WaitOptions{NoWait: true})
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, f.updateApplicationCalls, 1) // deploy の有効化だけ
		require.Len(t, f.deleteApplicationCalls, 0)
		_ = app
	})
}
