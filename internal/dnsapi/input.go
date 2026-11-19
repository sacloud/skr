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
	"fmt"
	"slices"
	"strconv"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/dns"
	"github.com/sacloud/skr/internal/textjson"
)

// recordSpec は --record フラグの key=value 形式の仕様です。
var recordSpec = []textjson.Field{
	{Name: "name", Required: true},
	{Name: "type", Required: true},
	{Name: "rdata", Required: true},
	{Name: "ttl", Required: true},
}

// createRequestFromFlags は create の個別フラグから SDK リクエストを組み立てます。
// レコードは未指定でも空配列を送り、ゾーン作成後にレコードを持たない状態を明示します。
func createRequestFromFlags(c *CreateCommand) (*dns.CreateRequest, error) {
	records, err := parseDNSRecords(c.Records)
	if err != nil {
		return nil, err
	}
	if records == nil {
		// レコード未指定でも空配列を送り、レコードを持たないゾーン作成を明示します
		records = make(iaas.DNSRecords, 0)
	}
	request := &dns.CreateRequest{
		Name:    *c.Name,
		Records: records,
	}
	if c.Description != nil {
		request.Description = *c.Description
	}
	if c.Tags != nil {
		request.Tags = types.Tags(*c.Tags)
	}
	if c.IconID != nil {
		request.IconID = types.ID(*c.IconID)
	}
	if c.MonitoringSuiteLogEnabled != nil {
		request.MonitoringSuiteLog = &iaas.MonitoringSuiteLog{Enabled: *c.MonitoringSuiteLogEnabled}
	}
	return request, nil
}

// updateRequestFromFlags は update の個別フラグから SDK リクエストを組み立てます。
// 未指定の項目は nil のまま維持し、SDK のサービス層が現在値を引き継ぎます。
// SettingsHash はサービス層が現在値から設定するため、ここでは設定しません。
func updateRequestFromFlags(c *UpdateCommand) (*dns.UpdateRequest, error) {
	request := &dns.UpdateRequest{ID: types.ID(*c.ID)}
	if c.Description != nil {
		request.Description = c.Description
	}
	if c.Tags != nil {
		tags := types.Tags(*c.Tags)
		request.Tags = &tags
	}
	if c.IconID != nil {
		iconID := types.ID(*c.IconID)
		request.IconID = &iconID
	}
	if c.Records != nil {
		records, err := parseDNSRecords(c.Records)
		if err != nil {
			return nil, err
		}
		request.Records = records
	}
	if c.MonitoringSuiteLogEnabled != nil {
		request.MonitoringSuiteLog = &iaas.MonitoringSuiteLog{Enabled: *c.MonitoringSuiteLogEnabled}
	}
	return request, nil
}

// parseDNSRecords は --record フラグの反復入力を DNS レコード一覧に変換します。
// 入力が nil の場合は nil を返し、未指定であることを保持します。
func parseDNSRecords(inputs []string) (iaas.DNSRecords, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make(iaas.DNSRecords, 0, len(inputs))
	for i, input := range inputs {
		record, err := parseDNSRecord(input)
		if err != nil {
			return nil, fmt.Errorf("--record %d: %w", i+1, err)
		}
		result = append(result, &record)
	}
	return result, nil
}

// parseDNSRecord は 1 件の key=value 入力を DNS レコードに変換します。
func parseDNSRecord(input string) (iaas.DNSRecord, error) {
	fields, err := textjson.Parse(input, recordSpec)
	if err != nil {
		return iaas.DNSRecord{}, err
	}
	name := fields.Get("name")
	typeValue := fields.Get("type")
	rdata := fields.Get("rdata")
	ttlText := fields.Get("ttl")

	if !slices.Contains(types.DNSRecordTypeStrings, typeValue) {
		return iaas.DNSRecord{}, fmt.Errorf(
			"type %q は対応していません。指定可能な値: %s", typeValue, joinRecordTypes())
	}
	ttl, err := strconv.Atoi(ttlText)
	if err != nil || ttl < 0 {
		return iaas.DNSRecord{}, fmt.Errorf("ttl は 0 以上の整数で指定してください: %q", ttlText)
	}
	return iaas.DNSRecord{
		Name:  name,
		Type:  types.EDNSRecordType(typeValue),
		RData: rdata,
		TTL:   ttl,
	}, nil
}

// joinRecordTypes はヘルプやエラーメッセージ向けにレコード種別をカンマ区切りで返します。
func joinRecordTypes() string {
	result := ""
	for i, recordType := range types.DNSRecordTypeStrings {
		if i > 0 {
			result += ", "
		}
		result += recordType
	}
	return result
}
