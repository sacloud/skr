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

// updatedInternetDescription は更新確認中に使用される一時的な description である。
const updatedInternetDescription = internetDesc + " (updated)"

// updatedInternetName は更新確認中に使用される一時的な name を返す。
func updatedInternetName(name string) string {
	return name + "-upd"
}

// updateInternetAndVerify は、作成した Router + Switch の Name と Description を更新し、
// Read で反映を確認してから元の値に戻し、復元を確認します。
// これにより既存の削除フローが Internet を再び識別できるようになります。
func (s scenario) updateInternetAndVerify(ctx context.Context, id types.ID, name string, switchID types.ID) error {
	if err := s.updateInternet(ctx, "test-update-internet", id, updatedInternetName(name), updatedInternetDescription); err != nil {
		return err
	}
	item, err := s.readInternet(ctx, "test-read-internet", id)
	if err != nil {
		return fmt.Errorf("read updated Internet: %w", err)
	}
	if err := checkUpdatedInternet(item, id, name, switchID); err != nil {
		return err
	}
	if err := s.updateInternet(ctx, "test-update-internet-restore", id, name, internetDesc); err != nil {
		return fmt.Errorf("restore Internet Name and Description: %w", err)
	}
	item, err = s.readInternet(ctx, "test-read-internet", id)
	if err != nil {
		return fmt.Errorf("read restored Internet: %w", err)
	}
	return internetIdentity(item, id, name, switchID)
}

func (s scenario) updateInternet(ctx context.Context, step string, id types.ID, name, desc string) error {
	_, err := s.call(ctx, step, map[string]any{
		"Zone": zone, "ID": id, "Name": name, "Description": desc,
	})
	return err
}

func checkUpdatedInternet(item internetItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != updatedInternetName(name) || item.Description != updatedInternetDescription ||
		item.Switch.ID != switchID || item.NetworkMaskLen != 28 || item.BandWidthMbps != 100 {
		return fmt.Errorf("updated Internet identity mismatch for ID %s (expected updated name and description)", id)
	}
	return nil
}
