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
)

func TestRunNetworkingSuiteAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"networking-suite-api", "--help"},
			want: []string{"subnet-group", "subnet", "interface-connection", "address", "JSON", "SAKURA_ENDPOINTS_NETWORKING_SUITE"},
		},
		{
			args: []string{"networking-suite-api", "subnet-group", "--help"},
			want: []string{"list", "read", "create", "update", "delete"},
		},
		{
			args: []string{"networking-suite-api", "subnet-group", "create", "--help"},
			want: []string{"--request", "--name", "--description", "--ipv4-address-range-cidr", "--region", "@path.json", "標準入力", "併用できません", "例: skr networking-suite-api subnet-group create"},
		},
		{
			args: []string{"networking-suite-api", "subnet", "create", "--help"},
			want: []string{"--zone", "--subnet-group-srn", "IPv4AddressRangeCIDR"},
		},
		{
			args: []string{"networking-suite-api", "interface-connection", "create", "--help"},
			want: []string{"--interface-srn", "--subnet-srn", "--ephemeral-ipv4-address", "自動割り当て", "srnv1:sakura-ZONE:sakura.iaas.interface:ID"},
		},
		{
			args: []string{"networking-suite-api", "address", "list", "--help"},
			want: []string{"<subnet-srn>", "--interface-connection-srn"},
		},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) exit code = %d; stderr: %s", test.args, code, stderr.String())
			}
			for _, text := range test.want {
				if !strings.Contains(stdout.String(), text) {
					t.Errorf("help output does not contain %q", text)
				}
			}
		})
	}
}
