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
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioWorkflowsSubscription は workflows-api のプランと契約 (サブスクリプション)
// 操作のライブ E2E です。プラン一覧と契約状態の参照を検証し、契約がない場合は最安プランで
// 契約を開始して契約状態を確認した上で解約します。
// 既存の契約が存在する場合は作成・解約を実行せず、参照だけを検証します。
func TestScenarioWorkflowsSubscription(t *testing.T) {
	h := helper.New(t, "workflows-api-subscription",
		"プラン一覧と契約状態の参照、および契約がない場合の契約開始と解約が正しく動作することを確認する")

	// プラン一覧を取得する
	plans := listWorkflowsPlans(t, h)
	for _, plan := range plans {
		require.NotZero(t, plan.ID, "plan should have an ID: %+v", plan)
		require.NotEmpty(t, plan.Name, "plan should have a name: %+v", plan)
	}

	current := readWorkflowsSubscription(t, h, "Workflows の契約状態を確認する")
	if current != nil {
		// 既存の契約は変更・解約しない。参照だけを検証して終了する
		t.Logf("Workflows の契約が既に存在します (planId=%d planName=%s)。create-subscription と delete-subscription は実行しません", current.PlanId, current.PlanName)
		return
	}

	// 契約を開始する。最安のプランを選ぶ。後片付けは ensure の前に defer で登録し、
	// 契約作成直後から検証が失敗しても解約されるようにする
	cheapest := cheapestWorkflowsPlan(t, plans)
	var subscriptionCleanup workflowsSubscriptionCleanup
	defer releaseWorkflowsSubscription(t, h, &subscriptionCleanup)
	ensureWorkflowsSubscription(t, h, &subscriptionCleanup)
	require.Equal(t, cheapest.ID, subscriptionCleanup.planID, "created plan should be the cheapest plan")

	// 契約状態を再確認する
	after := readWorkflowsSubscription(t, h, "契約開始後に契約状態を確認する")
	require.NotNil(t, after, "subscription should exist after create-subscription")
	require.Equal(t, cheapest.ID, after.PlanId, "current plan should be the plan created by this run")
	require.NotEmpty(t, after.ID, "current plan should have a subscription ID: %+v", after)
}
