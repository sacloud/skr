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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/stretchr/testify/require"
)

// fakeClock はテスト用の時計です。Advance で時刻を進め、ticker に tick を供給します。
type fakeClock struct {
	mu           sync.Mutex
	now          time.Time
	ticks        chan time.Time
	lastInterval time.Duration
}

type fakeTicker struct {
	clock *fakeClock
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:   time.Unix(1700000000, 0),
		ticks: make(chan time.Time, 1024),
	}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(d time.Duration) ticker {
	c.mu.Lock()
	c.lastInterval = d
	c.mu.Unlock()
	return &fakeTicker{clock: c}
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.clock.ticks
}

func (t *fakeTicker) Stop() {}

// Advance は時刻を進めて tick を1つ供給します。
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	c.mu.Unlock()
	select {
	case c.ticks <- now:
	default:
	}
}

// runningPlacement は指定バージョンの running コンテナを持つ Placement を返します。
func runningPlacement(version v1.ApplicationVersionNumber) application.Placement {
	return application.Placement{
		ContainersStats: v1.ApplicationCurrentContainers{
			Containers: []v1.ApplicationCurrentContainer{
				{ID: "container-1", ApplicationVersion: version, State: "running"},
			},
		},
	}
}

// twoRunningPlacements は指定バージョンの running コンテナを 2 ノードに配置した Placement を返します。
func twoRunningPlacements(version v1.ApplicationVersionNumber) []application.Placement {
	second := runningPlacement(version)
	second.ContainersStats.Containers[0].ID = "container-2"
	return []application.Placement{runningPlacement(version), second}
}

func TestContainersRunning(t *testing.T) {
	running := func(id string, version v1.ApplicationVersionNumber, state string) v1.ApplicationCurrentContainer {
		return v1.ApplicationCurrentContainer{ID: id, ApplicationVersion: version, State: state}
	}
	placements := func(nodes ...[]v1.ApplicationCurrentContainer) []application.Placement {
		var out []application.Placement
		for _, containers := range nodes {
			out = append(out, application.Placement{ContainersStats: v1.ApplicationCurrentContainers{Containers: containers}})
		}
		return out
	}

	tests := []struct {
		name        string
		placements  []application.Placement
		version     v1.ApplicationVersionNumber
		wantTotal   int
		wantRunning int
	}{
		{
			name:        "対象バージョンのコンテナを数える",
			placements:  placements([]v1.ApplicationCurrentContainer{running("c1", 2, "running"), running("c2", 2, "running")}),
			version:     2,
			wantTotal:   2,
			wantRunning: 2,
		},
		{
			name:       "別バージョンのコンテナは対象外",
			placements: placements([]v1.ApplicationCurrentContainer{running("c1", 1, "running")}),
			version:    2,
			wantTotal:  0,
		},
		{
			name:        "running 以外も total に数える",
			placements:  placements([]v1.ApplicationCurrentContainer{running("c1", 2, "running"), running("c2", 2, "exited")}),
			version:     2,
			wantTotal:   2,
			wantRunning: 1,
		},
		{
			name:       "コンテナがない場合は 0",
			placements: nil,
			version:    2,
			wantTotal:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runningCount, total := containersRunning(tc.placements, tc.version)
			require.Equal(t, tc.wantTotal, total)
			require.Equal(t, tc.wantRunning, runningCount)
		})
	}
}

// newWaitRunningService は waitRunning の完了条件を検証するための Service と fake を
// 組み立てます。
func newWaitRunningService(t *testing.T) (*Service, *fakeSDK, *fakeApp) {
	t.Helper()
	f := newFakeSDK(t)
	f.addCluster(t, "c1")
	app := f.addApp(t, "c1", "app1")
	return newTestService(t, f), f, app
}

func TestWaitRunningRequiresRequestedContainers(t *testing.T) {
	t.Run("要求されたコンテナ数が一部しか現れていない間は完了しない", func(t *testing.T) {
		s, _, app := newWaitRunningService(t)

		// DesiredCount が 2 に対し、running なコンテナは 1 つのままでは完了しません。
		two := int32(2)
		app.desiredCount = &two
		app.containers = []application.Placement{runningPlacement(1)}

		_, err := runWithTicks(t, s, func() (any, error) {
			return nil, s.waitRunning(t.Context(), app.applicationID, 1, DefaultPollInterval*2)
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "タイムアウト")
	})

	t.Run("要求されたコンテナ数すべてが running なら完了", func(t *testing.T) {
		s, _, app := newWaitRunningService(t)

		two := int32(2)
		app.desiredCount = &two
		app.containers = twoRunningPlacements(1)

		err := s.waitRunning(t.Context(), app.applicationID, 1, time.Minute)
		require.NoError(t, err)
	})

	t.Run("DesiredCount が確定していない間は完了しない", func(t *testing.T) {
		s, _, app := newWaitRunningService(t)

		// DesiredCount が null の間は、running なコンテナがあっても完了しません。
		app.containers = []application.Placement{runningPlacement(1)}

		_, err := runWithTicks(t, s, func() (any, error) {
			return nil, s.waitRunning(t.Context(), app.applicationID, 1, DefaultPollInterval*2)
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "タイムアウト")
	})

	for _, desired := range []int32{0, -1} {
		t.Run(fmt.Sprintf("DesiredCount が %d の場合は完了しない", desired), func(t *testing.T) {
			s, _, app := newWaitRunningService(t)

			// 0 や負数は要求されたコンテナ数として確定していないため、
			// running なコンテナがあっても完了しません。
			d := desired
			app.desiredCount = &d
			app.containers = []application.Placement{runningPlacement(1)}

			_, err := runWithTicks(t, s, func() (any, error) {
				return nil, s.waitRunning(t.Context(), app.applicationID, 1, DefaultPollInterval*2)
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "タイムアウト")
		})
	}
}

func TestPollImmediateCheck(t *testing.T) {
	clock := newFakeClock()
	s := &Service{clock: clock}

	// 最初の確認で完了条件を満たせば、tick を待たずに終了します。
	var calls int
	err := s.poll(t.Context(), time.Minute, func() (bool, error) {
		calls++
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestPollTimeout(t *testing.T) {
	clock := newFakeClock()
	s := &Service{clock: clock}

	// 完了しないまま期限を超えるとタイムアウトします。
	var calls int
	_, err := runWithTicks(t, s, func() (any, error) {
		return nil, s.poll(t.Context(), DefaultPollInterval*2, func() (bool, error) {
			calls++
			return false, nil
		})
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "タイムアウト")
	require.GreaterOrEqual(t, calls, 2)
}

func TestPollCheckError(t *testing.T) {
	clock := newFakeClock()
	s := &Service{clock: clock}

	// check が返したエラーはそのまま伝わります。
	wantErr := errors.New("確認に失敗しました")
	_, err := runWithTicks(t, s, func() (any, error) {
		return nil, s.poll(t.Context(), time.Minute, func() (bool, error) {
			return false, wantErr
		})
	})
	require.ErrorIs(t, err, wantErr)
}

func TestPollContextCancel(t *testing.T) {
	clock := newFakeClock()
	s := &Service{clock: clock}

	// context がキャンセルされたら即座に終了します。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var calls int
	err := s.poll(ctx, time.Minute, func() (bool, error) {
		calls++
		return false, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}

func TestPollInterval(t *testing.T) {
	clock := newFakeClock()
	s := &Service{clock: clock}

	// デフォルトのポーリング間隔で ticker を作ります。
	_, err := runWithTicks(t, s, func() (any, error) {
		return nil, s.poll(t.Context(), time.Minute, func() (bool, error) {
			return true, nil
		})
	})
	require.NoError(t, err)
	clock.mu.Lock()
	defer clock.mu.Unlock()
	require.Equal(t, DefaultPollInterval, clock.lastInterval)
}

func TestPollWaitBoundedByDeadline(t *testing.T) {
	t.Run("ポーリング間隔より短い期限でも期限を超えて待機しない", func(t *testing.T) {
		clock := newFakeClock()
		s := &Service{clock: clock}

		// 期限までの残り時間だけ待ってからタイムアウトします。
		var calls int
		_, err := runWithTicks(t, s, func() (any, error) {
			return nil, s.poll(t.Context(), time.Second, func() (bool, error) {
				calls++
				return false, nil
			})
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "タイムアウト")
		require.Equal(t, 2, calls)
		clock.mu.Lock()
		defer clock.mu.Unlock()
		require.Equal(t, time.Second, clock.lastInterval)
	})

	t.Run("期限が近づくと残り時間で待つ", func(t *testing.T) {
		clock := newFakeClock()
		s := &Service{clock: clock}

		// 期限 12 秒に対し、最後の待ちはポーリング間隔 (5 秒) ではなく残り 2 秒です。
		var calls int
		_, err := runWithTicks(t, s, func() (any, error) {
			return nil, s.poll(t.Context(), 12*time.Second, func() (bool, error) {
				calls++
				return false, nil
			})
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "タイムアウト")
		clock.mu.Lock()
		defer clock.mu.Unlock()
		require.Equal(t, 2*time.Second, clock.lastInterval)
	})
}
