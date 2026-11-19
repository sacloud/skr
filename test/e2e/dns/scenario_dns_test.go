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

package main

import (
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	dnsSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/dns"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// dnsZoneNamePrefix はテスト用 DNS ゾーンの名前の接頭辞です。ランダムな接尾辞とともに使い、
// 過去の実行のリソースと衝突しないようにします。DNS ゾーン名には、public suffix そのもの (TLD や
// Public Suffix List 掲載のサフィックス) と一致しない、実在 TLD 配下の名前を指定します。
// 文書用に予約されたドメイン (example.com など) や存在しない TLD は登録できません。
// 委譲しないため外部からは応答せず、DNS ゾーン自体は無料です。
const dnsZoneNamePrefix = "skr-e2e-"

// dnsZoneTLD はテスト用 DNS ゾーンに使う実在 gTLD です。配下のランダムな名前は public suffix と一致しないため登録できます。
const dnsZoneTLD = ".cloud"

// TestScenarioDNS は DNS E2E です。委譲されていないダミーゾーンを無料で作成し、
// レコード付きの作成・参照・検索・更新・削除と、権威 DNS サーバーからの実際の A レコード応答を確認します。
// テスト対象の操作は skr CLI と dig で実行し、事前確認と後片付けの確認は SDK で静かに実行します。
// 途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioDNS(t *testing.T) {
	h := helper.New(t, "dns-api", "DNS ゾーンの作成・参照・検索・更新・削除と、権威 DNS サーバーからの実際の A レコード応答が正しく動作することを確認する")

	// dig は DNS の実応答の確認に使います。環境にない場合は API 応答の照合だけで続行します
	digAvailable := true
	if _, err := exec.LookPath("dig"); err != nil {
		digAvailable = false
		t.Log("dig not found; skip the authoritative DNS response check")
	}

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	dnss := dnsSDK.New(client)

	name := dnsZoneNamePrefix + helper.RandomSuffix(t) + dnsZoneTLD

	// 作成前に同名の DNS ゾーンが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	requireNoDNSZoneNameCollision(t, dnss, name)

	// 後片付け: テスト用の DNS ゾーンを識別情報の再照合つきで削除する
	var zoneID types.ID
	defer func() {
		if err := cleanupDNSZone(t, h, dnss, name, zoneID); err != nil {
			t.Errorf("DNS zone cleanup failed: %v", err)
		}
	}()

	// レコード付きのゾーンを作成します。MX の値は字義どおり保持されるため、空白を含む値も指定できます
	data, _, err := h.CallAllowingFailure(t, "テスト用の DNS ゾーンを作成する",
		"dns-api", "create",
		"--name", name, "--description", dnsZoneDescription, "--tags", "skr-e2e",
		"--record", "name=www,type=A,rdata=192.0.2.10,ttl=3600",
		"--record", "name="+name+",type=MX,rdata=10 www."+name+".,ttl=3600",
		"--record", "name="+name+",type=TXT,rdata=skr-e2e-dns-test,ttl=3600",
		"--record", "name=docs,type=CNAME,rdata=www."+name+".,ttl=3600",
	)
	require.NoError(t, err, "create DNS zone")
	created, err := parseDNSZone(data)
	require.NoError(t, err)
	zoneID = created.ID
	require.NoError(t, checkDNSZoneIdentity(created, zoneID, name, dnsZoneDescription),
		"the create response should contain the created DNS zone")
	require.True(t, slicesContainsTag(created.Tags, "skr-e2e"), "the create response should contain the tag")
	// レコードの要素順序は API 応答で変わるため、内容で並べ替えて比較します
	requireEqualDNSRecords(t, created.Records, []iaas.DNSRecord{
		{Name: "www", Type: types.EDNSRecordType("A"), RData: "192.0.2.10", TTL: 3600},
		{Name: name, Type: types.EDNSRecordType("MX"), RData: "10 www." + name + ".", TTL: 3600},
		{Name: name, Type: types.EDNSRecordType("TXT"), RData: "skr-e2e-dns-test", TTL: 3600},
		{Name: "docs", Type: types.EDNSRecordType("CNAME"), RData: "www." + name + ".", TTL: 3600},
	})
	require.NotEmpty(t, created.DNSNameServers, "the create response should include the authoritative name servers")

	// dns-api find が作成したゾーンを見つけられることを確認します
	found := findDNSZones(t, h, name)
	require.Len(t, found, 1, "dns-api find --name should return only the created DNS zone")
	require.NoError(t, checkDNSZoneIdentity(found[0], zoneID, name, dnsZoneDescription),
		"dns-api find should return the created DNS zone")

	// dns-api read が作成済みのゾーンとレコード一覧を返すことを確認します
	read := readDNSZone(t, h, zoneID)
	require.NoError(t, checkDNSZoneIdentity(read, zoneID, name, dnsZoneDescription),
		"dns-api read should return the created DNS zone")
	requireEqualDNSRecords(t, read.Records, []iaas.DNSRecord{
		{Name: "www", Type: types.EDNSRecordType("A"), RData: "192.0.2.10", TTL: 3600},
		{Name: name, Type: types.EDNSRecordType("MX"), RData: "10 www." + name + ".", TTL: 3600},
		{Name: name, Type: types.EDNSRecordType("TXT"), RData: "skr-e2e-dns-test", TTL: 3600},
		{Name: "docs", Type: types.EDNSRecordType("CNAME"), RData: "www." + name + ".", TTL: 3600},
	})
	require.NotEmpty(t, read.DNSNameServers, "dns-api read should return the authoritative name servers")

	// 作成したレコードが権威 DNS サーバーから応答されることを確認します (補助確認)
	if digAvailable {
		waitForDNSRecord(t, h, read.DNSNameServers[0], dnsRecordQueryName(name, "www"), "A", "192.0.2.10")
	}

	// 説明とレコード一覧を更新します。--record を指定するとレコード一覧全体が置き換わります
	data, _, err = h.CallAllowingFailure(t, "DNS ゾーンの説明とレコード一覧を更新する",
		"dns-api", "update", "--id", zoneID.String(),
		"--description", updatedDNSZoneDescription,
		"--record", "name=www,type=A,rdata=192.0.2.20,ttl=600",
		"--record", "name=www6,type=AAAA,rdata=2001:db8::10,ttl=600",
		"--record", "name="+name+",type=TXT,rdata=skr-e2e-dns-updated,ttl=600",
	)
	require.NoError(t, err, "update DNS zone")
	updated, err := parseDNSZone(data)
	require.NoError(t, err)
	require.NoError(t, checkDNSZoneIdentity(updated, zoneID, name, updatedDNSZoneDescription),
		"the update should be reflected in the response")

	// dns-api read が更新済みのゾーンを返すことを確認します。更新前のレコードは残りません
	read = readDNSZone(t, h, zoneID)
	require.NoError(t, checkDNSZoneIdentity(read, zoneID, name, updatedDNSZoneDescription),
		"dns-api read should return the updated DNS zone")
	requireEqualDNSRecords(t, read.Records, []iaas.DNSRecord{
		{Name: "www", Type: types.EDNSRecordType("A"), RData: "192.0.2.20", TTL: 600},
		{Name: "www6", Type: types.EDNSRecordType("AAAA"), RData: "2001:db8::10", TTL: 600},
		{Name: name, Type: types.EDNSRecordType("TXT"), RData: "skr-e2e-dns-updated", TTL: 600},
	})

	// 更新した A レコードが権威 DNS サーバーから応答されることを確認します (補助確認)
	if digAvailable {
		waitForDNSRecord(t, h, read.DNSNameServers[0], dnsRecordQueryName(name, "www"), "A", "192.0.2.20")
	}

	// skr dns-api delete で削除し、後片付けの不在確認は SDK で行います
	if _, _, err := h.CallAllowingFailure(t, "テスト用の DNS ゾーンを削除する",
		"dns-api", "delete", "--id", zoneID.String(), "--fail-if-not-found",
	); err != nil {
		t.Errorf("delete test DNS zone %s: %v", zoneID, err)
	}
	matches, err := lookupDNSZones(t, dnss, name)
	require.NoError(t, err, "verify DNS zone absence after delete")
	require.Empty(t, matches, "the deleted DNS zone %q should not be found", name)
	zoneID = types.ID(0) // 削除済みなので後片付けの再削除を行わない
}

// slicesContainsTag はタグ一覧に指定したタグが含まれるかを返します。
func slicesContainsTag(tags types.Tags, want string) bool {
	return slices.Contains(tags, want)
}

// waitForDNSRecord は権威 DNS サーバーへの dig で、レコードが期待する値だけになるまで期限付きで待ちます。
// 作成・更新直後の反映待ちの間の問い合わせエラーは再試行し、期限を過ぎたらテストを失敗させます。
func waitForDNSRecord(t *testing.T, h *helper.Helper, nameServer, fqdn, recordType, value string) {
	t.Helper()

	const (
		waitTimeout = 5 * time.Minute
		polling     = 5 * time.Second
	)

	deadline := time.Now().Add(waitTimeout)
	for {
		answers, err := lookupDNSRecord(t, h, nameServer, fqdn, recordType)
		if err == nil && len(answers) == 1 && answers[0] == value {
			return
		}
		if time.Now().Add(polling).After(deadline) {
			t.Fatalf("DNS %s record for %s did not become exactly [%s] within %s: answers=%v error=%v",
				recordType, fqdn, value, waitTimeout, answers, err)
		}
		time.Sleep(polling)
	}
}
