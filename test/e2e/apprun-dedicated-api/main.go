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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	zone           = "is1b"
	namePrefix     = "skr-e2e-"
	commandTimeout = time.Minute
	cleanupTimeout = 5 * time.Minute
	deployTimeout  = 15 * time.Minute
)

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r *cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // Structured arguments are passed directly without a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	recordedStdout, recordedStderr := stdout.String(), stderr.String()
	switch step {
	case "create-version", "verify-created-version", "verify-container-placement":
		recordedStdout = "[REDACTED: potentially sensitive AppRun Dedicated response]"
		recordedStderr = "[REDACTED: potentially sensitive AppRun Dedicated response]"
	}
	if err := r.evidence.Record(step, args, nil, recordedStdout, recordedStderr, runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (see evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type namedItem struct {
	ID   string `json:"clusterID"`
	Name string `json:"name"`
}

type clusterPage struct {
	Items      []namedItem `json:"items"`
	NextCursor string      `json:"nextCursor"`
}

type groupPage struct {
	Items []struct {
		ID   string `json:"autoScalingGroupID"`
		Name string `json:"name"`
	} `json:"items"`
	NextCursor string `json:"nextCursor"`
}

type applicationPage struct {
	Items []struct {
		ID        string `json:"applicationID"`
		Name      string `json:"name"`
		ClusterID string `json:"clusterID"`
	} `json:"items"`
	NextCursor string `json:"nextCursor"`
}

type resource struct {
	id        string
	name      string
	attempted bool
}

type scenario struct {
	client           cli
	clusterRequest   string
	groupRequest     string
	versionRequest   string
	cluster          resource
	group            resource
	application      resource
	version          int
	versionAttempted bool
}

func (s *scenario) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	return s.client.call(ctx, step, args...)
}

func (s *scenario) listClusters(ctx context.Context, step string) ([]namedItem, error) {
	var result []namedItem
	var cursor string
	seen := map[string]struct{}{}
	for {
		args := []string{"apprun-dedicated-api", "cluster", "list", "--max-items", "100", "--output", "json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		data, err := s.call(ctx, step, args...)
		if err != nil {
			return nil, err
		}
		var page clusterPage
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("%s: decode cluster page: %w", step, err)
		}
		if page.Items == nil {
			return nil, fmt.Errorf("%s: expected cluster items array", step)
		}
		result = append(result, page.Items...)
		if page.NextCursor == "" {
			break
		}
		if _, ok := seen[page.NextCursor]; ok {
			return nil, fmt.Errorf("%s: repeated cluster cursor %q", step, page.NextCursor)
		}
		seen[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	return result, nil
}

func (s *scenario) listGroups(ctx context.Context, step string) ([]groupPage, error) {
	var pages []groupPage
	cursor := ""
	seen := map[string]struct{}{}
	for {
		args := []string{"apprun-dedicated-api", "auto-scaling-group", "list", "--cluster-id", s.cluster.id, "--max-items", "100", "--output", "json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		data, err := s.call(ctx, step, args...)
		if err != nil {
			return nil, err
		}
		var page groupPage
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("%s: decode auto-scaling-group page: %w", step, err)
		}
		if page.Items == nil {
			return nil, fmt.Errorf("%s: expected auto-scaling-group items array", step)
		}
		pages = append(pages, page)
		if page.NextCursor == "" {
			return pages, nil
		}
		if _, ok := seen[page.NextCursor]; ok {
			return nil, fmt.Errorf("%s: repeated auto-scaling-group cursor %q", step, page.NextCursor)
		}
		seen[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
}

func (s *scenario) listApplications(ctx context.Context, step string) ([]applicationPage, error) {
	var pages []applicationPage
	cursor := ""
	seen := map[string]struct{}{}
	for {
		args := []string{"apprun-dedicated-api", "application", "list", "--max-items", "100", "--output", "json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		data, err := s.call(ctx, step, args...)
		if err != nil {
			return nil, err
		}
		var page applicationPage
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("%s: decode application page: %w", step, err)
		}
		if page.Items == nil {
			return nil, fmt.Errorf("%s: expected application items array", step)
		}
		pages = append(pages, page)
		if page.NextCursor == "" {
			return pages, nil
		}
		if _, ok := seen[page.NextCursor]; ok {
			return nil, fmt.Errorf("%s: repeated application cursor %q", step, page.NextCursor)
		}
		seen[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
}

func (s *scenario) preflight(ctx context.Context) error {
	profile, err := s.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	clusters, err := s.listClusters(ctx, "preflight-clusters")
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		if cluster.Name == s.cluster.name {
			return fmt.Errorf("cluster %q already exists; refusing to modify it", s.cluster.name)
		}
	}
	return nil
}

func loadRequest(path string) (map[string]json.RawMessage, error) {
	if path == "" {
		return nil, errors.New("request file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read request file %s: %w", path, err)
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("decode request file %s: %w", path, err)
	}
	if request == nil {
		return nil, fmt.Errorf("request file %s must contain a JSON object", path)
	}
	return request, nil
}

func writeRequest(dir, filename string, request map[string]json.RawMessage) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", filename, err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write private %s request: %w", filename, err)
	}
	return path, nil
}

func setString(request map[string]json.RawMessage, field, value string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	request[field] = data
	return nil
}

func setInteger(request map[string]json.RawMessage, field string, value int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	request[field] = data
	return nil
}

func (s *scenario) prepareRequests(dir string) error {
	clusterRequest, err := loadRequest(s.clusterRequest)
	if err != nil {
		return err
	}
	var principal string
	if err := json.Unmarshal(clusterRequest["servicePrincipalID"], &principal); err != nil || principal == "" {
		return errors.New("cluster request must include a non-empty servicePrincipalID")
	}
	if err := setString(clusterRequest, "name", s.cluster.name); err != nil {
		return err
	}
	s.clusterRequest, err = writeRequest(dir, "cluster.json", clusterRequest)
	if err != nil {
		return err
	}

	groupRequest, err := loadRequest(s.groupRequest)
	if err != nil {
		return err
	}
	if err := setString(groupRequest, "name", s.group.name); err != nil {
		return err
	}
	if err := setString(groupRequest, "zone", zone); err != nil {
		return err
	}
	for _, field := range []string{"minNodes", "maxNodes"} {
		if err := setInteger(groupRequest, field, 1); err != nil {
			return err
		}
	}
	s.groupRequest, err = writeRequest(dir, "auto-scaling-group.json", groupRequest)
	if err != nil {
		return err
	}

	versionRequest, err := loadRequest(s.versionRequest)
	if err != nil {
		return err
	}
	var image string
	if err := json.Unmarshal(versionRequest["image"], &image); err != nil || strings.TrimSpace(image) == "" {
		return errors.New("version request must include a non-empty image")
	}
	if err := setString(versionRequest, "scalingMode", "manual"); err != nil {
		return err
	}
	if err := setInteger(versionRequest, "fixedScale", 1); err != nil {
		return err
	}
	for _, field := range []string{"minScale", "maxScale", "scaleInThreshold", "scaleOutThreshold"} {
		delete(versionRequest, field)
	}
	s.versionRequest, err = writeRequest(dir, "version.json", versionRequest)
	return err
}

func (s *scenario) createCluster(ctx context.Context) error {
	s.cluster.attempted = true
	data, err := s.call(ctx, "create-cluster", "apprun-dedicated-api", "cluster", "create",
		"--request", "@"+s.clusterRequest, "--output", "json")
	if err != nil {
		return err
	}
	var result struct {
		ID string `json:"clusterID"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode created cluster: %w", err)
	}
	if result.ID == "" {
		return errors.New("created cluster ID is empty")
	}
	s.cluster.id = result.ID
	data, err = s.call(ctx, "verify-created-cluster", "apprun-dedicated-api", "cluster", "read", s.cluster.id, "--output", "json")
	if err != nil {
		return err
	}
	var current struct {
		ClusterID          string `json:"clusterID"`
		Name               string `json:"name"`
		ServicePrincipalID string `json:"servicePrincipalID"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode created cluster details: %w", err)
	}
	request, err := loadRequest(s.clusterRequest)
	if err != nil {
		return err
	}
	var servicePrincipalID string
	if err := json.Unmarshal(request["servicePrincipalID"], &servicePrincipalID); err != nil {
		return err
	}
	if current.ClusterID != s.cluster.id || current.Name != s.cluster.name || current.ServicePrincipalID != servicePrincipalID {
		return errors.New("created cluster identity does not match this E2E")
	}
	return nil
}

func (s *scenario) createGroup(ctx context.Context) error {
	s.group.attempted = true
	data, err := s.call(ctx, "create-auto-scaling-group", "apprun-dedicated-api", "auto-scaling-group", "create",
		"--cluster-id", s.cluster.id, "--request", "@"+s.groupRequest, "--output", "json")
	if err != nil {
		return err
	}
	var result struct {
		ID string `json:"autoScalingGroupID"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode created auto-scaling group: %w", err)
	}
	if result.ID == "" {
		return errors.New("created auto-scaling group ID is empty")
	}
	s.group.id = result.ID
	data, err = s.call(ctx, "verify-created-auto-scaling-group", "apprun-dedicated-api", "auto-scaling-group", "read",
		"--cluster-id", s.cluster.id, "--auto-scaling-group-id", s.group.id, "--output", "json")
	if err != nil {
		return err
	}
	var current struct {
		AutoScalingGroupID     string `json:"autoScalingGroupID"`
		Name                   string `json:"name"`
		Zone                   string `json:"zone"`
		WorkerServiceClassPath string `json:"workerServiceClassPath"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode created auto-scaling group details: %w", err)
	}
	request, err := loadRequest(s.groupRequest)
	if err != nil {
		return err
	}
	var workerServiceClassPath string
	if err := json.Unmarshal(request["workerServiceClassPath"], &workerServiceClassPath); err != nil {
		return err
	}
	if current.AutoScalingGroupID != s.group.id || current.Name != s.group.name || current.Zone != zone ||
		current.WorkerServiceClassPath != workerServiceClassPath {
		return errors.New("created auto-scaling group identity does not match this E2E")
	}
	return nil
}

func (s *scenario) createApplication(ctx context.Context) error {
	s.application.attempted = true
	data, err := s.call(ctx, "create-application", "apprun-dedicated-api", "application", "create",
		"--name", s.application.name, "--cluster-id", s.cluster.id, "--output", "json")
	if err != nil {
		return err
	}
	var result struct {
		ID string `json:"applicationID"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode created application: %w", err)
	}
	if result.ID == "" {
		return errors.New("created application ID is empty")
	}
	s.application.id = result.ID
	data, err = s.call(ctx, "verify-created-application", "apprun-dedicated-api", "application", "read", s.application.id, "--output", "json")
	if err != nil {
		return err
	}
	var current struct {
		ApplicationID string `json:"applicationID"`
		Name          string `json:"name"`
		ClusterID     string `json:"clusterID"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode created application details: %w", err)
	}
	if current.ApplicationID != s.application.id || current.Name != s.application.name || current.ClusterID != s.cluster.id {
		return errors.New("created application identity does not match this E2E")
	}
	return nil
}

func (s *scenario) createAndActivateVersion(ctx context.Context) error {
	s.versionAttempted = true
	data, err := s.call(ctx, "create-version", "apprun-dedicated-api", "version", "create",
		"--application-id", s.application.id, "--request", "@"+s.versionRequest, "--output", "json")
	if err != nil {
		return err
	}
	var result struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode created application version: %w", err)
	}
	if result.Version < 1 {
		return fmt.Errorf("created application version is invalid: %d", result.Version)
	}
	s.version = result.Version
	data, err = s.call(ctx, "verify-created-version", "apprun-dedicated-api", "version", "read",
		"--application-id", s.application.id, fmt.Sprint(s.version), "--output", "json")
	if err != nil {
		return err
	}
	var current struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode created application version details: %w", err)
	}
	if current.Version != s.version {
		return fmt.Errorf("application version identity mismatch: got %d, want %d", current.Version, s.version)
	}
	_, err = s.call(ctx, "activate-version", "apprun-dedicated-api", "application", "update",
		s.application.id, "--active-version", fmt.Sprint(s.version))
	return err
}

func (s *scenario) verifyDeployment(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, deployTimeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		data, err := s.call(ctx, "verify-active-version", "apprun-dedicated-api", "application", "read", s.application.id, "--output", "json")
		if err != nil {
			return err
		}
		var application struct {
			ActiveVersion *int `json:"activeVersion"`
		}
		if err := json.Unmarshal(data, &application); err != nil {
			return fmt.Errorf("decode application state: %w", err)
		}
		if application.ActiveVersion != nil && *application.ActiveVersion == s.version {
			containers, err := s.call(ctx, "verify-container-placement", "apprun-dedicated-api", "application", "containers", s.application.id, "--output", "json")
			if err != nil {
				return err
			}
			var placements []json.RawMessage
			if err := json.Unmarshal(containers, &placements); err != nil {
				return fmt.Errorf("decode application container placements: %w", err)
			}
			if len(placements) > 0 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("application did not reach an active version with container placement within %s: %w", deployTimeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *scenario) recoverID(ctx context.Context, value *resource) error {
	if value.id != "" {
		return nil
	}
	switch value {
	case &s.cluster:
		items, err := s.listClusters(ctx, "cleanup-find-cluster")
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Name == value.name {
				if value.id != "" {
					return errors.New("multiple clusters match the E2E name; refusing cleanup")
				}
				value.id = item.ID
			}
		}
	case &s.group:
		pages, err := s.listGroups(ctx, "cleanup-find-auto-scaling-group")
		if err != nil {
			return err
		}
		for _, page := range pages {
			for _, item := range page.Items {
				if item.Name == value.name {
					if value.id != "" {
						return errors.New("multiple auto-scaling groups match the E2E name; refusing cleanup")
					}
					value.id = item.ID
				}
			}
		}
	case &s.application:
		pages, err := s.listApplications(ctx, "cleanup-find-application")
		if err != nil {
			return err
		}
		for _, page := range pages {
			for _, item := range page.Items {
				if item.Name == value.name && item.ClusterID == s.cluster.id {
					if value.id != "" {
						return errors.New("multiple applications match the E2E identity; refusing cleanup")
					}
					value.id = item.ID
				}
			}
		}
	}
	return nil
}

func (s *scenario) verifyIdentity(ctx context.Context, value *resource) error {
	switch value {
	case &s.cluster:
		data, err := s.call(ctx, "cleanup-read-cluster", "apprun-dedicated-api", "cluster", "read", value.id, "--output", "json")
		if err != nil {
			return err
		}
		var current struct {
			ClusterID          string `json:"clusterID"`
			Name               string `json:"name"`
			ServicePrincipalID string `json:"servicePrincipalID"`
		}
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
		request, err := loadRequest(s.clusterRequest)
		if err != nil {
			return err
		}
		var principal string
		if err := json.Unmarshal(request["servicePrincipalID"], &principal); err != nil {
			return err
		}
		if current.ClusterID != value.id || current.Name != value.name || current.ServicePrincipalID != principal {
			return errors.New("cleanup refused: cluster identity changed")
		}
	case &s.group:
		data, err := s.call(ctx, "cleanup-read-auto-scaling-group", "apprun-dedicated-api", "auto-scaling-group", "read",
			"--cluster-id", s.cluster.id, "--auto-scaling-group-id", value.id, "--output", "json")
		if err != nil {
			return err
		}
		var current struct {
			AutoScalingGroupID string `json:"autoScalingGroupID"`
			Name               string `json:"name"`
			Zone               string `json:"zone"`
		}
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
		if current.AutoScalingGroupID != value.id || current.Name != value.name || current.Zone != zone {
			return errors.New("cleanup refused: auto-scaling-group identity changed")
		}
	case &s.application:
		data, err := s.call(ctx, "cleanup-read-application", "apprun-dedicated-api", "application", "read", value.id, "--output", "json")
		if err != nil {
			return err
		}
		var current struct {
			ApplicationID string `json:"applicationID"`
			Name          string `json:"name"`
			ClusterID     string `json:"clusterID"`
		}
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
		if current.ApplicationID != value.id || current.Name != value.name || current.ClusterID != s.cluster.id {
			return errors.New("cleanup refused: application identity changed")
		}
	}
	return nil
}

func (s *scenario) deactivateApplication(ctx context.Context) error {
	data, err := s.call(ctx, "cleanup-read-application-state", "apprun-dedicated-api", "application", "read", s.application.id, "--output", "json")
	if err != nil {
		return err
	}
	var current struct {
		ActiveVersion *int `json:"activeVersion"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode application state before cleanup: %w", err)
	}
	if current.ActiveVersion == nil {
		return nil
	}
	_, err = s.call(ctx, "cleanup-deactivate-application", "apprun-dedicated-api", "application", "update",
		s.application.id, "--deactivate")
	return err
}

func (s *scenario) deleteResource(ctx context.Context, value *resource) error {
	if !value.attempted {
		return nil
	}
	if err := s.recoverID(ctx, value); err != nil {
		return err
	}
	if value.id == "" {
		return nil
	}
	if err := s.verifyIdentity(ctx, value); err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", value.name, err)
	}
	var step string
	var args []string
	switch value {
	case &s.application:
		step = "cleanup-delete-application"
		args = []string{"apprun-dedicated-api", "application", "delete", value.id}
	case &s.group:
		step = "cleanup-delete-auto-scaling-group"
		args = []string{"apprun-dedicated-api", "auto-scaling-group", "delete", "--cluster-id", s.cluster.id, "--auto-scaling-group-id", value.id}
	case &s.cluster:
		step = "cleanup-delete-cluster"
		args = []string{"apprun-dedicated-api", "cluster", "delete", value.id}
	default:
		return errors.New("unknown E2E resource")
	}
	if _, err := s.call(ctx, step, args...); err != nil {
		return err
	}
	return s.verifyAbsent(ctx, value)
}

func (s *scenario) verifyAbsent(ctx context.Context, value *resource) error {
	switch value {
	case &s.cluster:
		items, err := s.listClusters(ctx, "cleanup-verify-cluster-deleted")
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.ID == value.id || item.Name == value.name {
				return fmt.Errorf("cluster %s remains after cleanup", value.id)
			}
		}
	case &s.group:
		pages, err := s.listGroups(ctx, "cleanup-verify-auto-scaling-group-deleted")
		if err != nil {
			return err
		}
		for _, page := range pages {
			for _, item := range page.Items {
				if item.ID == value.id || item.Name == value.name {
					return fmt.Errorf("auto-scaling group %s remains after cleanup", value.id)
				}
			}
		}
	case &s.application:
		pages, err := s.listApplications(ctx, "cleanup-verify-application-deleted")
		if err != nil {
			return err
		}
		for _, page := range pages {
			for _, item := range page.Items {
				if item.ID == value.id || item.Name == value.name && item.ClusterID == s.cluster.id {
					return fmt.Errorf("application %s remains after cleanup", value.id)
				}
			}
		}
	}
	return nil
}

func (s *scenario) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var result error
	if s.application.attempted {
		if err := s.recoverID(ctx, &s.application); err != nil {
			result = errors.Join(result, err)
		} else if s.application.id != "" {
			if err := s.verifyIdentity(ctx, &s.application); err != nil {
				result = errors.Join(result, fmt.Errorf("version cleanup refused: %w", err))
			} else if s.versionAttempted {
				if err := s.deactivateApplication(ctx); err != nil {
					result = errors.Join(result, fmt.Errorf("deactivate application before cleanup: %w", err))
				} else if s.version > 0 {
					_, err := s.call(ctx, "cleanup-delete-version", "apprun-dedicated-api", "version", "delete",
						"--application-id", s.application.id, fmt.Sprint(s.version))
					result = errors.Join(result, err)
					if err == nil {
						data, err := s.call(ctx, "cleanup-verify-version-deleted", "apprun-dedicated-api", "version", "list",
							"--application-id", s.application.id, "--max-items", "100", "--output", "json")
						if err != nil {
							result = errors.Join(result, err)
						} else {
							var page struct {
								Items []struct {
									Version int `json:"version"`
								} `json:"items"`
							}
							if err := json.Unmarshal(data, &page); err != nil {
								result = errors.Join(result, fmt.Errorf("decode remaining versions: %w", err))
							} else {
								for _, version := range page.Items {
									if version.Version == s.version {
										result = errors.Join(result, fmt.Errorf("application version %d remains after cleanup", s.version))
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}
	for _, value := range []*resource{&s.application, &s.group, &s.cluster} {
		result = errors.Join(result, s.deleteResource(ctx, value))
	}
	return result
}

func (s *scenario) run(ctx context.Context) (result error) {
	if err := s.preflight(ctx); err != nil {
		return err
	}
	requestDir, err := os.MkdirTemp("", "skr-e2e-apprun-dedicated-")
	if err != nil {
		return fmt.Errorf("create private request directory: %w", err)
	}
	if err := os.Chmod(requestDir, 0o700); err != nil {
		_ = os.RemoveAll(requestDir)
		return fmt.Errorf("protect private request directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(requestDir); err != nil {
			result = errors.Join(result, fmt.Errorf("remove private request directory: %w", err))
		}
	}()
	if err := s.prepareRequests(requestDir); err != nil {
		return err
	}
	defer func() {
		if err := s.cleanup(); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", err))
		}
	}()
	if err := s.createCluster(ctx); err != nil {
		return err
	}
	if err := s.createGroup(ctx); err != nil {
		return err
	}
	if err := s.createApplication(ctx); err != nil {
		return err
	}
	if err := s.createAndActivateVersion(ctx); err != nil {
		return err
	}
	return s.verifyDeployment(ctx)
}

func randomName() (string, error) {
	var value [4]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate unique E2E resource name: %w", err)
	}
	return namePrefix + hex.EncodeToString(value[:]), nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	clusterRequest := flag.String("cluster-request", "", "Path to private cluster create request JSON")
	groupRequest := flag.String("auto-scaling-group-request", "", "Path to private auto-scaling-group create request JSON")
	versionRequest := flag.String("version-request", "", "Path to private application version create request JSON")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating and deleting a dedicated worker and deploying an application in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *clusterRequest == "" || *groupRequest == "" || *versionRequest == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/apprun-dedicated-api --skr ./skr --cluster-request cluster.json --auto-scaling-group-request group.json --version-request version.json --confirm-is1b-live")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	recorder, err := evidence.New("apprun-dedicated-api")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())
	defer func() {
		result := "failed"
		if exitCode == 0 {
			result = "passed"
		}
		if err := recorder.SetResult(result, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	name, err := randomName()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Resources: %s (zone: %s, one worker node and one application replica)\n", name, zone)
	fmt.Println("The selected SDK profile will be used; confirm its project before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &scenario{
		client:         &cliRunner{binary: path, evidence: recorder},
		clusterRequest: *clusterRequest,
		groupRequest:   *groupRequest,
		versionRequest: *versionRequest,
		cluster:        resource{name: name},
		group:          resource{name: name},
		application:    resource{name: name},
	}
	if err := s.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "AppRun Dedicated E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("AppRun Dedicated E2E passed; the test application, version, auto-scaling group, and cluster were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
