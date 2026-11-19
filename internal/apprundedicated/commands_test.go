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
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestParseClusterPorts(t *testing.T) {
	t.Run("指定なしは nil", func(t *testing.T) {
		ports, err := parseClusterPorts(nil)
		require.NoError(t, err)
		require.Nil(t, ports)
	})

	t.Run("1 ポートを変換", func(t *testing.T) {
		ports, err := parseClusterPorts([]string{"port=80,protocol=http"})
		require.NoError(t, err)
		require.Equal(t, []v1.ClusterLoadBalancerPort{
			{Port: 80, Protocol: v1.ClusterLoadBalancerPortProtocolHTTP},
		}, ports)
	})

	t.Run("複数のポートを指定順に変換", func(t *testing.T) {
		ports, err := parseClusterPorts([]string{
			"port=80,protocol=http",
			"port=443,protocol=https",
			"port=5432,protocol=tcp",
		})
		require.NoError(t, err)
		require.Equal(t, []v1.ClusterLoadBalancerPort{
			{Port: 80, Protocol: v1.ClusterLoadBalancerPortProtocolHTTP},
			{Port: 443, Protocol: v1.ClusterLoadBalancerPortProtocolHTTPS},
			{Port: 5432, Protocol: v1.ClusterLoadBalancerPortProtocolTCP},
		}, ports)
	})

	t.Run("エラー系", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			input string
			want  string
		}{
			{name: "key=value 形式でない", input: "80/http", want: "key=value 形式"},
			{name: "未対応のキー", input: "port=80,proto=http", want: "未対応のキー"},
			{name: "必須キーの欠落", input: "port=80", want: "必須キーがありません"},
			{name: "重複キー", input: "port=80,port=443,protocol=tcp", want: "重複して指定できません"},
			{name: "空の要素", input: "port=80,,protocol=tcp", want: "key=value 形式"},
			{name: "空の値", input: "port=,protocol=tcp", want: "key=value 形式"},
			{name: "port が数値でない", input: "port=abc,protocol=tcp", want: "1 ～ 65535 の整数"},
			{name: "port が 0", input: "port=0,protocol=tcp", want: "1 ～ 65535 の整数"},
			{name: "port が 65536", input: "port=65536,protocol=tcp", want: "1 ～ 65535 の整数"},
			{name: "protocol が列挙値でない", input: "port=80,protocol=grpc", want: "http、https、tcp のいずれか"},
		} {
			t.Run(test.name, func(t *testing.T) {
				_, err := parseClusterPorts([]string{test.input})
				require.Error(t, err)
				require.Contains(t, err.Error(), test.want)
			})
		}
	})
}
