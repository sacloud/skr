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
	"encoding/json"
	"fmt"
)

type appUser struct {
	Limit struct {
		ApplicationCount int `json:"application_count"`
	} `json:"limit"`
}

// userScenario は user の読み込みと、アカウントがまだサインアップしていない場合の user 作成を実行します。
// サインアップはアカウントレベルで不可逆なため、
// 既に AppRun user が存在する場合は作成をスキップします。
func (s *scenario) userScenario(ctx context.Context) error {
	createdData, createErr := s.call(ctx, "create-user", "user", "create")
	var signedUp appUser
	signedUpNow := createErr == nil
	if signedUpNow {
		if err := json.Unmarshal(createdData, &signedUp); err != nil {
			return fmt.Errorf("decode AppRun signup: %w", err)
		}
	} else {
		// アカウントは既にサインアップ済みの可能性が高いです。
		// 次の read で実際の状態を確認します。
		fmt.Println("AppRun user create returned an error; assuming the account already signed up.")
	}
	read, err := s.readUser(ctx, "read-user")
	if err != nil {
		if !signedUpNow {
			return fmt.Errorf("AppRun user read failed after the signup error: %w", err)
		}
		return err
	}
	if read.Limit.ApplicationCount <= 0 {
		return fmt.Errorf("AppRun user limit application_count = %d; want a positive value", read.Limit.ApplicationCount)
	}
	if signedUpNow && signedUp.Limit.ApplicationCount != read.Limit.ApplicationCount {
		return fmt.Errorf("AppRun signup limit application_count = %d, read returned %d",
			signedUp.Limit.ApplicationCount, read.Limit.ApplicationCount)
	}
	return nil
}

func (s *scenario) readUser(ctx context.Context, step string) (appUser, error) {
	data, err := s.call(ctx, step, "user", "read")
	if err != nil {
		return appUser{}, err
	}
	var result appUser
	if err := json.Unmarshal(data, &result); err != nil {
		return appUser{}, fmt.Errorf("%s: decode AppRun user: %w", step, err)
	}
	return result, nil
}
