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
	"errors"
	"strings"
	"testing"
)

func TestUserScenarioVerifiesLimit(t *testing.T) {
	t.Run("signup on first use", func(t *testing.T) {
		fake := &fakeCLI{}
		s := testScenarioForAdditions(t, fake)
		if err := s.userScenario(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !containsStep(fake.calls, "create-user") || !containsStep(fake.calls, "read-user") {
			t.Errorf("scenario must call create-user and read-user: %v", fake.calls)
		}
	})
	t.Run("already signed up", func(t *testing.T) {
		fake := &fakeCLI{userLimit: 20}
		s := testScenarioForAdditions(t, fake)
		// オーバーライドで create-user を失敗させ、シナリオは「アカウントは既にサインアップ済み」として許容し、
		// read にフォールバックする
		s.client = &userOverrideCLI{base: fake}
		if err := s.userScenario(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !containsStep(fake.calls, "read-user") {
			t.Errorf("read must verify the state after the tolerated signup error: %v", fake.calls)
		}
	})
}

func TestUserScenarioReportsFailures(t *testing.T) {
	t.Run("read failure after signup error", func(t *testing.T) {
		fake := &fakeCLI{}
		s := testScenarioForAdditions(t, fake)
		s.client = &userOverrideCLI{base: fake, failRead: true}
		err := s.userScenario(context.Background())
		if err == nil || !strings.Contains(err.Error(), "after the signup error") {
			t.Fatalf("error = %v, want read failure after signup error", err)
		}
	})
	t.Run("zero limit", func(t *testing.T) {
		fake := &fakeCLI{userLimit: -1}
		s := testScenarioForAdditions(t, fake)
		err := s.userScenario(context.Background())
		if err == nil || !strings.Contains(err.Error(), "want a positive value") {
			t.Fatalf("error = %v, want positive value check failure", err)
		}
	})
}

// userOverrideCLI はサインアップを失敗させ、必要に応じて read-user の応答も失敗させつつ、
// それ以外はベースの fake に委譲する。
type userOverrideCLI struct {
	base     *fakeCLI
	failRead bool
}

func (o *userOverrideCLI) callRoot(ctx context.Context, step string, args ...string) ([]byte, error) {
	return o.base.callRoot(ctx, step, args...)
}

func (o *userOverrideCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if step == "create-user" {
		return nil, errors.New("injected signup failure")
	}
	data, err := o.base.call(ctx, step, args...)
	if err != nil {
		return data, err
	}
	if step == "read-user" && o.failRead {
		return nil, errors.New("injected read failure")
	}
	return data, nil
}
