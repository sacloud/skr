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
	"slices"
	"strings"
	"testing"
)

func newHierarchyScenario(t *testing.T, fake *fakeCLI) *scenario {
	t.Helper()
	s := testScenario(t, fake)
	if err := s.createFolder(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.createProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHierarchyScenarioMovesChildFolderAndProject(t *testing.T) {
	fake := newFakeCLI()
	s := newHierarchyScenario(t, fake)
	if err := s.hierarchyScenario(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"create-child-folder",
		"move-child-folder",
		"read-child-folder",
		"move-project",
		"read-project-moved",
	} {
		if !slices.Contains(fake.calls, want) {
			t.Errorf("hierarchy scenario did not call %s: %v", want, fake.calls)
		}
	}
	if got := fake.argsByStep["move-child-folder"]; !slices.Equal(got, []string{
		"iam-api", "folder", "move", "--ids", "3002", "--parent-id", "3001",
	}) {
		t.Errorf("move-child-folder args = %v", got)
	}
	if got := fake.argsByStep["move-project"]; !slices.Equal(got, []string{
		"iam-api", "project", "move", "--ids", "4001", "--parent-folder-id", "3002",
	}) {
		t.Errorf("move-project args = %v", got)
	}
}

func TestHierarchyScenarioDetectsUnexpectedChildFolderParent(t *testing.T) {
	fake := newFakeCLI()
	fake.childFolderParentID = 9999
	s := newHierarchyScenario(t, fake)
	if err := s.hierarchyScenario(context.Background()); err == nil {
		t.Fatal("hierarchy scenario accepted a child folder with an unexpected parent")
	} else if !strings.Contains(err.Error(), "parent_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHierarchyScenarioFailsWhenMoveIsRejected(t *testing.T) {
	fake := newFakeCLI()
	s := newHierarchyScenario(t, fake)
	fake.failAt = "move-child-folder"
	if err := s.hierarchyScenario(context.Background()); err == nil {
		t.Fatal("hierarchy scenario succeeded although the move was rejected")
	}
}
