// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type replication struct {
	Source createdBucket `json:"source_bucket"`
	Target createdBucket `json:"dest_bucket"`
	Status string        `json:"config_status"`
}

func (s scenario) runReplication(ctx context.Context, source, target, targetSite string) (result error) {
	read := func(ctx context.Context) ([]byte, error) {
		return s.client.call(ctx, "replication-read", "バケットのレプリケーション設定を確認する", "object-storage-api", "bucket", "replication", "read", s.siteID, source)
	}
	_, err := read(ctx)
	if !apiNotFound(err, "BucketExtra.ReadReplication") {
		return errors.Join(err, errors.New("new source bucket must have no replication setting"))
	}
	disabled := false
	disable := func(ctx context.Context) error {
		output, err := read(ctx)
		if apiNotFound(err, "BucketExtra.ReadReplication") {
			return nil
		}
		if err != nil {
			return err
		}
		if err := checkReplication(output, source, s.siteID, target, targetSite); err != nil {
			return err
		}
		if _, err := s.client.call(ctx, "replication-disable", "バケットのレプリケーションを無効化する", "object-storage-api", "bucket", "replication", "disable", s.siteID, source); err != nil {
			return err
		}
		return waitFor(ctx, func(ctx context.Context) (bool, error) {
			_, err := read(ctx)
			if apiNotFound(err, "BucketExtra.ReadReplication") {
				return true, nil
			}
			return false, err
		})
	}
	defer func() {
		if !disabled {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, disable(cleanupCtx))
		}
	}()
	output, err := s.client.call(ctx, "replication-enable", "バケットのレプリケーションを有効化する", "object-storage-api", "bucket", "replication", "enable", s.siteID, source, "--target-bucket", target)
	if err != nil {
		return err
	}
	if err := checkReplication(output, source, s.siteID, target, targetSite); err != nil {
		return err
	}
	if err := waitFor(ctx, func(ctx context.Context) (bool, error) {
		output, err := read(ctx)
		if err != nil {
			return false, err
		}
		if err := checkReplication(output, source, s.siteID, target, targetSite); err != nil {
			return false, err
		}
		var actual replication
		if err := json.Unmarshal(output, &actual); err != nil {
			return false, err
		}
		return actual.Status == "created", nil
	}); err != nil {
		return err
	}
	if err := disable(ctx); err != nil {
		return err
	}
	disabled = true
	return nil
}

func checkReplication(output []byte, source, sourceSite, target, targetSite string) error {
	var actual replication
	if err := json.Unmarshal(output, &actual); err != nil {
		return fmt.Errorf("decode replication: %w", err)
	}
	if actual.Source.Name != source || actual.Source.ClusterID != sourceSite ||
		actual.Target.Name != target || actual.Target.ClusterID != targetSite ||
		(actual.Status != "creating" && actual.Status != "created") {
		return errors.New("replication source, target, or status does not match the created buckets")
	}
	return nil
}
