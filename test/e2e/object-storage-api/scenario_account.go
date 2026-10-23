// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

func (s scenario) runAccount(ctx context.Context) (result error) {
	return s.runAccountWith(ctx, nil)
}

func (s scenario) runAccountWith(ctx context.Context, exercise func(context.Context) error) (result error) {
	siteOnly := s
	siteOnly.plan = ""
	if err := siteOnly.checkSite(ctx); err != nil {
		return err
	}
	_, err := s.client.call(ctx, "account-preflight", "object-storage-api", "account", "read", s.siteID)
	if err == nil {
		return errors.New("site account already exists; refusing to create or delete an existing account")
	}
	if !accountNotFound(err) {
		return fmt.Errorf("account absence was not confirmed by an Accounts.Read API 404: %w", err)
	}
	output, createErr := s.client.call(ctx, "account-create", "object-storage-api", "account", "create", s.siteID)
	var created account
	if err := json.Unmarshal(output, &created); err != nil || created.ResourceID == "" {
		return errors.Join(createErr, errors.New("created account ID is unknown; manual inspection required"))
	}
	deleted := false
	defer func() {
		if !deleted {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, s.deleteAccount(cleanupCtx, created))
		}
	}()
	if createErr != nil {
		return createErr
	}
	if err := s.verifyAccount(ctx, created); err != nil {
		return err
	}
	plans, err := s.client.call(ctx, "account-site-plans", "object-storage-api", "site", "plans", s.siteID,
		"--query", "map({service_class_path,type,cluster_id,capacity_gib,fee})")
	if err != nil {
		return err
	}
	available, err := decodeArray[plan](plans, "account site plans")
	if err != nil {
		return err
	}
	if len(available) == 0 {
		return errors.New("site returned no available plans")
	}
	dir, err := os.MkdirTemp("", "skr-object-storage-account-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(dir)) }()
	if err := s.runKey(ctx, dir, []string{"account", "access-key"}, []string{s.siteID}); err != nil {
		return err
	}
	if exercise != nil {
		if err := exercise(ctx); err != nil {
			return err
		}
	}
	if err := s.deleteAccount(ctx, created); err != nil {
		return err
	}
	deleted = true
	return nil
}

func (s scenario) verifyAccount(ctx context.Context, expected account) error {
	output, err := s.client.call(ctx, "account-identity-check", "object-storage-api", "account", "read", s.siteID)
	if err != nil {
		return err
	}
	var actual account
	if err := json.Unmarshal(output, &actual); err != nil {
		return fmt.Errorf("decode site account: %w", err)
	}
	if actual.ResourceID != expected.ResourceID {
		return errors.New("site account ID changed; refusing modification")
	}
	return nil
}

func (s scenario) deleteAccount(ctx context.Context, expected account) error {
	if err := s.verifyAccount(ctx, expected); err != nil {
		if accountNotFound(err) {
			return nil
		}
		return err
	}
	buckets, err := s.list(ctx, "account-cleanup-buckets")
	if err != nil {
		return err
	}
	permissions, err := s.permissions(ctx, "account-cleanup-permissions")
	if err != nil {
		return err
	}
	keys, err := s.keys(ctx, "account-cleanup-keys", []string{"account", "access-key"}, []string{s.siteID})
	if err != nil {
		return err
	}
	if len(buckets) != 0 || len(permissions) != 0 || len(keys) != 0 {
		return errors.New("site account contains resources; refusing to delete it")
	}
	if err := s.verifyAccount(ctx, expected); err != nil {
		return err
	}
	if _, err := s.client.call(ctx, "account-delete", "object-storage-api", "account", "delete", s.siteID); err != nil {
		return err
	}
	_, err = s.client.call(ctx, "account-verify-delete", "object-storage-api", "account", "read", s.siteID)
	if !accountNotFound(err) {
		return errors.Join(err, errors.New("account deletion was not confirmed by an Accounts.Read API 404"))
	}
	return nil
}
