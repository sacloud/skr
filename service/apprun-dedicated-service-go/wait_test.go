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
		wantOK      bool
		wantTotal   int
		wantRunning int
	}{
		{
			name:        "対象バージョンのすべてのコンテナが running なら完了",
			placements:  placements([]v1.ApplicationCurrentContainer{running("c1", 2, "running"), running("c2", 2, "running")}),
			version:     2,
			wantOK:      true,
			wantTotal:   2,
			wantRunning: 2,
		},
		{
			name:       "別バージョンのコンテナは対象外",
			placements: placements([]v1.ApplicationCurrentContainer{running("c1", 1, "running")}),
			version:    2,
			wantOK:     false,
			wantTotal:  0,
		},
		{
			name:        "running 以外が混ざると未完了",
			placements:  placements([]v1.ApplicationCurrentContainer{running("c1", 2, "running"), running("c2", 2, "exited")}),
			version:     2,
			wantOK:      false,
			wantTotal:   2,
			wantRunning: 1,
		},
		{
			name:       "コンテナがない場合は未完了",
			placements: nil,
			version:    2,
			wantOK:     false,
			wantTotal:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			running, total := containersRunning(tc.placements, tc.version)
			require.Equal(t, tc.wantOK, total > 0 && running == total)
			require.Equal(t, tc.wantTotal, total)
			require.Equal(t, tc.wantRunning, running)
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
