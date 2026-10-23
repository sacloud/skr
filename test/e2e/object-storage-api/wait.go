// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func apiNotFound(err error, method string) bool {
	var failure *commandError
	return errors.As(err, &failure) &&
		!failure.recordFailed &&
		strings.HasPrefix(strings.TrimSpace(failure.stderr), "object-storage: "+method+": API Error 404:")
}

func waitFor(ctx context.Context, check func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("waiting for API state: %w", err)
		}
		done, err := check(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for API state: %w", ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
}
