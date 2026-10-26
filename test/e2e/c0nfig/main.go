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

// config コマンドのライブ E2E runner です。
// 実行されるシナリオの流れは scenario_profile.go を参照してください。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sacloud/skr/test/e2e/internal/e2e"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-config-live", false, "Confirm creating and editing a profile in a temporary profile directory")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/c0nfig --skr ./skr --confirm-config-live")
		return 2
	}
	path, err := e2e.ValidateExecutable(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	recorder, err := evidence.New("c0nfig", evidence.Scenario{
		Title: "config プロファイルの作成・切り替え・編集",
		Description: "一時的なプロファイルディレクトリで config コマンドの create / current / list / show / use / edit を実行し、" +
			"プロファイルの作成、現在プロファイルの切り替え、属性の編集が正しく反映されることを確認します。",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())
	// 結果の保存は後片付けの後に実行されるため、後片付けの失敗も結果に反映されます。
	defer e2e.RecordResult(recorder, &exitCode)()

	profileDir, removeProfileDir, err := e2e.TempDir("skr-e2e-config-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := removeProfileDir(); err != nil {
			fmt.Fprintln(os.Stderr, "remove temporary profile directory:", err)
			exitCode = 1
		}
	}()

	keyPath, err := writePrivateKey(profileDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println("This test uses a temporary profile directory and does not overwrite existing profiles.")
	fmt.Println("Profile directory:", profileDir)
	ctx, stop := e2e.SignalContext()
	defer stop()

	runner := e2e.NewRunner(path, recorder, e2e.WithEnv("SAKURA_PROFILE_DIR="+profileDir))
	s := scenario{client: runner, keyPath: keyPath}
	if err := s.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Config E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Config E2E passed; profiles were created, switched, shown, edited, and listed in isolation.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
