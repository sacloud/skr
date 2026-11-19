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

package dnsapi

import (
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

func TestParseDNSRecords(t *testing.T) {
	t.Run("未指定は nil", func(t *testing.T) {
		records, err := parseDNSRecords(nil)
		require.NoError(t, err)
		require.Nil(t, records)
	})

	t.Run("1件の A レコード", func(t *testing.T) {
		records, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1,ttl=3600"})
		require.NoError(t, err)
		require.Equal(t, iaas.DNSRecords{&iaas.DNSRecord{
			Name:  "www",
			Type:  types.EDNSRecordType("A"),
			RData: "192.0.2.1",
			TTL:   3600,
		}}, records)
	})

	t.Run("複数件は指定順に保持する", func(t *testing.T) {
		records, err := parseDNSRecords([]string{
			"name=www,type=A,rdata=192.0.2.1,ttl=3600",
			"name=example.com,type=MX,rdata=10 mx.example.com.,ttl=3600",
			"name=example.com,type=TXT,rdata=skr-e2e-test,ttl=600",
		})
		require.NoError(t, err)
		require.Len(t, records, 3)
		require.Equal(t, types.EDNSRecordType("MX"), records[1].Type)
		require.Equal(t, "10 mx.example.com.", records[1].RData)
		require.Equal(t, "skr-e2e-test", records[2].RData)
		require.Equal(t, 600, records[2].TTL)
	})

	t.Run("必須キーの欠落", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "ttl")
	})

	t.Run("未対応のキー", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1,ttl=3600,priority=10"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "未対応のキー")
	})

	t.Run("重複キー", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,name=mail,type=A,rdata=192.0.2.1,ttl=3600"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "重複")
	})

	t.Run("対応していないレコード種別", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,type=DS,rdata=192.0.2.1,ttl=3600"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "対応していません")
	})

	t.Run("ttl は整数", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1,ttl=1h"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "0 以上の整数")
	})

	t.Run("ttl は負の値を拒否する", func(t *testing.T) {
		_, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1,ttl=-1"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "0 以上の整数")
	})

	t.Run("ttl 0 は受理する", func(t *testing.T) {
		records, err := parseDNSRecords([]string{"name=www,type=A,rdata=192.0.2.1,ttl=0"})
		require.NoError(t, err)
		require.Equal(t, 0, records[0].TTL)
	})
}
