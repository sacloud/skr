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

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

// SignalContext は SIGINT と SIGTERM を検知してキャンセルされる context を返します。
// runner の main で defer stop を呼び出してください。
func SignalContext() (ctx context.Context, stop context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// RecordResult は runner の終了時に evidence の結果を書き込む defer 用関数を返します。
// *exitCode が 0 のとき passed、それ以外のとき failed を記録します。
// 結果の書き込みに失敗した場合は、その失敗を *exitCode に反映します。
func RecordResult(recorder *evidence.Recorder, exitCode *int) func() {
	return func() {
		result := "failed"
		if *exitCode == 0 {
			result = "passed"
		}
		if err := recorder.SetResult(result, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			*exitCode = 1
		}
	}
}
