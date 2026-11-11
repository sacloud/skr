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
	"fmt"
	"time"
)

const (
	// DefaultWaitTimeout は待機系処理の既定のタイムアウトです。
	DefaultWaitTimeout = 5 * time.Minute
	// DefaultPollInterval は待機系処理の既定のポーリング間隔です。
	DefaultPollInterval = 5 * time.Second
)

// clock は待機処理で使う時刻と ticker の供給源です。
// 単体テストでは fake に差し替えて、実時間を使わずに待機を検証します。
type clock interface {
	Now() time.Time
	NewTicker(time.Duration) ticker
}

// ticker は一定間隔で時刻を通知します。
type ticker interface {
	C() <-chan time.Time
	Stop()
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTicker(d time.Duration) ticker {
	return realTicker{ticker: time.NewTicker(d)}
}

type realTicker struct{ ticker *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.ticker.C }

func (t realTicker) Stop() { t.ticker.Stop() }

// poll は check が完了 (true) を返すまで DefaultPollInterval 間隔で再試行します。
//
// 参照エラーなど check がエラーを返しても打ち切らず、期限まで再試行します。最初の
// check は待機せずに即座に行います。期限を超えた場合と context がキャンセルされた
// 場合はエラーを返します。timeout の零値は DefaultWaitTimeout として扱います。
func (s *Service) poll(ctx context.Context, timeout time.Duration, check func() (bool, error)) error {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	deadline := s.clock.Now().Add(timeout)
	tk := s.clock.NewTicker(DefaultPollInterval)
	defer tk.Stop()
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if !s.clock.Now().Before(deadline) {
			return fmt.Errorf("待機がタイムアウトしました (期限: %s)", timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("待機がキャンセルされました: %w", ctx.Err())
		case <-tk.C():
		}
	}
}
