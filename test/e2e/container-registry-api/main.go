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

// Container Registry API のライブ E2E runner です。
// 実行されるシナリオの流れは scenario_registry.go を参照してください。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sacloud/skr/test/e2e/internal/e2e"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-container-registry-live", false, "Confirm creating, updating, and deleting a uniquely named Container Registry and user")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/container-registry-api --skr ./skr --confirm-container-registry-live")
		return 2
	}
	path, err := e2e.ValidateExecutable(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	recorder, err := evidence.New("container-registry-api", evidence.Scenario{
		Title:       "コンテナレジストリとユーザーの CRUD",
		Description: "コンテナレジストリとそのユーザーを作成・参照・更新・削除し、リソースのライフサイクルが正しく動作することを確認します。",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())
	// 結果の保存は後片付けの後に実行されるため、後片付けの失敗も結果に反映されます。
	defer e2e.RecordResult(recorder, &exitCode)()

	tempDir, removeTempDir, err := e2e.TempDir("skr-e2e-container-registry-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := removeTempDir(); err != nil {
			fmt.Fprintln(os.Stderr, "remove private temporary directory:", err)
			exitCode = 1
		}
	}()

	passwordFile := filepath.Join(tempDir, "registry-password")
	passwordSuffix, err := e2e.RandomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(passwordFile, []byte(passwordSuffix), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write private E2E password file:", err)
		return 1
	}

	suffix, err := e2e.RandomSuffix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	name := registryPrefix + suffix
	description := descriptionPrefix + suffix
	fmt.Println("Unique registry to create:", name)
	fmt.Println("This scenario creates a registry and user, changes the description and permission, then deletes only those created resources.")
	fmt.Println("Check `skr config current` and the selected project before running.")

	ctx, stop := e2e.SignalContext()
	defer stop()

	runner := e2e.NewRunner(path, recorder, e2e.WithPrefix("container-registry-api"))
	s := scenario{
		client:       runner,
		requestDir:   tempDir,
		passwordFile: passwordFile,
	}
	if err := s.run(ctx, name, description); err != nil {
		fmt.Fprintln(os.Stderr, "Container Registry E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Container Registry E2E passed; the uniquely created registry and user were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
