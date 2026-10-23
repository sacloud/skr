// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func validateDependencies(mode, siteID, kmsKeyID, targetSiteID string) error {
	if mode == "encryption" {
		if siteID != "tky01" || kmsKeyID == "" || targetSiteID != "" {
			return errors.New("encryption requires --site-id tky01 and --kms-key-id; no --target-site-id")
		}
		return nil
	}
	if mode == "replication" {
		if err := validateSiteID(targetSiteID); err != nil || targetSiteID == siteID || kmsKeyID != "" {
			return errors.New("replication requires different standard --site-id and --target-site-id; no --kms-key-id")
		}
		return nil
	}
	if kmsKeyID != "" || targetSiteID != "" {
		return errors.New("--kms-key-id and --target-site-id are only valid for their respective scenarios")
	}
	return nil
}

func requireObjectFields(output []byte, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(output, &object); err != nil {
		return fmt.Errorf("decode API object: %w", err)
	}
	for _, field := range fields {
		value, ok := object[field]
		if !ok || string(value) == "null" {
			return fmt.Errorf("API response is missing %s", field)
		}
	}
	return nil
}

func validateMetricsTarget(mode, name, id string, create bool) error {
	if mode == "metrics" {
		if create {
			if name != "" || id != "" {
				return errors.New("metrics must not mix a newly created bucket with an existing bucket target")
			}
		} else if name == "" || id == "" {
			return errors.New("metrics requires --bucket-name and --bucket-resource-id")
		}
	} else if name != "" || id != "" {
		return errors.New("--bucket-name and --bucket-resource-id are only for the read-only metrics scenario")
	}
	return nil
}

func (s scenario) runMetrics(ctx context.Context, name, id string) error {
	if err := s.checkSite(ctx); err != nil {
		return err
	}
	items, err := s.list(ctx, "metrics-bucket-identity")
	if err != nil {
		return err
	}
	matches := bucketsNamed(items, name)
	if len(matches) != 1 || matches[0].ResourceID != id {
		return errors.New("metrics bucket does not match the supplied resource ID")
	}
	return s.readMetrics(ctx, name)
}

func (s scenario) readMetrics(ctx context.Context, name string) (result error) {
	for _, test := range []struct {
		step   string
		args   []string
		fields []string
	}{
		{"site-status-read", []string{"site", "status", "read", s.siteID}, []string{"accept_new", "status_code"}},
		{"site-quota", []string{"site", "status", "quota", s.siteID}, []string{"num_buckets", "num_permissions"}},
		{"bucket-penalty", []string{"bucket", "penalty", s.siteID, name}, []string{"num_objects_per_bucket", "amount_gib_per_bucket"}},
		{"bucket-usage", []string{"bucket", "usage", s.siteID, name}, []string{"num_objects_per_bucket", "amount_gib_per_bucket"}},
		{"bucket-quota", []string{"bucket", "quota", s.siteID, name}, []string{"num_objects_per_bucket", "amount_gib_per_bucket"}},
	} {
		output, err := s.client.call(ctx, test.step, append([]string{"object-storage-api"}, test.args...)...)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := requireObjectFields(output, test.fields...); err != nil {
			result = errors.Join(result, fmt.Errorf("%s: %w", test.step, err))
			continue
		}
		if test.step == "bucket-usage" {
			var usage struct {
				Objects *int     `json:"num_objects_per_bucket"`
				Amount  *float64 `json:"amount_gib_per_bucket"`
			}
			if err := json.Unmarshal(output, &usage); err != nil {
				result = errors.Join(result, fmt.Errorf("decode bucket usage: %w", err))
				continue
			}
			if usage.Objects == nil || usage.Amount == nil || *usage.Objects < 0 || *usage.Amount < 0 {
				result = errors.Join(result, errors.New("bucket has invalid storage usage"))
			}
		}
	}
	now := time.Now().UTC()
	to := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, -1, 0)
	output, err := s.client.call(ctx, "bucket-metering", "object-storage-api", "site", "status", "bucket-metering", s.siteID, name,
		"--from", from.Format(time.RFC3339), "--to", to.Format(time.RFC3339))
	if err != nil {
		return errors.Join(result, err)
	}
	var billing []struct {
		YearMonth string `json:"year_month"`
	}
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "[") {
		return errors.Join(result, errors.New("bucket metering must return a JSON array"))
	}
	if err := json.Unmarshal(output, &billing); err != nil {
		return errors.Join(result, fmt.Errorf("decode bucket metering: %w", err))
	}
	if len(billing) != 2 {
		return errors.Join(result, errors.New("metering must include both requested months, including zero-filled months"))
	}
	seen := make(map[string]bool)
	for _, item := range billing {
		month, err := time.Parse("2006-01", item.YearMonth)
		if err != nil || month.Before(from) || month.After(to) || seen[item.YearMonth] {
			return errors.Join(result, errors.New("metering returned an invalid, unexpected, or duplicate year_month"))
		}
		seen[item.YearMonth] = true
	}
	return result
}
