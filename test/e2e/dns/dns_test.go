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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	dnsSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/dns"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// dnsZoneDescription はテスト用 DNS ゾーンに付ける固定の説明です。後片付けの識別にも使います。
	dnsZoneDescription = "Temporary DNS zone created by skr live E2E"
	// updatedDNSZoneDescription は更新確認中に使用する一時的な説明です。
	// 更新確認は説明を元に戻さないため、後片付けはこの説明も許容します。
	updatedDNSZoneDescription = "Temporary DNS zone updated by skr live E2E"
)

// dnsZoneItem は DNS ゾーンの参照結果です。
type dnsZoneItem struct {
	ID              types.ID
	Name            string
	Description     string
	Tags            []string
	Records         iaas.DNSRecords
	DNSNameServers  []string
	DNSZone         string
	MonitoringSuite *iaas.MonitoringSuiteLog
}

// decodeDNSZoneList は DNS ゾーンの一括検索の応答を解析します。
func decodeDNSZoneList(data []byte) ([]dnsZoneItem, error) {
	var items []dnsZoneItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode DNS zone list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of DNS zones")
	}
	return items, nil
}

// parseDNSZone は DNS ゾーンの参照応答を解析します。ID が空の場合はエラーを返します。
func parseDNSZone(data []byte) (dnsZoneItem, error) {
	var item dnsZoneItem
	if err := json.Unmarshal(data, &item); err != nil {
		return dnsZoneItem{}, fmt.Errorf("decode DNS zone: %w", err)
	}
	if item.ID.IsEmpty() {
		return dnsZoneItem{}, errors.New("DNS zone ID is empty")
	}
	return item, nil
}

// checkDNSZoneIdentity は参照結果が期待する DNS ゾーン (ID・ゾーン名・説明) と一致することを確認します。
// 元の識別情報と更新後の識別情報のどちらも、期待値を渡して同じ関数で確認します。
func checkDNSZoneIdentity(item dnsZoneItem, id types.ID, name, description string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("DNS zone identity mismatch for ID %s (expected name %q and description %q)", id, name, description)
	}
	return nil
}

// requireEqualDNSRecords はレコード一覧が期待する内容と一致することを確認します。
// 要素順序は API 応答で変わるため、名前・種別・値で並べ替えて比較します。
func requireEqualDNSRecords(t *testing.T, actual iaas.DNSRecords, expected []iaas.DNSRecord) {
	t.Helper()

	require.Len(t, actual, len(expected), "record count mismatch")
	sortedActual := sortedDNSRecords(dnsRecordValues(actual))
	sortedExpected := sortedDNSRecords(expected)
	for i := range sortedExpected {
		require.Equal(t, sortedExpected[i].Name, sortedActual[i].Name, "record %d name", i)
		require.Equal(t, sortedExpected[i].Type, sortedActual[i].Type, "record %d type", i)
		require.Equal(t, sortedExpected[i].RData, sortedActual[i].RData, "record %d rdata", i)
		require.Equal(t, sortedExpected[i].TTL, sortedActual[i].TTL, "record %d ttl", i)
	}
}

// dnsRecordValues は応答のレコード (ポインター) を値のコピーへ変換します。nil 要素は除外します。
func dnsRecordValues(records iaas.DNSRecords) []iaas.DNSRecord {
	values := make([]iaas.DNSRecord, 0, len(records))
	for _, record := range records {
		if record != nil {
			values = append(values, *record)
		}
	}
	return values
}

// sortedDNSRecords はレコード一覧を名前・種別・値で並べ替えたコピーを返します。
// 実際の応答と期待値の両方を受けられるよう、値型のスライスで受けます。
func sortedDNSRecords(records []iaas.DNSRecord) []iaas.DNSRecord {
	sorted := make([]iaas.DNSRecord, 0, len(records))
	for _, record := range records {
		if record.Name != "" || record.Type != "" || record.RData != "" {
			sorted = append(sorted, record)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return sorted[i].RData < sorted[j].RData
	})
	return sorted
}

// requireNoDNSZoneNameCollision は作成前に同名の DNS ゾーンが存在しないことを SDK で確認します。
// 確認の実行は証跡に残しません。既存リソースは変更しません。
func requireNoDNSZoneNameCollision(t *testing.T, dnss *dnsSDK.Service, name string) {
	t.Helper()

	matches, err := lookupDNSZones(t, dnss, name)
	require.NoError(t, err, "find existing DNS zones")
	require.Empty(t, matches, "test DNS zone name %q already exists; refusing to modify it", name)
}

// lookupDNSZones は SDK でゾーン名の部分一致検索を行い、結果を返します。
// 事前確認と後片付けの確認など、証跡に残さない安全確認で使います。
func lookupDNSZones(t *testing.T, dnss *dnsSDK.Service, name string) ([]dnsZoneItem, error) {
	t.Helper()

	found, err := dnss.FindWithContext(t.Context(), &dnsSDK.FindRequest{Names: []string{name}})
	if err != nil {
		return nil, fmt.Errorf("find DNS zones by name: %w", err)
	}
	items := make([]dnsZoneItem, 0, len(found))
	for _, zone := range found {
		if zone == nil {
			continue
		}
		items = append(items, dnsZoneItem{
			ID:             zone.ID,
			Name:           zone.Name,
			Description:    zone.Description,
			Records:        zone.Records,
			DNSNameServers: zone.DNSNameServers,
			DNSZone:        zone.DNSZone,
		})
	}
	return items, nil
}

// findDNSZones は skr CLI で DNS ゾーンを名前で検索し、確認に必要な項目へ絞り込んだ応答を返します。
func findDNSZones(t *testing.T, h *helper.Helper, name string) []dnsZoneItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "DNS ゾーンを名前で検索する",
		"dns-api", "find", "--name", name,
		"--query", "map({ID,Name,Description})",
	)
	require.NoError(t, err, "find DNS zone by name")
	items, err := decodeDNSZoneList(data)
	require.NoError(t, err)
	return items
}

// readDNSZone は skr CLI で DNS ゾーンを参照し、応答を返します。
func readDNSZone(t *testing.T, h *helper.Helper, id types.ID) dnsZoneItem {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "DNS ゾーンを参照する",
		"dns-api", "read", "--id", id.String(),
	)
	require.NoError(t, err, "read DNS zone")
	item, err := parseDNSZone(data)
	require.NoError(t, err)
	return item
}

// cleanupDNSZone は後片付けです。作成に成功した DNS ゾーンの ID を優先して SDK で再照合し、
// ID がない場合は名前検索で回収を試みます。識別できない場合は削除しません。
// 削除は skr CLI で行い、削除後は SDK で不在を確認します。
// 更新確認は説明を元に戻さないため、更新後の説明も許容します。
func cleanupDNSZone(t *testing.T, h *helper.Helper, dnss *dnsSDK.Service, name string, knownID types.ID) error {
	t.Helper()

	id := knownID
	if id.IsEmpty() {
		matches, err := lookupDNSZones(t, dnss, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created DNS zone: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() ||
			(matches[0].Description != dnsZoneDescription && matches[0].Description != updatedDNSZoneDescription) {
			return errors.New("cannot safely identify created DNS zone; manual cleanup required")
		}
		id = matches[0].ID
	}

	// 削除前に SDK で再照合し、保持した ID と許容する識別情報が一致する場合だけ削除する
	current, err := dnss.ReadWithContext(t.Context(), &dnsSDK.ReadRequest{ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify DNS zone before cleanup: %w", err)
	}
	if current.ID != id || current.Name != name ||
		(current.Description != dnsZoneDescription && current.Description != updatedDNSZoneDescription) {
		return errors.New("cleanup refused: DNS zone identity changed; manual cleanup required")
	}
	if _, _, err := h.CallAllowingFailure(t, "テスト用の DNS ゾーンを削除する",
		"dns-api", "delete", "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test DNS zone %s: %w", id, err)
	}
	matches, err := lookupDNSZones(t, dnss, name)
	if err != nil {
		return fmt.Errorf("verify DNS zone absence after cleanup: %w", err)
	}
	if len(matches) != 0 {
		return fmt.Errorf("DNS zone %q remains after cleanup; manual cleanup required", name)
	}
	return nil
}

// dnsRecordQueryName はレコード名をゾーン内の完全な名前にします。
func dnsRecordQueryName(zoneName, recordName string) string {
	if recordName == zoneName {
		return zoneName
	}
	return recordName + "." + zoneName
}
