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
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// updatedServerDescription is the temporary description used while verifying the update.
const updatedServerDescription = description + " (updated)"

// updatedServerName returns the temporary name used while verifying the update.
func updatedServerName(name string) string {
	return name + "-upd"
}

// updateAndVerify updates the created Server's Name and Description, verifies the
// reflection with Read, restores the original values, and verifies the restore so
// that the existing delete flow can identify the Server again.
func (s scenario) updateAndVerify(ctx context.Context, id types.ID, name string) error {
	if err := s.updateServer(ctx, "test-update", id, updatedServerName(name), updatedServerDescription); err != nil {
		return err
	}
	item, err := s.read(ctx, "test-read", id)
	if err != nil {
		return fmt.Errorf("read updated Server: %w", err)
	}
	if err := checkUpdatedServer(item, id, name); err != nil {
		return err
	}
	if err := s.updateServer(ctx, "test-update-restore", id, name, description); err != nil {
		return fmt.Errorf("restore Server Name and Description: %w", err)
	}
	item, err = s.read(ctx, "test-read", id)
	if err != nil {
		return fmt.Errorf("read restored Server: %w", err)
	}
	return checkServer(item, id, name)
}

func (s scenario) updateServer(ctx context.Context, step string, id types.ID, name, desc string) error {
	_, err := s.call(ctx, step, map[string]any{
		"Zone": zone, "ID": id, "Name": name, "Description": desc,
	})
	return err
}

func checkUpdatedServer(item serverItem, id types.ID, name string) error {
	if item.ID != id || item.Name != updatedServerName(name) || item.Description != updatedServerDescription {
		return fmt.Errorf("updated Server identity mismatch for ID %s (expected updated name and description)", id)
	}
	return nil
}
