// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type encryptionTestCLI struct {
	callFn func(context.Context, string, ...string) ([]byte, error)
}

func (c encryptionTestCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return c.callFn(ctx, step, args...)
}

func TestEncryptionCleanupRequiresCurrentKeyIdentity(t *testing.T) {
	for _, test := range []struct {
		name          string
		response      string
		readError     bool
		cancel        bool
		afterVerified bool
		wantDisable   bool
		wantError     bool
		wantCancel    bool
	}{
		{name: "matching key", response: `{"kms_key_id":"ours"}`, wantDisable: true},
		{name: "different key while waiting", response: `{"kms_key_id":"other"}`, wantError: true},
		{name: "key replaced before disable", response: `{"kms_key_id":"other"}`, afterVerified: true, wantError: true},
		{name: "invalid JSON", response: `{`, wantError: true},
		{name: "null response", response: `null`, wantError: true},
		{name: "missing key ID", response: `{}`, wantError: true},
		{name: "null key ID", response: `{"kms_key_id":null}`, wantError: true},
		{name: "empty key ID", response: `{"kms_key_id":""}`, wantError: true},
		{name: "wrong key type", response: `{"kms_key_id":123}`, wantError: true},
		{name: "lookup fails", readError: true, wantError: true},
		{name: "already absent", response: "absent", afterVerified: true},
		{name: "cancel after replacement", response: `{"kms_key_id":"other"}`, cancel: true, wantError: true, wantCancel: true},
		{name: "cancel then lookup fails", readError: true, cancel: true, wantError: true, wantCancel: true},
		{name: "cancel then invalid response", response: `null`, cancel: true, wantError: true, wantCancel: true},
		{name: "cancel then absent", response: "absent", cancel: true, wantError: true, wantCancel: true},
		{name: "cancel with matching key", response: `{"kms_key_id":"ours"}`, cancel: true, wantDisable: true, wantError: true, wantCancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			enabled, disabled := false, false
			reads := 0
			lookupError := errors.New("encryption lookup failed")
			client := encryptionTestCLI{callFn: func(ctx context.Context, step string, _ ...string) ([]byte, error) {
				switch step {
				case "encryption-read":
					if !enabled || disabled {
						return nil, notFound("BucketExtra.ReadEncryption")
					}
					if err := ctx.Err(); err != nil {
						t.Fatalf("cleanup reused canceled context: %v", err)
					}
					reads++
					if test.afterVerified && reads == 1 {
						return []byte(`{"kms_key_id":"ours"}`), nil
					}
					if test.readError {
						return nil, lookupError
					}
					if test.response == "absent" {
						return nil, notFound("BucketExtra.ReadEncryption")
					}
					return []byte(test.response), nil
				case "encryption-enable":
					enabled = true
					if test.cancel {
						cancel()
					}
					return nil, nil
				case "encryption-disable":
					if err := ctx.Err(); err != nil {
						t.Fatalf("disable used canceled context: %v", err)
					}
					if disabled {
						t.Fatal("disable called more than once")
					}
					disabled = true
					return nil, nil
				default:
					t.Fatalf("unexpected step %s", step)
					return nil, nil
				}
			}}
			err := (scenario{client: client, siteID: "tky01"}).runEncryption(ctx, "test-bucket", "ours")
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
			if disabled != test.wantDisable {
				t.Fatalf("disable called = %v, want %v", disabled, test.wantDisable)
			}
			if test.wantCancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was lost: %v", err)
			}
			if test.readError && !errors.Is(err, lookupError) {
				t.Fatalf("lookup failure was lost: %v", err)
			}
			if test.cancel && strings.Contains(test.response, "other") && !strings.Contains(err.Error(), "refusing to disable") {
				t.Fatalf("cleanup refusal was lost: %v", err)
			}
			unverified := !test.wantDisable && test.response != "absent"
			if errors.Is(err, errUnverifiedBucketSetting) != unverified {
				t.Fatalf("unsafe bucket deletion guard = %v, want %v: %v", errors.Is(err, errUnverifiedBucketSetting), unverified, err)
			}
		})
	}
}

func TestEncryptionOwnershipFailurePreservesParentBucket(t *testing.T) {
	fake := &fakeCLI{}
	s := scenario{client: fake, siteID: "isk01", extra: func(context.Context, string) error {
		return errUnverifiedBucketSetting
	}}
	err := s.run(context.Background(), "skr-e2e-0123456789abcdef")
	if !errors.Is(err, errUnverifiedBucketSetting) {
		t.Fatalf("ownership error was lost: %v", err)
	}
	for _, step := range []string{"bucket-delete", "bucket-delete-retry"} {
		if _, called := fake.args[step]; called {
			t.Fatalf("deleted parent bucket after configuration ownership failure: %s", step)
		}
	}
}
