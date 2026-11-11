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
	"time"
)

const (
	// DefaultWaitTimeout は待機系処理の既定のタイムアウトです。
	DefaultWaitTimeout = 5 * time.Minute
	// DefaultPollInterval は待機系処理の既定のポーリング間隔です。
	DefaultPollInterval = 5 * time.Second
)

// clock は待機処理で使う時刻、ticker、待機期限付き context の供給源です。
// 単体テストでは fake に差し替えて、実時間を使わずに待機を検証します。
type clock interface {
	Now() time.Time
	NewTicker(time.Duration) ticker
	// WithTimeout は親 context に指定した期間の期限を持つ context を作ります。
	// 待機中の API 呼び出しに待機期限を適用するために使います。
	WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc)
}

// ticker は一定間隔で時刻を通知します。
type ticker interface {
	C() <-chan time.Time
	Stop()
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

func (realClock) NewTicker(d time.Duration) ticker {
	return realTicker{ticker: time.NewTicker(d)}
}

type realTicker struct{ ticker *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.ticker.C }

func (t realTicker) Stop() { t.ticker.Stop() }

// poll は check が完了 (true) を返すまで DefaultPollInterval 間隔で再試行します。
//
// check には待機期限を持つ context を渡します。check 内の API 呼び出しとリトライも
// 待機期限で打ち切られるため、API 呼び出しが長引いても待機期限を超えて待機しません。
//
// check がエラーを返した場合は打ち切ってエラーを返します。ただし待機 context が
// 完了している場合は、期限切れまたはキャンセルのエラーを優先して返します。参照エラー
// など打ち切りたくない一時的な失敗は、check が (false, nil) を返すことで期限まで
// 再試行できます。最初の check は待機せずに即座に行います。待機はポーリング間隔と
// 期限までの残り時間の短い方までとし、期限を超えて待機しません。期限を超えた場合と
// context がキャンセルされた場合はエラーを返します。timeout の零値は
// DefaultWaitTimeout として扱います。
func (s *Service) poll(ctx context.Context, timeout time.Duration, check func(ctx context.Context) (bool, error)) error {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	deadline := s.clock.Now().Add(timeout)
	// 待機中の API 呼び出しとリトライにも待機期限を適用します。
	ctx, cancel := s.clock.WithTimeout(ctx, timeout)
	defer cancel()
	tk := s.clock.NewTicker(DefaultPollInterval)
	defer tk.Stop()
	for {
		done, err := check(ctx)
		if err != nil {
			// API 呼び出しが待機期限や利用者によるキャンセルで打ち切られた場合は、
			// その理由を示すエラーを優先して返します。
			if ctxErr := ctx.Err(); ctxErr != nil {
				return waitContextError(ctxErr, timeout)
			}
			return err
		}
		if done {
			return nil
		}
		// 待ちに入る前にも完了を確認します。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return waitContextError(ctxErr, timeout)
		}
		now := s.clock.Now()
		if !now.Before(deadline) {
			return fmt.Errorf("待機がタイムアウトしました (期限: %s)", timeout)
		}
		// Ticker の間隔は作成後に変えられないため、期限までの残り時間がポーリング間隔
		// より短くなったら、残り時間で Ticker を作り直します。
		wait := DefaultPollInterval
		if remaining := deadline.Sub(now); remaining < wait {
			wait = remaining
		}
		tk.Stop()
		tk = s.clock.NewTicker(wait)
		select {
		case <-ctx.Done():
			return waitContextError(ctx.Err(), timeout)
		case <-tk.C():
		}
	}
}

// waitContextError は待機 context の状態を利用者向けのエラーに変換します。
func waitContextError(ctxErr error, timeout time.Duration) error {
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return fmt.Errorf("待機がタイムアウトしました (期限: %s): %w", timeout, ctxErr)
	}
	return fmt.Errorf("待機がキャンセルされました: %w", ctxErr)
}
