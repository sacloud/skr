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
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// serverInterface は Server の NIC の参照結果です。
type serverInterface struct {
	ID           types.ID `json:"ID"`
	MACAddress   string   `json:"MACAddress"`
	IPAddress    string   `json:"IPAddress"`
	SwitchID     types.ID `json:"SwitchID"`
	UpstreamType string   `json:"UpstreamType"`
}

// serverItem は Server の参照結果です。
type serverItem struct {
	ID             types.ID          `json:"ID"`
	Name           string            `json:"Name"`
	Description    string            `json:"Description"`
	InstanceStatus string            `json:"InstanceStatus"`
	Interfaces     []serverInterface `json:"Interfaces"`
}

// parseServerOutput は Server の参照応答を解析します。ID が空の場合はエラーを返します。
func parseServerOutput(data []byte) (serverItem, error) {
	var item serverItem
	if err := json.Unmarshal(data, &item); err != nil {
		return serverItem{}, fmt.Errorf("decode Server: %w", err)
	}
	if item.ID.IsEmpty() {
		return serverItem{}, errors.New("Server ID is empty")
	}
	return item, nil
}

// checkServerIdentity は参照結果が期待する Server (ID・名前・説明・NIC の接続先) と
// 一致することを確認します。
func checkServerIdentity(item serverItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != serverDesc {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	if len(item.Interfaces) != 1 || item.Interfaces[0].SwitchID != switchID {
		return errors.New("Server NIC is not connected to the expected Router + Switch")
	}
	return nil
}

// TestParseServerOutput は Server の参照応答の解析を確認します。
func TestParseServerOutput(t *testing.T) {
	item, err := parseServerOutput([]byte(`{"ID":34,"Name":"skr-e2e-internet-test","Description":"d","InstanceStatus":"down","Interfaces":[]}`))
	require.NoError(t, err)
	require.Equal(t, types.ID(34), item.ID)
	require.Equal(t, "skr-e2e-internet-test", item.Name)
	require.Empty(t, item.Interfaces)

	_, err = parseServerOutput([]byte(`{"Name":"no-id"}`))
	require.Error(t, err, "Server ID is required")

	_, err = parseServerOutput([]byte("{"))
	require.Error(t, err, "invalid JSON should fail")
}

// TestCheckServerIdentity は Server の識別情報の確認を確認します。
func TestCheckServerIdentity(t *testing.T) {
	item := serverItem{ID: 34, Name: "skr-e2e-internet-test", Description: serverDesc}
	item.Interfaces = []serverInterface{{SwitchID: 13, MACAddress: "02:00:00:00:00:01"}}

	require.NoError(t, checkServerIdentity(item, 34, "skr-e2e-internet-test", 13))

	wrongSwitch := item
	wrongSwitch.Interfaces = []serverInterface{{SwitchID: 99, MACAddress: "02:00:00:00:00:01"}}
	require.Error(t, checkServerIdentity(wrongSwitch, 34, "skr-e2e-internet-test", 13),
		"the NIC should be connected to the created Switch")

	sharedNIC := item
	sharedNIC.Interfaces = []serverInterface{{SwitchID: 99, MACAddress: "02:00:00:00:00:01", UpstreamType: "shared"}}
	require.Error(t, checkServerIdentity(sharedNIC, 34, "skr-e2e-internet-test", 13),
		"a Server on the shared segment should be refused")

	multipleNICs := item
	multipleNICs.Interfaces = []serverInterface{
		{SwitchID: 13, MACAddress: "02:00:00:00:00:01"},
		{SwitchID: 99, MACAddress: "02:00:00:00:00:02"},
	}
	require.Error(t, checkServerIdentity(multipleNICs, 34, "skr-e2e-internet-test", 13),
		"multiple NICs should be refused")

	wrongName := item
	wrongName.Name = "skr-e2e-internet-other"
	require.Error(t, checkServerIdentity(wrongName, 34, "skr-e2e-internet-test", 13),
		"the name should match the create request")
}
