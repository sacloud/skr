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

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	apprunmock "github.com/sacloud/sakumock/apprundedicated"
)

func TestRunAppRunDedicatedHighLevelHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"apprun-dedicated", "--help"},
			want: []string{"init", "deploy", "diff", "render", "status", "versions", "containers", "rollback", "deactivate", "delete", "create-auto-scaling-group", "delete-auto-scaling-group", "delete-cluster", "--app"},
		},
		{
			args: []string{"apprun-dedicated", "deploy", "--help"},
			want: []string{"--app=FILE|-", "--[no-]wait", "--wait-timeout", "5 分"},
		},
		{
			args: []string{"apprun-dedicated", "rollback", "--help"},
			want: []string{"--target=VERSION", "--force", "アクティブバージョンより前"},
		},
		{
			args: []string{"apprun-dedicated", "init", "--help"},
			want: []string{"--output-file", "application.json", "上書きしません"},
		},
		{
			args: []string{"apprun-dedicated", "diff", "--help"},
			want: []string{"unified diff", "registryPassword", "--query には対応せず"},
		},
		{
			args: []string{"apprun-dedicated", "delete", "--help"},
			want: []string{"--force", "--[no-]wait", "--wait-timeout"},
		},
		{
			args: []string{"apprun-dedicated", "create-auto-scaling-group", "--help"},
			want: []string{"--zone", "--worker-service-class-path", "--name-servers", "--min-nodes", "--max-nodes", "--[no-]wait", "--wait-timeout"},
		},
		{
			args: []string{"apprun-dedicated", "delete-auto-scaling-group", "--help"},
			want: []string{"--force", "--[no-]wait", "--wait-timeout"},
		},
		{
			args: []string{"apprun-dedicated", "delete-cluster", "--help"},
			want: []string{"--force", "--[no-]wait", "--wait-timeout"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help does not contain %q", text)
			}
		})
	}
}

func TestRunAppRunDedicatedCommandsWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	// 準備として、低レベルコマンドでクラスタを作成します。
	var createdCluster v1.ClusterIdentifier
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "create-cluster", "--request",
		`{"name":"cli-cluster","servicePrincipalID":"sp-000000000","ports":[{"port":443,"protocol":"https"}]}`), &createdCluster))

	definitionDir := t.TempDir()
	definitionPath := definitionDir + "/application.json"
	writeDefinition := func(image string) {
		t.Helper()
		definition := `{"cluster":"cli-cluster","name":"cli-app","image":"` + image + `","cpu":1000,"memory":1024,` +
			`"scalingMode":"manual","fixedScale":1,"registryUsername":"user",` +
			`"registryPassword":"SECRET-PASSWORD",` +
			`"env":[{"key":"API_TOKEN","value":"SECRET-TOKEN","secret":true}]},`
		require.NoError(t, os.WriteFile(definitionPath, []byte(strings.TrimSuffix(definition, ",")), 0600))
	}

	// deploy はアプリケーションとバージョンを作成して有効化します。
	writeDefinition("ghcr.io/example/app:v1")
	var deployed struct {
		Cluster            string `json:"cluster"`
		Application        string `json:"application"`
		ApplicationID      string `json:"applicationID"`
		Version            int32  `json:"version"`
		ApplicationCreated bool   `json:"applicationCreated"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "deploy", "--app", definitionPath, "--no-wait"), &deployed))
	require.Equal(t, "cli-cluster", deployed.Cluster)
	require.Equal(t, "cli-app", deployed.Application)
	require.NotEmpty(t, deployed.ApplicationID, "deploy should return the application ID")
	require.Equal(t, int32(1), deployed.Version)
	require.True(t, deployed.ApplicationCreated)
	// 出力に秘密情報の値は含めません (リクエスト本文への値の設定は service 層の wire テストで検証します)。
	deployedJSON := string(runCommand("apprun-dedicated", "status", "--app", definitionPath))
	require.NotContains(t, deployedJSON, "SECRET-PASSWORD", "status output must not contain registryPassword")
	require.NotContains(t, deployedJSON, "SECRET-TOKEN", "status output must not contain secret env value")

	// --force を指定しない変更系操作は、確認への応答がない場合は中止します
	// (go test の標準入力は /dev/null なので、すぐに EOF になります)。
	var stdoutAbort, stderrAbort bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "rollback", "--app", definitionPath}, &stdoutAbort, &stderrAbort))
	require.Contains(t, stderrAbort.String(), "中止")

	// status は状態を出力します。
	var status struct {
		Cluster     string `json:"cluster"`
		Application struct {
			ApplicationID string `json:"applicationID"`
			Name          string `json:"name"`
			ActiveVersion *int32 `json:"activeVersion"`
			DesiredCount  *int32 `json:"desiredCount"`
		} `json:"application"`
		ActiveVersionDetail *struct {
			Image string `json:"image"`
			CPU   int64  `json:"cpu"`
		} `json:"activeVersionDetail"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "status", "--app", definitionPath), &status))
	require.Equal(t, "cli-cluster", status.Cluster)
	require.Equal(t, "cli-app", status.Application.Name)
	require.NotNil(t, status.Application.ActiveVersion)
	require.Equal(t, int32(1), *status.Application.ActiveVersion)
	require.NotNil(t, status.ActiveVersionDetail)
	require.Equal(t, "ghcr.io/example/app:v1", status.ActiveVersionDetail.Image)

	// versions はバージョンの一覧を出力します。
	var versions []struct {
		Version int32  `json:"version"`
		Image   string `json:"image"`
		Active  bool   `json:"active"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "versions", "--app", definitionPath), &versions))
	require.Len(t, versions, 1)
	require.Equal(t, int32(1), versions[0].Version)
	require.True(t, versions[0].Active)

	// containers はノードごとの配置を出力します。mock の状態では配置が空でも、
	// 配列として出力されることを確認します。
	var containers []map[string]any
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "containers", "--app", definitionPath), &containers))

	// render は検証と正規化を行った定義を出力します。秘密情報の値は含めません。
	var rendered map[string]any
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "render", "--app", definitionPath), &rendered))
	require.Equal(t, "cli-cluster", rendered["cluster"])
	require.Equal(t, "ghcr.io/example/app:v1", rendered["image"])
	renderedJSON := string(runCommand("apprun-dedicated", "render", "--app", definitionPath))
	require.NotContains(t, renderedJSON, "SECRET-PASSWORD", "render output must not contain registryPassword")
	require.NotContains(t, renderedJSON, "SECRET-TOKEN", "render output must not contain secret env value")

	// 2 回目の deploy は既存アプリケーションに新しいバージョンを作成します。
	writeDefinition("ghcr.io/example/app:v2")
	var second struct {
		Version            int32 `json:"version"`
		ApplicationCreated bool  `json:"applicationCreated"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "deploy", "--app", definitionPath, "--no-wait"), &second))
	require.Equal(t, int32(2), second.Version)
	require.False(t, second.ApplicationCreated)

	// diff は一致しない定義との差分を出力します。
	writeDefinition("ghcr.io/example/app:v1")
	var stdoutDiff, stderrDiff bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "diff", "--app", definitionPath}, &stdoutDiff, &stderrDiff))
	require.Contains(t, stdoutDiff.String(), "-  \"image\": \"ghcr.io/example/app:v1\"", "diff output = %s", stdoutDiff.String())
	require.Contains(t, stdoutDiff.String(), "+  \"image\": \"ghcr.io/example/app:v2\"", "diff output = %s", stdoutDiff.String())
	require.Contains(t, stderrDiff.String(), "比較から除外")
	// 現状と一致する定義なら差分はありません。
	writeDefinition("ghcr.io/example/app:v2")
	var stdoutSame, stderrSame bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "diff", "--app", definitionPath}, &stdoutSame, &stderrSame))
	require.Empty(t, stdoutSame.String(), "diff output should be empty when definitions match")
	require.Contains(t, stderrSame.String(), "差分はありません")

	// init はアクティブバージョンから定義を復元します。秘密情報は含めません。
	initPath := definitionDir + "/restored.json"
	var stdoutInit, stderrInit bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "init", "cli-cluster", "cli-app", "--output-file", initPath}, &stdoutInit, &stderrInit))
	restored, err := os.ReadFile(initPath) // #nosec G304 -- テストが自分で書き出したパスを読み戻します。
	require.NoError(t, err)
	var restoredDef map[string]any
	require.NoError(t, json.Unmarshal(restored, &restoredDef))
	require.Equal(t, "cli-cluster", restoredDef["cluster"])
	require.Equal(t, "cli-app", restoredDef["name"])
	require.Equal(t, "ghcr.io/example/app:v2", restoredDef["image"])
	require.NotContains(t, string(restored), "SECRET-PASSWORD", "init output must not contain registryPassword")
	require.Contains(t, stderrInit.String(), initPath)
	// 既存のファイルは上書きしません。
	var stdoutAgain, stderrAgain bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "init", "cli-cluster", "cli-app", "--output-file", initPath}, &stdoutAgain, &stderrAgain))
	require.Contains(t, stderrAgain.String(), "既に存在")

	// rollback はアクティブバージョンより前のバージョンへ切り替えます (--force で確認を省略)。
	var rolledBack struct {
		Version int32 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "rollback", "--app", definitionPath, "--force", "--no-wait"), &rolledBack))
	require.Equal(t, int32(1), rolledBack.Version)

	// --target でバージョン番号を指定して切り替えられます。
	var targeted struct {
		Version int32 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "rollback", "--app", definitionPath, "--target", "2", "--force", "--no-wait"), &targeted))
	require.Equal(t, int32(2), targeted.Version)

	// deactivate はアクティブバージョンを解除します。
	var deactivated struct {
		DeactivatedVersion *int32 `json:"deactivatedVersion"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "deactivate", "--app", definitionPath, "--force", "--no-wait"), &deactivated))
	require.NotNil(t, deactivated.DeactivatedVersion)

	// delete はアプリケーションを削除します。出力はありません。
	var stdoutDelete, stderrDelete bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "delete", "--app", definitionPath, "--force", "--no-wait"}, &stdoutDelete, &stderrDelete))
	require.Empty(t, stdoutDelete.String(), "delete output should be empty")

	// 削除後の status は見つからないことを示します。
	var stdoutMissing, stderrMissing bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "status", "--app", definitionPath}, &stdoutMissing, &stderrMissing))
	require.Contains(t, stderrMissing.String(), "見つかりません")

	violations := server.SpecViolations()
	require.Len(t, violations, 0, "sakumock recorded OpenAPI response violations: %+v", violations)
}

func TestRunAppRunDedicatedInputErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", server.TestURL())

	definitionDir := t.TempDir()
	definitionPath := definitionDir + "/application.json"
	require.NoError(t, os.WriteFile(definitionPath, []byte(`{"cluster":"c1","name":"app1","image":"ghcr.io/example/app:v1","cpu":1000,"memory":1024,"scalingMode":"manual","fixedScale":1}`), 0600))

	runError := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.NotEqual(t, 0, run(args, &stdout, &stderr), "run(%v) should fail", args)
		return stderr.String()
	}

	t.Run("--app は必須です", func(t *testing.T) {
		message := runError("apprun-dedicated", "deploy", "--no-wait")
		require.Contains(t, message, "--app")
	})

	t.Run("定義ファイルが存在しない", func(t *testing.T) {
		message := runError("apprun-dedicated", "status", "--app", definitionDir+"/missing.json")
		require.Contains(t, message, "存在しません")
	})

	// --app にパスでなく秘密情報を含む値を誤って指定した場合も、その値は
	// エラーメッセージへ出力しません。
	t.Run("--app の読み込みエラーに入力値を含めない", func(t *testing.T) {
		message := runError("apprun-dedicated", "status", "--app", `{"registryPassword":"SECRET-VALUE"}`)
		require.Contains(t, message, "--app")
		require.NotContains(t, message, "SECRET-VALUE")
	})

	t.Run("未知のフィールドは拒否します", func(t *testing.T) {
		path := definitionDir + "/unknown-field.json"
		require.NoError(t, os.WriteFile(path, []byte(`{"cluster":"c1","name":"app1","unknown":1}`), 0600))
		message := runError("apprun-dedicated", "render", "--app", path)
		require.Contains(t, message, "解釈できません")
	})

	t.Run("末尾の余分なデータは拒否します", func(t *testing.T) {
		path := definitionDir + "/trailing-data.json"
		require.NoError(t, os.WriteFile(path, []byte(`{"cluster":"c1","name":"app1"}{}`), 0600))
		message := runError("apprun-dedicated", "render", "--app", path)
		require.Contains(t, message, "余分なデータ")
	})

	t.Run("deploy は定義の検証に失敗した操作を実行しません", func(t *testing.T) {
		path := definitionDir + "/no-image.json"
		require.NoError(t, os.WriteFile(path, []byte(`{"cluster":"c1","name":"app1","cpu":1000,"memory":1024,"scalingMode":"manual","fixedScale":1}`), 0600))
		message := runError("apprun-dedicated", "deploy", "--app", path)
		require.Contains(t, message, "image")
	})

	t.Run("rollback の --target は 0 以上の整数です", func(t *testing.T) {
		message := runError("apprun-dedicated", "rollback", "--app", definitionPath, "--target", "-1", "--force", "--no-wait")
		require.Contains(t, message, "--target")
	})

	t.Run("存在しないアプリケーションは見つからないことを示します", func(t *testing.T) {
		message := runError("apprun-dedicated", "status", "--app", definitionPath)
		require.Contains(t, message, "見つかりません")
	})

	t.Run("トレースが有効な deploy は実行しません", func(t *testing.T) {
		message := runError("--trace", "apprun-dedicated", "deploy", "--app", definitionPath)
		require.Contains(t, message, "トレースを無効にしてください")
	})

	t.Run("JSON 以外の出力のコマンドは --query を拒否します", func(t *testing.T) {
		for _, args := range [][]string{
			{"apprun-dedicated", "diff", "--app", definitionPath, "--query", ".name"},
			{"apprun-dedicated", "init", "c1", "app1", "--query", ".name"},
			{"apprun-dedicated", "delete", "--app", definitionPath, "--query", ".name"},
			{"apprun-dedicated", "delete-auto-scaling-group", "c1", "asg1", "--query", ".name"},
			{"apprun-dedicated", "delete-cluster", "c1", "--query", ".name"},
		} {
			message := runError(args...)
			require.Contains(t, message, "--query には対応しません")
		}
	})
}

func TestRunAppRunDedicatedInfrastructureCommandsWithSakumock(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	// 準備として、低レベルコマンドでクラスタを作成します。
	var createdCluster v1.ClusterIdentifier
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "create-cluster", "--request",
		`{"name":"infra-cluster","servicePrincipalID":"sp-000000000","ports":[{"port":443,"protocol":"https"}]}`), &createdCluster))
	// create-auto-scaling-group でオートスケーリンググループを作成します。
	var createdGroup struct {
		Cluster                string `json:"cluster"`
		Name                   string `json:"name"`
		AutoScalingGroupID     string `json:"autoScalingGroupID"`
		Zone                   string `json:"zone"`
		WorkerServiceClassPath string `json:"workerServiceClassPath"`
		MinNodes               int32  `json:"minNodes"`
		MaxNodes               int32  `json:"maxNodes"`
	}
	data := runCommand("apprun-dedicated", "create-auto-scaling-group", "infra-cluster", "infra-asg",
		"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
		"--name-servers", "210.188.224.10",
		"--min-nodes", "1", "--max-nodes", "1", "--no-wait")
	require.NoError(t, json.Unmarshal(data, &createdGroup))
	require.Equal(t, "infra-cluster", createdGroup.Cluster)
	require.Equal(t, "infra-asg", createdGroup.Name)
	require.NotEmpty(t, createdGroup.AutoScalingGroupID)
	require.Equal(t, "is1a", createdGroup.Zone)
	require.Equal(t, "cloud/apprun/dedicated/worker/1vcpu_2gb", createdGroup.WorkerServiceClassPath)
	require.EqualValues(t, 1, createdGroup.MinNodes)
	require.EqualValues(t, 1, createdGroup.MaxNodes)

	// 同名のグループを作成しようとすると、既存エラーになります。
	var stdoutDup, stderrDup bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "create-auto-scaling-group", "infra-cluster", "infra-asg",
		"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
		"--name-servers", "210.188.224.10",
		"--min-nodes", "1", "--max-nodes", "1", "--no-wait"}, &stdoutDup, &stderrDup))
	require.Contains(t, stderrDup.String(), "既に存在します")

	// delete-auto-scaling-group でグループを削除します。出力はありません。
	var stdoutDeleteASG, stderrDeleteASG bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "delete-auto-scaling-group", "infra-cluster", "infra-asg", "--force", "--no-wait"},
		&stdoutDeleteASG, &stderrDeleteASG))
	require.Empty(t, stdoutDeleteASG.String(), "delete-auto-scaling-group output should be empty")

	// 削除済みのグループを再度削除しようとすると、見つからないエラーになります。
	var stdoutMissingASG, stderrMissingASG bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "delete-auto-scaling-group", "infra-cluster", "infra-asg", "--force", "--no-wait"},
		&stdoutMissingASG, &stderrMissingASG))
	require.Contains(t, stderrMissingASG.String(), "見つかりません")

	// delete-cluster でクラスタを削除します。アプリケーションなどの依存リソースは残っていません。
	var stdoutDeleteCluster, stderrDeleteCluster bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "delete-cluster", "infra-cluster", "--force", "--no-wait"},
		&stdoutDeleteCluster, &stderrDeleteCluster))
	require.Empty(t, stdoutDeleteCluster.String(), "delete-cluster output should be empty")

	// 削除済みのクラスタを再度削除しようとすると、見つからないエラーになります。
	var stdoutMissingCluster, stderrMissingCluster bytes.Buffer
	require.NotEqual(t, 0, run([]string{"apprun-dedicated", "delete-cluster", "infra-cluster", "--force", "--no-wait"},
		&stdoutMissingCluster, &stderrMissingCluster))
	require.Contains(t, stderrMissingCluster.String(), "見つかりません")

	violations := server.SpecViolations()
	require.Len(t, violations, 0, "sakumock recorded OpenAPI response violations: %+v", violations)
}

// TestRunAppRunDedicatedWaitFlagForms は --no-wait の否定形への値指定が
// 既定 (待機あり) と同じ扱いで機能することを確認します。negatable な単一
// フラグは --wait / --no-wait / --no-wait=false のすべてを受け付けます。
func TestRunAppRunDedicatedWaitFlagForms(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "dummy-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "dummy-secret")
	server := apprunmock.NewTestServer(apprunmock.Config{})
	t.Cleanup(server.Close)
	t.Setenv("SAKURA_ENDPOINTS_APPRUN_DEDICATED", server.TestURL())

	runCommand := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
		return stdout.Bytes()
	}

	// 準備として、低レベルコマンドでクラスタを作成します。
	var createdCluster v1.ClusterIdentifier
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated-api", "create-cluster", "--request",
		`{"name":"wait-flag-cluster","servicePrincipalID":"sp-000000000","ports":[{"port":443,"protocol":"https"}]}`), &createdCluster))

	// --no-wait で作成します (待機を省略)。
	var createdGroup struct {
		AutoScalingGroupID string `json:"autoScalingGroupID"`
	}
	require.NoError(t, json.Unmarshal(runCommand("apprun-dedicated", "create-auto-scaling-group", "wait-flag-cluster", "asg-wait",
		"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
		"--name-servers", "210.188.224.10",
		"--min-nodes", "1", "--max-nodes", "1", "--no-wait"), &createdGroup))
	require.NotEmpty(t, createdGroup.AutoScalingGroupID, "create-auto-scaling-group should return the group ID")

	// --no-wait=false は --wait (既定) と同じ扱いで、削除後の不在確認まで行います。
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"apprun-dedicated", "delete-auto-scaling-group", "wait-flag-cluster", "asg-wait", "--force", "--no-wait=false"},
		&stdout, &stderr), "stderr: %s", stderr.String())
	require.Empty(t, stdout.String(), "delete-auto-scaling-group output should be empty")

	violations := server.SpecViolations()
	require.Len(t, violations, 0, "sakumock recorded OpenAPI response violations: %+v", violations)
}

func TestRunAppRunDedicatedInfrastructureInputErrors(t *testing.T) {
	runError := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.NotEqual(t, 0, run(args, &stdout, &stderr), "run(%v) should fail", args)
		return stderr.String()
	}

	t.Run("--min-nodes は 1 以上です", func(t *testing.T) {
		message := runError("apprun-dedicated", "create-auto-scaling-group", "c1", "asg1",
			"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
			"--name-servers", "210.188.224.10",
			"--min-nodes", "0", "--max-nodes", "1")
		require.Contains(t, message, "--min-nodes")
	})

	t.Run("--max-nodes は --min-nodes 以上です", func(t *testing.T) {
		message := runError("apprun-dedicated", "create-auto-scaling-group", "c1", "asg1",
			"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
			"--name-servers", "210.188.224.10",
			"--min-nodes", "2", "--max-nodes", "1")
		require.Contains(t, message, "--max-nodes")
	})

	t.Run("--name-servers は IPv4 アドレスです", func(t *testing.T) {
		message := runError("apprun-dedicated", "create-auto-scaling-group", "c1", "asg1",
			"--zone", "is1a", "--worker-service-class-path", "cloud/apprun/dedicated/worker/1vcpu_2gb",
			"--name-servers", "not-an-ip",
			"--min-nodes", "1", "--max-nodes", "1")
		require.Contains(t, message, "IPv4")
	})
}
