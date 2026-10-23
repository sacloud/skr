// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (s scenario) runEncryption(ctx context.Context, name, kmsKeyID string) (result error) {
	args := []string{"object-storage-api", "bucket", "encryption"}
	read := func(ctx context.Context) ([]byte, error) {
		return s.client.call(ctx, "encryption-read", append(append([]string{}, args...), "read", s.siteID, name)...)
	}
	output, err := read(ctx)
	if err != nil && !apiNotFound(err, "BucketExtra.ReadEncryption") {
		return err
	}
	if err == nil {
		var initial struct {
			KeyID string `json:"kms_key_id"`
		}
		if err := json.Unmarshal(output, &initial); err != nil {
			return err
		}
		if initial.KeyID != "" {
			return errors.New("new bucket unexpectedly has encryption configured")
		}
	}
	disabled := false
	disable := func(ctx context.Context) error {
		if _, err := s.client.call(ctx, "encryption-disable", append(append([]string{}, args...), "disable", s.siteID, name)...); err != nil {
			return err
		}
		return waitFor(ctx, func(ctx context.Context) (bool, error) {
			output, err := read(ctx)
			if apiNotFound(err, "BucketExtra.ReadEncryption") {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			var state struct {
				KeyID string `json:"kms_key_id"`
			}
			if err := json.Unmarshal(output, &state); err != nil {
				return false, err
			}
			return state.KeyID == "", nil
		})
	}
	defer func() {
		if !disabled {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, disable(cleanupCtx))
		}
	}()
	if _, err := s.client.call(ctx, "encryption-enable", append(append([]string{}, args...), "enable", s.siteID, name, "--kms-key-id", kmsKeyID)...); err != nil {
		return err
	}
	if err := waitFor(ctx, func(ctx context.Context) (bool, error) {
		output, err := read(ctx)
		if err != nil {
			return false, err
		}
		var state struct {
			KeyID string `json:"kms_key_id"`
		}
		if err := json.Unmarshal(output, &state); err != nil {
			return false, err
		}
		return state.KeyID == kmsKeyID, nil
	}); err != nil {
		return err
	}
	if err := disable(ctx); err != nil {
		return err
	}
	disabled = true
	return nil
}
