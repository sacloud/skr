// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type site struct {
	ID string `json:"id"`
}

type plan struct {
	Path      string `json:"service_class_path"`
	Type      string `json:"type"`
	ClusterID string `json:"cluster_id"`
}

func decodeArray[T any](output []byte, label string) ([]T, error) {
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "[") {
		return nil, fmt.Errorf("%s must be a JSON array", label)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	items := make([]T, len(values))
	for i, value := range values {
		if !strings.HasPrefix(strings.TrimSpace(string(value)), "{") {
			return nil, fmt.Errorf("%s item %d must be a JSON object", label, i)
		}
		if err := json.Unmarshal(value, &items[i]); err != nil {
			return nil, fmt.Errorf("decode %s item %d: %w", label, i, err)
		}
	}
	return items, nil
}

func validateOptions(mode, siteID, plan string, bucketConfirmed, accountConfirmed bool) error {
	if mode == "metrics" {
		if plan != "" || (accountConfirmed && !bucketConfirmed) {
			return errors.New("metrics accepts no --plan; creating an account also requires --confirm-create-bucket-live")
		}
		return validateSiteID(siteID)
	}
	if mode == "account" {
		if !accountConfirmed || plan != "" || bucketConfirmed {
			return errors.New("account scenario requires --confirm-create-account-live; no --plan or bucket creation confirmation")
		}
		if siteID == "arc02" {
			return nil
		}
		return validateSiteID(siteID)
	}
	if !bucketConfirmed {
		return errors.New("bucket scenarios require --confirm-create-bucket-live")
	}
	switch mode {
	case "tutorial", "encryption", "replication":
		if plan != "" {
			return errors.New("--plan is only supported by the archive scenario")
		}
		return validateSiteID(siteID)
	case "archive":
		if siteID != "arc02" || plan == "" || strings.ContainsAny(plan, "/ \t\r\n") {
			return errors.New("archive scenario requires --site-id arc02 and a single --plan value from site plans")
		}
		return nil
	default:
		return fmt.Errorf("unknown scenario %q", mode)
	}
}

func (s scenario) checkSite(ctx context.Context) error {
	output, err := s.client.call(ctx, "site-list", "利用可能なオブジェクトストレージサイト一覧を取得する", "object-storage-api", "site", "list",
		"--query", "map({id,display_name,region,plan_family})")
	if err != nil {
		return err
	}
	sites, err := decodeArray[site](output, "sites")
	if err != nil {
		return err
	}
	matches := 0
	for _, item := range sites {
		if item.ID == s.siteID {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("expected exactly one site %q, got %d", s.siteID, matches)
	}
	output, err = s.client.call(ctx, "site-read", "指定サイトの詳細を確認する", "object-storage-api", "site", "read", s.siteID)
	if err != nil {
		return err
	}
	var selected site
	if err := json.Unmarshal(output, &selected); err != nil {
		return fmt.Errorf("decode site: %w", err)
	}
	if selected.ID != s.siteID {
		return errors.New("site read identity does not match the requested site")
	}
	if s.plan == "" {
		return nil
	}
	output, err = s.client.call(ctx, "site-plans", "指定サイトで利用可能なプラン一覧を取得する", "object-storage-api", "site", "plans", s.siteID,
		"--query", "map({service_class_path,type,cluster_id,capacity_gib,fee})")
	if err != nil {
		return err
	}
	plans, err := decodeArray[plan](output, "plans")
	if err != nil {
		return err
	}
	for _, item := range plans {
		if item.ClusterID == s.siteID && item.Type == "archive" && item.Path == s.planPath() {
			return nil
		}
	}
	return errors.New("requested archive plan was not present in the site's plan list")
}

func (s scenario) planPath() string {
	path := "objectstorage/" + s.siteID + "/bucket"
	if s.plan != "" {
		path += "/" + s.plan
	}
	return path
}

func (s scenario) checkBucketPlan(item bucket) error {
	var actual plan
	if err := json.Unmarshal(item.Plan, &actual); err != nil {
		return fmt.Errorf("decode created bucket plan: %w", err)
	}
	wantType := "standard"
	if s.plan != "" {
		wantType = "archive"
	}
	if actual.Path != s.planPath() || actual.Type != wantType {
		return errors.New("created bucket plan does not match the requested plan")
	}
	return nil
}

type bucketControl struct {
	BucketName string `json:"bucket_name"`
	CanRead    bool   `json:"can_read"`
	CanWrite   bool   `json:"can_write"`
}

type permission struct {
	ID          int64           `json:"id,omitempty"`
	DisplayName string          `json:"display_name"`
	Controls    []bucketControl `json:"bucket_controls"`
}

type accessKey struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func (s scenario) permissions(ctx context.Context, step, description string) ([]permission, error) {
	output, err := s.client.call(ctx, step, description, "object-storage-api", "permission", "list", s.siteID,
		"--query", "map({id,display_name,bucket_controls})")
	if err != nil {
		return nil, err
	}
	return decodeArray[permission](output, "permission list")
}

func (s scenario) runPermission(ctx context.Context, name string) (result error) {
	dir, err := os.MkdirTemp("", "skr-object-storage-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(dir)) }()
	expected := permission{DisplayName: name, Controls: []bucketControl{{BucketName: name, CanRead: true, CanWrite: true}}}
	items, err := s.permissions(ctx, "permission-preflight", "作成前に同名パーミッションが存在しないことを確認する")
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.DisplayName == name {
			return errors.New("temporary permission name already exists; refusing to modify it")
		}
	}
	requestPath := filepath.Join(dir, "permission.json")
	request, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if err := os.WriteFile(requestPath, request, 0o600); err != nil {
		return err
	}
	output, createErr := s.client.call(ctx, "permission-create", "テスト用パーミッションを作成する", "object-storage-api", "permission", "create", s.siteID, "--request", "@"+requestPath)
	var created permission
	if err := json.Unmarshal(output, &created); err != nil {
		return errors.Join(createErr, fmt.Errorf("cannot identify created permission; inspect site %s and name %s manually: %w", s.siteID, name, err))
	}
	if created.ID <= 0 || created.DisplayName != expected.DisplayName || !controlsMatch(created.Controls, expected.Controls) {
		return errors.Join(createErr, errors.New("created permission identity or controls do not match; manual inspection required"))
	}
	expected.ID = created.ID
	deleted := false
	defer func() {
		if !deleted {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, s.deletePermission(cleanupCtx, expected))
		}
	}()
	if createErr != nil {
		return createErr
	}
	if err := s.verifyPermission(ctx, "permission-verify-create", expected); err != nil {
		return err
	}
	permissionID := strconv.FormatInt(expected.ID, 10)
	if err := s.runKey(ctx, dir, []string{"permission", "access-key"}, []string{s.siteID, permissionID}); err != nil {
		return err
	}
	expected.Controls[0].CanWrite = false
	request, err = json.Marshal(permission{DisplayName: expected.DisplayName, Controls: expected.Controls})
	if err != nil {
		return err
	}
	if err := os.WriteFile(requestPath, request, 0o600); err != nil {
		return err
	}
	if _, err := s.client.call(ctx, "permission-update", "パーミッションの書き込み権限を変更する", "object-storage-api", "permission", "update", s.siteID, permissionID, "--request", "@"+requestPath); err != nil {
		return err
	}
	if err := s.verifyPermission(ctx, "permission-verify-update", expected); err != nil {
		return err
	}
	if err := s.deletePermission(ctx, expected); err != nil {
		return err
	}
	deleted = true
	return nil
}

func (s scenario) verifyPermission(ctx context.Context, step string, expected permission) error {
	description := "パーミッションの作成または更新内容を確認する"
	if step == "permission-verify-delete" {
		description = "パーミッション削除後に対象が一覧に残っていないことを確認する"
	}
	items, err := s.permissions(ctx, step, description)
	if err != nil {
		return err
	}
	matches := 0
	for _, item := range items {
		if item.ID == expected.ID {
			matches++
			if item.DisplayName != expected.DisplayName || !controlsMatch(item.Controls, expected.Controls) {
				return errors.New("permission ID matches but name or bucket controls changed; refusing modification")
			}
		}
	}
	if matches != 1 {
		return errors.New("created permission ID is missing or ambiguous")
	}
	output, err := s.client.call(ctx, step+"-read", "パーミッションの詳細を確認する", "object-storage-api", "permission", "read", s.siteID, strconv.FormatInt(expected.ID, 10))
	if err != nil {
		return err
	}
	var actual permission
	if err := json.Unmarshal(output, &actual); err != nil {
		return fmt.Errorf("decode permission: %w", err)
	}
	if actual.ID != expected.ID || actual.DisplayName != expected.DisplayName || !controlsMatch(actual.Controls, expected.Controls) {
		return errors.New("permission read does not match the created permission")
	}
	return nil
}

func (s scenario) deletePermission(ctx context.Context, expected permission) error {
	expected.Controls = append([]bucketControl(nil), expected.Controls...)
	items, err := s.permissions(ctx, "permission-cleanup-presence", "パーミッション削除前に対象が存在することを確認する")
	if err != nil {
		return err
	}
	present := false
	for _, item := range items {
		if item.ID == expected.ID {
			present = true
			// Both values are authorized states of this scenario's update.
			if len(expected.Controls) == 1 && controlsMatch(item.Controls, []bucketControl{{
				BucketName: expected.Controls[0].BucketName, CanRead: true, CanWrite: true,
			}}) {
				expected.Controls[0].CanWrite = true
			}
		}
	}
	if !present {
		return nil
	}
	if err := s.verifyPermission(ctx, "permission-cleanup-check", expected); err != nil {
		return err
	}
	permissionID := strconv.FormatInt(expected.ID, 10)
	keys, err := s.keys(ctx, "permission-cleanup-keys", "パーミッション削除前にアクセスキーが残っていないことを確認する", []string{"permission", "access-key"}, []string{s.siteID, permissionID})
	if err != nil {
		return err
	}
	if len(keys) != 0 {
		return errors.New("permission still has access keys; refusing to delete it")
	}
	if _, err := s.client.call(ctx, "permission-delete", "一時的なパーミッションを削除する", "object-storage-api", "permission", "delete", s.siteID, permissionID); err != nil {
		return err
	}
	items, err = s.permissions(ctx, "permission-verify-delete", "パーミッション削除後に対象が一覧に残っていないことを確認する")
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == expected.ID {
			return errors.New("permission remains after deletion")
		}
	}
	return nil
}

func controlsMatch(actual, expected []bucketControl) bool {
	for _, want := range expected {
		count := 0
		for _, item := range actual {
			if item.BucketName == want.BucketName {
				count++
				if !reflect.DeepEqual(item, want) {
					return false
				}
			}
		}
		if count != 1 {
			return false
		}
	}
	for _, item := range actual {
		known := false
		for _, want := range expected {
			if item.BucketName == want.BucketName {
				known = true
			}
		}
		if !known && (item.CanRead || item.CanWrite) {
			return false
		}
	}
	return true
}
func keyArgs(path []string, operation string, target []string, id string) []string {
	args := append([]string{"object-storage-api"}, path...)
	args = append(args, operation)
	args = append(args, target...)
	if id != "" {
		args = append(args, id)
	}
	return args
}

func (s scenario) keys(ctx context.Context, step, description string, path, target []string) ([]accessKey, error) {
	args := keyArgs(path, "list", target, "")
	args = append(args, "--query", "map({id})")
	output, err := s.client.call(ctx, step, description, args...)
	if err != nil {
		return nil, err
	}
	return decodeArray[accessKey](output, "access key list")
}

func (s scenario) runKey(ctx context.Context, dir string, path, target []string) (result error) {
	prefix := path[0] + "-key"
	items, err := s.keys(ctx, prefix+"-preflight", "作成前に既存アクセスキーがないことを確認する", path, target)
	if err != nil {
		return err
	}
	if len(items) != 0 {
		return errors.New("access keys already exist; refusing to replace them")
	}
	output, createErr := s.client.call(ctx, prefix+"-create", "アクセスキーを作成する", keyArgs(path, "create", target, "")...)
	var created accessKey
	if err := json.Unmarshal(output, &created); err != nil || created.ID == "" {
		return errors.Join(createErr, errors.New("cannot identify created access key; inspect the new permission or account manually"))
	}
	deleted := false
	defer func() {
		if !deleted {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, s.deleteKey(cleanupCtx, path, target, created.ID))
		}
	}()
	if createErr != nil {
		return createErr
	}
	if created.Secret == "" {
		return errors.New("access key creation returned no secret")
	}
	keyPath := filepath.Join(dir, prefix+".json")
	if err := os.WriteFile(keyPath, output, 0o600); err != nil {
		return fmt.Errorf("save access key to private temporary file: %w", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("access key output file is not private")
	}
	saved, err := os.ReadFile(keyPath) //nolint:gosec // Reads only the key file just written in our private temporary directory.
	if err != nil {
		return err
	}
	if string(saved) != string(output) {
		return errors.New("saved access key output does not match the creation response")
	}
	response, err := s.client.call(ctx, prefix+"-read", "作成したアクセスキーの詳細を確認する", keyArgs(path, "read", target, created.ID)...)
	if err != nil {
		return err
	}
	var read accessKey
	if err := json.Unmarshal(response, &read); err != nil {
		return errors.New("decode access key read response")
	}
	if read.ID != created.ID || read.Secret != "" {
		return errors.New("access key read ID mismatch or secret was unexpectedly returned")
	}
	if err := s.deleteKey(ctx, path, target, created.ID); err != nil {
		return err
	}
	deleted = true
	return nil
}

func (s scenario) deleteKey(ctx context.Context, path, target []string, id string) error {
	prefix := path[0] + "-key"
	items, err := s.keys(ctx, prefix+"-cleanup-check", "アクセスキー削除前に対象が存在することを確認する", path, target)
	if err != nil {
		return err
	}
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
		}
	}
	if !found {
		return nil
	}
	if _, err := s.client.call(ctx, prefix+"-delete", "アクセスキーを削除する", keyArgs(path, "delete", target, id)...); err != nil {
		return err
	}
	items, err = s.keys(ctx, prefix+"-verify-delete", "アクセスキー削除後に対象が一覧に残っていないことを確認する", path, target)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == id {
			return errors.New("access key remains after deletion")
		}
	}
	return nil
}
