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

func (s scenario) runEncryption(ctx context.Context, name, kmsKeyID string) (result error) {
	cleanupConfirmed := false
	defer func() {
		if !cleanupConfirmed {
			result = errors.Join(result, errUnverifiedBucketSetting)
		}
	}()
	args := []string{"object-storage-api", "bucket", "encryption"}
	read := func(ctx context.Context) ([]byte, error) {
		return s.client.call(ctx, "encryption-read", "バケットの暗号化設定を確認する", append(append([]string{}, args...), "read", s.siteID, name)...)
	}
	output, err := read(ctx)
	if err != nil && !apiNotFound(err, "BucketExtra.ReadEncryption") {
		return err
	}
	if err == nil {
		if _, err := encryptionKeyID(output); err != nil {
			return err
		}
		return errors.New("new bucket unexpectedly has encryption configured")
	}
	disabled := false
	disable := func(ctx context.Context) error {
		output, err := read(ctx)
		if apiNotFound(err, "BucketExtra.ReadEncryption") {
			return nil
		}
		if err != nil {
			return err
		}
		keyID, err := encryptionKeyID(output)
		if err != nil {
			return err
		}
		if keyID != kmsKeyID {
			return errors.New("encryption key ID does not match this scenario; refusing to disable it")
		}
		if _, err := s.client.call(ctx, "encryption-disable", "バケットの暗号化を無効化する", append(append([]string{}, args...), "disable", s.siteID, name)...); err != nil {
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
			keyID, err := encryptionKeyID(output)
			if err != nil {
				return false, err
			}
			if keyID != kmsKeyID {
				return false, errors.New("encryption key ID changed while waiting for disable")
			}
			return false, nil
		})
	}
	defer func() {
		if !disabled {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			err := disable(cleanupCtx)
			if err == nil {
				cleanupConfirmed = true
			}
			result = errors.Join(result, err)
		}
	}()
	if _, err := s.client.call(ctx, "encryption-enable", "バケットの暗号化を有効化する", append(append([]string{}, args...), "enable", s.siteID, name, "--kms-key-id", kmsKeyID)...); err != nil {
		return err
	}
	if err := waitFor(ctx, func(ctx context.Context) (bool, error) {
		output, err := read(ctx)
		if err != nil {
			return false, err
		}
		keyID, err := encryptionKeyID(output)
		if err != nil {
			return false, err
		}
		if keyID != kmsKeyID {
			return false, errors.New("enabled encryption key ID does not match this scenario")
		}
		return true, nil
	}); err != nil {
		return err
	}

	if err := disable(ctx); err != nil {
		return err
	}
	disabled = true
	cleanupConfirmed = true
	return nil
}

func encryptionKeyID(output []byte) (string, error) {
	var state struct {
		KeyID string `json:"kms_key_id"`
	}
	if err := json.Unmarshal(output, &state); err != nil {
		return "", fmt.Errorf("decode encryption setting: %w", err)
	}
	if state.KeyID == "" {
		return "", errors.New("encryption setting has no key ID; refusing to modify it")
	}
	return state.KeyID, nil
}
