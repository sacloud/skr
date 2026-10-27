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
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	zone        = "is1b"
	namePrefix  = "skr-e2e-disk-"
	description = "Temporary Disk for skr API E2E"
	diskPlanID  = 4
	diskSizeGB  = 20
)

func TestScenarioDisk(t *testing.T) {
	h := helper.New(t, "disk-api", "IaaS ディスクを作成・参照・更新・削除し、ディスクリソースのライフサイクルが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := namePrefix + suffix
	updatedName := name + "-updated"

	// 作成前に同名ディスクが存在しないことを確認する
	require.Empty(t, findDisks(t, h, "作成前に同名ディスクが存在しないことを確認する", name), "disk name %q already exists; refusing to modify it", name)
	require.Empty(t, findDisks(t, h, "作成前に更新後の同名ディスクが存在しないことを確認する", updatedName), "disk name %q already exists; refusing to modify it", updatedName)

	// ディスクを作成する
	data := h.Call(t, "ディスクを作成する",
		"iaas-api", "disk", "create",
		"--zone", zone,
		"--name", name,
		"--description", description,
		"--disk-plan-id", fmt.Sprintf("%d", diskPlanID),
		"--connection", "virtio",
		"--size-gb", fmt.Sprintf("%d", diskSizeGB))
	createdDisk := parseDisk(t, data)
	require.Equal(t, name, createdDisk.Name)
	require.Equal(t, description, createdDisk.Description)

	defer func() {
		// ディスクを削除し、一覧に残っていないことを確認する
		_ = h.Call(t, "ディスクを削除する", "iaas-api", "disk", "delete", "--zone", zone, "--id", createdDisk.ID.String())
		remaining := findDisks(t, h, "削除後にディスクが一覧に残っていないことを確認する", name)
		require.Empty(t, remaining, "disk %s should not remain after deletion", name)
	}()

	// ディスクを参照する
	data = h.Call(t, "ディスクを参照する", "iaas-api", "disk", "read", "--zone", zone, "--id", createdDisk.ID.String())
	readDisk := parseDisk(t, data)
	require.Equal(t, name, readDisk.Name)

	// ディスクを更新する
	data = h.Call(t, "ディスクを更新する", "iaas-api", "disk", "update", "--zone", zone, "--id", createdDisk.ID.String(), "--name", updatedName)
	updatedDisk := parseDisk(t, data)
	require.Equal(t, updatedName, updatedDisk.Name)

	// 更新後のディスクを参照する
	data = h.Call(t, "更新後のディスクを参照する", "iaas-api", "disk", "read", "--zone", zone, "--id", createdDisk.ID.String())
	readUpdatedDisk := parseDisk(t, data)
	require.Equal(t, updatedName, readUpdatedDisk.Name)
}

func findDisks(t *testing.T, h *helper.Helper, description, name string) []*iaas.Disk {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "disk", "find", "--zone", zone, "--name", name)
	var disks []*iaas.Disk
	err := json.Unmarshal(data, &disks)
	require.NoError(t, err, "%s: decode disks: %v", description, err)
	require.NotNil(t, disks, "%s: expected a JSON disk array", description)
	return disks
}

func parseDisk(t *testing.T, data []byte) iaas.Disk {
	t.Helper()

	var disk iaas.Disk
	err := json.Unmarshal(data, &disk)
	require.NoError(t, err, "decode disk: %v", err)
	require.NotZero(t, disk.ID, "disk ID is missing")
	return disk
}
