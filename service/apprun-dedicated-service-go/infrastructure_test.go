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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateAutoScalingGroup(t *testing.T) {
	t.Run("作成後にワーカノードが healthy になるまで待つ", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)

		var result *CreateAutoScalingGroupResult
		var nodeAdded bool
		err := runWithTicksAndHook(t, s, func() error {
			var err error
			result, err = s.CreateAutoScalingGroup(t.Context(), "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3, nil)
			return err
		}, func() {
			if nodeAdded {
				return
			}
			f.addWorkerNode(t, "c1", "asg1", true)
			nodeAdded = true
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "c1", result.Cluster)
		require.Equal(t, "asg1", result.Name)
		require.NotEmpty(t, result.AutoScalingGroupID)
		require.Equal(t, "is1a", result.Zone)
		require.Equal(t, "@cloudservice/worker/001", result.WorkerServiceClassPath)
		require.Equal(t, int32(1), result.MinNodes)
		require.Equal(t, int32(3), result.MaxNodes)
	})

	t.Run("NoWait では healthy 待ちを省略", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)

		result, err := s.CreateAutoScalingGroup(t.Context(), "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3, &CreateAutoScalingGroupOptions{NoWait: true})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "asg1", result.Name)
	})

	t.Run("クラスタが存在しない", func(t *testing.T) {
		f := newFakeSDK(t)
		s := newTestService(t, f)

		_, err := s.CreateAutoScalingGroup(t.Context(), "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "クラスタ \"c1\" が見つかりません")
	})

	t.Run("同名のグループが既に存在", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addAutoScalingGroup(t, "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3)
		s := newTestService(t, f)

		_, err := s.CreateAutoScalingGroup(t.Context(), "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "オートスケーリンググループ \"asg1\" は既に存在します")
	})
}

func TestDeleteAutoScalingGroup(t *testing.T) {
	t.Run("削除して不在確認まで行う", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addAutoScalingGroup(t, "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3)
		s := newTestService(t, f)

		result, err := s.DeleteAutoScalingGroup(t.Context(), "c1", "asg1", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "c1", result.Cluster)
		require.Equal(t, "asg1", result.Name)
		require.NotEmpty(t, result.AutoScalingGroupID)

		_, err = s.DeleteAutoScalingGroup(t.Context(), "c1", "asg1", nil)
		require.Error(t, err)
		var notFound *AutoScalingGroupNotFoundError
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("NoWait では削除完了待ちを省略", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addAutoScalingGroup(t, "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3)
		s := newTestService(t, f)

		result, err := s.DeleteAutoScalingGroup(t.Context(), "c1", "asg1", &DeleteAutoScalingGroupOptions{NoWait: true})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "asg1", result.Name)
		require.Len(t, f.asgs, 1)
		require.True(t, f.asgs[0].deleted)
	})

	t.Run("既に削除済み", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		asg := f.addAutoScalingGroup(t, "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3)
		asg.deleted = true
		s := newTestService(t, f)

		_, err := s.DeleteAutoScalingGroup(t.Context(), "c1", "asg1", nil)
		require.Error(t, err)
		var notFound *AutoScalingGroupNotFoundError
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("グループが存在しない", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)

		_, err := s.DeleteAutoScalingGroup(t.Context(), "c1", "asg1", nil)
		require.Error(t, err)
		var notFound *AutoScalingGroupNotFoundError
		require.ErrorAs(t, err, &notFound)
	})
}

func TestDeleteCluster(t *testing.T) {
	t.Run("関連リソースがなければ削除", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		s := newTestService(t, f)

		result, err := s.DeleteCluster(t.Context(), "c1", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "c1", result.Name)
		require.NotEmpty(t, result.ClusterID)
		require.True(t, f.clusters[0].deleted)

		_, err = s.DeleteCluster(t.Context(), "c1", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "クラスタ \"c1\" が見つかりません")
	})

	t.Run("アプリケーションが残っていると中断", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addApp(t, "c1", "app1")
		s := newTestService(t, f)

		_, err := s.DeleteCluster(t.Context(), "c1", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "削除対象外のリソースが残っています")
		require.Contains(t, err.Error(), "application \"app1\"")
		require.False(t, f.clusters[0].deleted)
	})

	t.Run("オートスケーリンググループが残っていると中断", func(t *testing.T) {
		f := newFakeSDK(t)
		f.addCluster(t, "c1")
		f.addAutoScalingGroup(t, "c1", "asg1", "is1a", "@cloudservice/worker/001", 1, 3)
		s := newTestService(t, f)

		_, err := s.DeleteCluster(t.Context(), "c1", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "削除対象外のリソースが残っています")
		require.Contains(t, err.Error(), "auto-scaling-group \"asg1\"")
		require.False(t, f.clusters[0].deleted)
	})

	t.Run("クラスタが存在しない", func(t *testing.T) {
		f := newFakeSDK(t)
		s := newTestService(t, f)

		_, err := s.DeleteCluster(t.Context(), "c1", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "クラスタ \"c1\" が見つかりません")
	})
}

// runWithTicksAndHook は runWithTicks と同じく時計を進めますが、各 tick の直前に
// hook を呼び出して状態を変化させることができます。
func runWithTicksAndHook(t *testing.T, s *Service, op func() error, hook func()) error {
	t.Helper()
	clock, ok := s.clock.(*fakeClock)
	require.True(t, ok, "テスト用の時計が設定されていません")
	done := make(chan error, 1)
	go func() {
		done <- op()
	}()
	for i := 0; i < 1000; i++ {
		select {
		case err := <-done:
			return err
		case <-time.After(time.Millisecond):
			hook()
			clock.Advance(DefaultPollInterval)
		}
	}
	t.Fatal("操作が完了しません")
	return nil
}
