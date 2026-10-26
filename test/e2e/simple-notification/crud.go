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
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type crudItem struct {
	ID          string         `json:"ID,omitempty"`
	Name        string         `json:"Name"`
	Description string         `json:"Description"`
	Tags        []string       `json:"Tags"`
	Settings    map[string]any `json:"Settings,omitempty"`
	Icon        any            `json:"Icon"`
}

type crudResponse struct {
	CommonServiceItem crudItem `json:"CommonServiceItem"`
}

type ownedItem struct {
	resource string
	item     crudItem
	attempts []crudItem
}

type crudScenario struct {
	scenario
}

func (s crudScenario) listItems(ctx context.Context, step, resource string) ([]crudItem, error) {
	data, err := s.call(ctx, step, nil, resource, "list",
		"--query", ".CommonServiceItems | map({ID,Name,Description,Tags,Settings,Icon})")
	if err != nil {
		return nil, err
	}
	var items []crudItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON resource array", step)
	}
	return items, nil
}

func (s crudScenario) readItem(ctx context.Context, step, resource, id string) (crudItem, error) {
	data, err := s.call(ctx, step, nil, resource, "read", id)
	if err != nil {
		return crudItem{}, err
	}
	var response crudResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return crudItem{}, fmt.Errorf("%s: decode resource: %w", step, err)
	}
	if response.CommonServiceItem.ID != id {
		return crudItem{}, fmt.Errorf("%s: resource ID mismatch", step)
	}
	return response.CommonServiceItem, nil
}

func (s crudScenario) writeItem(ctx context.Context, step, resource, operation string, item crudItem) (crudItem, error) {
	return s.writeItemBeforeCall(ctx, step, resource, operation, item, nil)
}

func (s crudScenario) writeItemBeforeCall(ctx context.Context, step, resource, operation string, item crudItem, beforeCall func()) (crudItem, error) {
	id := item.ID
	item.ID = ""
	data, err := json.Marshal(crudResponse{CommonServiceItem: item})
	if err != nil {
		return crudItem{}, err
	}
	return s.writeRequestBeforeCall(ctx, step, resource, operation, id, data, beforeCall)
}

func (s crudScenario) writeRequest(ctx context.Context, step, resource, operation, id string, data []byte) (crudItem, error) {
	return s.writeRequestBeforeCall(ctx, step, resource, operation, id, data, nil)
}

func (s crudScenario) writeRequestBeforeCall(ctx context.Context, step, resource, operation, id string, data []byte, beforeCall func()) (crudItem, error) {
	if err := os.WriteFile(s.requestPath, data, 0o600); err != nil {
		return crudItem{}, fmt.Errorf("write private request file: %w", err)
	}
	args := []string{resource, operation}
	if id != "" {
		args = append(args, id)
	}
	args = append(args, "--request", "@"+s.requestPath)
	if beforeCall != nil {
		beforeCall()
	}
	output, err := s.call(ctx, step, evidence.RequestFileInput{Path: s.requestPath, Content: string(data)}, args...)
	if err != nil {
		return crudItem{}, err
	}
	if operation == "reorder" {
		var result struct {
			IsOK bool `json:"is_ok"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			return crudItem{}, fmt.Errorf("%s: decode reorder response: %w", step, err)
		}
		if !result.IsOK {
			return crudItem{}, errors.New("routing reorder was not accepted")
		}
		return crudItem{}, nil
	}
	var response crudResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return crudItem{}, fmt.Errorf("%s: decode resource response: %w", step, err)
	}
	return response.CommonServiceItem, nil
}

func matchesCRUDItem(actual, expected crudItem) bool {
	actualTags, expectedTags := slices.Clone(actual.Tags), slices.Clone(expected.Tags)
	slices.Sort(actualTags)
	slices.Sort(expectedTags)
	if actual.ID != expected.ID || actual.Name != expected.Name || actual.Description != expected.Description ||
		!slices.Equal(actualTags, expectedTags) {
		return false
	}
	for key, value := range expected.Settings {
		if !reflect.DeepEqual(actual.Settings[key], value) {
			return false
		}
	}
	return true
}

func matchesExactCRUDItem(actual, expected crudItem) bool {
	return matchesCRUDItem(actual, expected) &&
		reflect.DeepEqual(actual.Settings, expected.Settings) &&
		reflect.DeepEqual(actual.Icon, expected.Icon)
}

func (s crudScenario) verifyItem(ctx context.Context, step, resource string, expected crudItem) error {
	for attempt := 1; attempt <= 10; attempt++ {
		actual, err := s.readItem(ctx, fmt.Sprintf("%s-%02d", step, attempt), resource, expected.ID)
		if err != nil {
			return err
		}
		if matchesCRUDItem(actual, expected) {
			return nil
		}
		if attempt < 10 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return fmt.Errorf("%s: resource identity or settings mismatch (see private evidence)", step)
}

func (s crudScenario) cleanupItems(items []*ownedItem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var cleanupErr error
	for i := len(items) - 1; i >= 0; i-- {
		owned := items[i]
		if owned.item.ID == "" {
			return errors.Join(cleanupErr, errors.New("cleanup refused: created resource ID unavailable; manual cleanup required"))
		}
		if err := s.verifyOwnedItem(ctx, owned); err != nil {
			return errors.Join(cleanupErr, fmt.Errorf("cleanup refused: %w", err))
		}
		_, deleteErr := s.call(ctx, "crud-cleanup-delete-"+owned.resource, nil, owned.resource, "delete", owned.item.ID)
		remaining, listErr := s.listItems(ctx, "crud-cleanup-list-"+owned.resource, owned.resource)
		if listErr != nil {
			return errors.Join(cleanupErr, deleteErr, listErr)
		}
		for _, item := range remaining {
			if item.ID == owned.item.ID || item.Name == owned.item.Name {
				return errors.Join(cleanupErr, deleteErr, fmt.Errorf("cleanup failed: temporary %s remains", owned.resource))
			}
		}
		cleanupErr = errors.Join(cleanupErr, deleteErr)
	}
	return cleanupErr
}

func (s crudScenario) verifyOwnedItem(ctx context.Context, owned *ownedItem) error {
	actual, err := s.readItem(ctx, "crud-cleanup-read-"+owned.resource, owned.resource, owned.item.ID)
	if err != nil {
		return err
	}
	for _, expected := range append([]crudItem{owned.item}, owned.attempts...) {
		if matchesCRUDItem(actual, expected) {
			return nil
		}
	}
	return errors.New("resource identity or settings do not match a known state of this E2E run")
}

func (s crudScenario) runCRUD(ctx context.Context, suffix string) (result error) {
	profile, err := s.scenario.selectedProfile(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Selected SDK profile: %s", profile)
	if !strings.HasSuffix(profile, "\n") {
		fmt.Println()
	}
	// 書き込み前に既存のルーティング設定を取得し、後でその優先順位を確認できるようにする。
	before, err := s.listItems(ctx, "crud-routing-before", "routing")
	if err != nil {
		return err
	}
	for _, item := range before {
		rank, ok := item.Settings["PriorityRank"].(float64)
		if !ok || rank >= 99 {
			return errors.New("CRUD preflight refused: priority ranks 99 and 100 must be unused")
		}
	}
	sourceData, err := s.call(ctx, "crud-routing-sources", nil, "routing", "list-source")
	if err != nil {
		return err
	}
	var sources struct {
		Sources []struct {
			ID string `json:"id"`
		} `json:"Sources"`
	}
	if err := json.Unmarshal(sourceData, &sources); err != nil {
		return err
	}
	if len(sources.Sources) == 0 || sources.Sources[0].ID == "" {
		return errors.New("CRUD preflight refused: no notification source available")
	}
	source, err := s.readItem(ctx, "crud-source-destination", "destination", s.destinationID)
	if err != nil {
		return err
	}
	address, ok := source.Settings["Value"].(string)
	if source.Settings["Type"] != "email" || !ok || address == "" {
		return errors.New("CRUD requires an existing email destination")
	}

	name := "skr-e2e-crud-" + suffix
	description := "skr-e2e-simple-notification-crud/" + suffix
	var owned []*ownedItem
	defer func() { result = errors.Join(result, s.cleanupItems(owned)) }()

	create := func(resource string, settings map[string]any) (*ownedItem, error) {
		listed, err := s.listItems(ctx, "crud-preflight-"+resource, resource)
		if err != nil {
			return nil, err
		}
		for _, item := range listed {
			if item.Name == name {
				return nil, fmt.Errorf("CRUD %s name already exists", resource)
			}
		}
		item := crudItem{Name: name, Description: description, Tags: []string{"skr-e2e", suffix}, Settings: settings}
		entry := &ownedItem{resource: resource, item: item}
		created, err := s.writeItemBeforeCall(ctx, "crud-create-"+resource, resource, "create", item, func() {
			owned = append(owned, entry)
		})
		if err != nil {
			return nil, err
		}
		entry.item.ID = created.ID
		if resource == "routing" {
			rank, ok := created.Settings["PriorityRank"].(float64)
			if !ok || rank < 1 || rank > 100 {
				return nil, errors.New("routing create response has invalid priority rank")
			}
			entry.item.Settings["PriorityRank"] = rank
		}
		if created.ID == "" || !matchesCRUDItem(created, entry.item) {
			return nil, fmt.Errorf("CRUD %s create response mismatch", resource)
		}
		if err := s.verifyItem(ctx, "crud-read-created-"+resource, resource, entry.item); err != nil {
			return nil, err
		}
		createdList, err := s.listItems(ctx, "crud-list-created-"+resource, resource)
		if err != nil {
			return nil, err
		}
		found := false
		for _, actual := range createdList {
			if matchesCRUDItem(actual, entry.item) {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("created %s is missing from list", resource)
		}
		return entry, nil
	}
	update := func(entry *ownedItem, settings map[string]any, step string) error {
		item := entry.item
		item.Settings = settings
		item.Tags = []string{"skr-e2e", suffix, "updated"}
		expected := item
		if settings == nil {
			expected.Settings = entry.item.Settings
		}
		entry.attempts = []crudItem{entry.item, expected}
		_, err := s.writeItem(ctx, step, entry.resource, "update", item)
		if err != nil {
			return err
		}
		entry.item.Tags = item.Tags
		if settings != nil {
			entry.item.Settings = settings
		}
		if err := s.verifyItem(ctx, step+"-read", entry.resource, entry.item); err != nil {
			return err
		}
		entry.attempts = nil
		return nil
	}

	destination, err := create("destination", map[string]any{"Type": "email", "Value": address, "Disabled": true})
	if err != nil {
		return err
	}
	if err := update(destination, nil, "crud-destination-metadata"); err != nil {
		return err
	}
	if err := update(destination, map[string]any{"Type": "email", "Value": address, "Disabled": false}, "crud-destination-enable"); err != nil {
		return err
	}
	if err := update(destination, map[string]any{"Type": "email", "Value": address, "Disabled": true}, "crud-destination-disable"); err != nil {
		return err
	}
	if err := update(destination, destination.item.Settings, "crud-destination-settings"); err != nil {
		return err
	}
	status, err := s.call(ctx, "crud-destination-status", nil, "destination", "read-status", destination.item.ID)
	if err != nil {
		return err
	}
	var statusResponse struct {
		NotificationStatus *struct {
			IsValid *bool `json:"IsValid"`
		} `json:"NotificationStatus"`
	}
	if err := json.Unmarshal(status, &statusResponse); err != nil {
		return err
	}
	if statusResponse.NotificationStatus == nil || statusResponse.NotificationStatus.IsValid == nil {
		return errors.New("destination status response is missing IsValid")
	}
	group, err := create("group", map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": true})
	if err != nil {
		return err
	}
	if err := update(group, nil, "crud-group-metadata"); err != nil {
		return err
	}
	if err := update(group, map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": false}, "crud-group-enable"); err != nil {
		return err
	}
	if err := update(group, map[string]any{"Destinations": []any{destination.item.ID}, "Disabled": true}, "crud-group-disable"); err != nil {
		return err
	}
	if err := update(group, group.item.Settings, "crud-group-settings"); err != nil {
		return err
	}
	routingSettings := map[string]any{
		"SourceID": sources.Sources[0].ID, "TargetGroupID": group.item.ID,
		"PriorityRank": float64(100),
		"MatchLabels":  []any{map[string]any{"Name": "skr-e2e", "Value": suffix}},
	}
	routing, err := create("routing", routingSettings)
	if err != nil {
		return err
	}
	if err := update(routing, nil, "crud-routing-metadata"); err != nil {
		return err
	}
	updatedRouting := map[string]any{
		"SourceID": sources.Sources[0].ID, "TargetGroupID": group.item.ID,
		"PriorityRank": routing.item.Settings["PriorityRank"],
		"MatchLabels":  []any{map[string]any{"Name": "skr-e2e", "Value": suffix + "-updated"}},
	}
	if err := update(routing, updatedRouting, "crud-routing-match-label"); err != nil {
		return err
	}
	if err := update(routing, routing.item.Settings, "crud-routing-settings"); err != nil {
		return err
	}
	for _, rank := range []int{99, 100} {
		data, err := json.Marshal(map[string]any{"Orders": []any{map[string]any{"RoutingID": routing.item.ID, "PriorityRank": rank}}})
		if err != nil {
			return err
		}
		step := fmt.Sprintf("crud-routing-reorder-%d", rank)
		expected := routing.item
		expected.Settings = maps.Clone(routing.item.Settings)
		expected.Settings["PriorityRank"] = float64(rank)
		routing.attempts = []crudItem{routing.item, expected}
		if _, err := s.writeRequest(ctx, step, "routing", "reorder", "", data); err != nil {
			return err
		}
		routing.item = expected
		if err := s.verifyItem(ctx, step+"-read", "routing", routing.item); err != nil {
			return err
		}
		routing.attempts = nil
	}
	after, err := s.listItems(ctx, "crud-routing-after", "routing")
	if err != nil {
		return err
	}
	var existing []crudItem
	for _, item := range after {
		if item.ID != routing.item.ID {
			existing = append(existing, item)
		}
	}
	if len(existing) != len(before) {
		return errors.New("existing routing list changed during CRUD test")
	}
	for _, expected := range before {
		found := false
		for _, actual := range existing {
			if matchesExactCRUDItem(actual, expected) {
				found = true
			}
		}
		if !found {
			return errors.New("existing routing identity or settings changed during CRUD test")
		}
	}
	actualSource, err := s.readItem(ctx, "crud-source-destination-unchanged", "destination", source.ID)
	if err != nil {
		return err
	}
	if !matchesExactCRUDItem(actualSource, source) {
		return errors.New("source destination identity or settings changed during CRUD test")
	}
	return nil
}
