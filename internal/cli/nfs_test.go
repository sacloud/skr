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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// nfs の実行テストは internal/nfs で service 層 (fake NoteFinder) とあわせて
// 行います。sakumock の IaaS モックは Note (sys-nfs) を含まないため、internal/cli
// では CLI 登録とヘルプの文言を確認します。

func TestRunNFSCommandHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"nfs", "--help"},
			want: []string{"plans", "nfs-api"},
		},
		{
			args: []string{"nfs", "plans", "--help"},
			want: []string{
				"--zone", "--all",
				"SSD", "HDD",
				"available", "disabled",
				"--plan-id",
			},
		},
		{
			args: []string{"nfs-api", "--help"},
			want: []string{
				"find", "read", "create", "update", "delete",
				"boot", "shutdown", "reset",
				"monitor-cpu", "monitor-free-disk-size", "monitor-interface",
			},
		},
		{
			args: []string{"nfs-api", "create", "--help"},
			want: []string{
				"--zone", "--name", "--plan-id", "--switch-id",
				"--ip-address", "--network-mask-len", "--default-route",
				"--description", "--tags", "--icon-id", "--request",
			},
		},
		{
			args: []string{"nfs-api", "shutdown", "--help"},
			want: []string{"--zone", "--id", "--no-wait", "--force"},
		},
		{
			args: []string{"nfs-api", "monitor-free-disk-size", "--help"},
			want: []string{"--zone", "--id", "--start", "--end", "00:00:00"},
		},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				require.Contains(t, help, text, "help output does not contain %q", text)
			}
		})
	}
}

func TestRunNFSPlansZoneRequired(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"nfs", "plans"}, &stdout, &stderr),
		"skr nfs plans without --zone should fail")
	require.Contains(t, stderr.String(), "--zone", "stderr: %s", stderr.String())
}
