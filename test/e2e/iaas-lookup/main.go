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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	archiveQuery  = "map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})"
	diskPlanQuery = ".DiskPlans | map({ID,Name,StorageClass,Availability,Size})"
)

type cli interface {
	call(context.Context, string, ...string) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The selected binary and structured arguments are not a shell command.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, nil, stdout.String(), stderr.String(), runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type archiveSummary struct {
	ID           int64
	Name         string
	Description  string
	DiskPlanID   int64
	DiskPlanName string
	SizeMB       int
	Tags         []string
}

func (a *archiveSummary) UnmarshalJSON(data []byte) error {
	type summary archiveSummary
	var value summary
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"ID", "Name", "Description", "DiskPlanID", "DiskPlanName", "SizeMB", "Tags"} {
		field, exists := fields[name]
		if !exists || (name != "Tags" && bytes.Equal(bytes.TrimSpace(field), []byte("null"))) {
			return fmt.Errorf("Archive output is missing field %s", name)
		}
	}
	*a = archiveSummary(value)
	return nil
}

type diskPlanSize struct {
	Availability  string
	DisplaySize   int
	DisplaySuffix string
	SizeMB        int
}

type diskPlanSummary struct {
	ID           int64
	Name         string
	StorageClass string
	Availability string
	Size         []diskPlanSize
}

type scenario struct {
	client cli
	zone   string
}

func (s scenario) run(ctx context.Context) error {
	profile, err := s.client.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	if s.zone == "" || s.zone == "all" {
		return errors.New("a single target zone is required")
	}

	archiveArgs := []string{"iaas-api", "archive", "find", "--zone", s.zone, "--query", archiveQuery}
	archiveOutput, err := s.client.call(ctx, "archive-find", archiveArgs...)
	if err != nil {
		return err
	}
	var archives []archiveSummary
	if err := json.Unmarshal(archiveOutput, &archives); err != nil {
		return fmt.Errorf("decode Archive find output: %w", err)
	}
	if len(archives) == 0 || archives[0].ID <= 0 {
		return errors.New("Archive find returned no item with a valid ID")
	}
	archiveReadArgs := []string{"iaas-api", "archive", "read", "--zone", s.zone, "--id", fmt.Sprint(archives[0].ID)}
	archiveOutput, err = s.client.call(ctx, "archive-read", archiveReadArgs...)
	if err != nil {
		return err
	}
	var archive archiveSummary
	if err := json.Unmarshal(archiveOutput, &archive); err != nil {
		return fmt.Errorf("decode Archive read output: %w", err)
	}
	if !sameArchive(archive, archives[0]) {
		return fmt.Errorf("Archive read result does not match find result ID %d", archives[0].ID)
	}

	diskPlanArgs := []string{"iaas-api", "product", "disk", "find", "--zone", s.zone, "--query", diskPlanQuery}
	diskPlanOutput, err := s.client.call(ctx, "disk-plan-find", diskPlanArgs...)
	if err != nil {
		return err
	}
	var diskPlans []diskPlanSummary
	if err := json.Unmarshal(diskPlanOutput, &diskPlans); err != nil {
		return fmt.Errorf("decode DiskPlan find output: %w", err)
	}
	if len(diskPlans) == 0 || diskPlans[0].ID <= 0 {
		return errors.New("DiskPlan find returned no item with a valid ID")
	}
	if len(diskPlans[0].Size) == 0 {
		return errors.New("DiskPlan find returned no size data")
	}
	readArgs := []string{"iaas-api", "product", "disk", "read", "--zone", s.zone, "--id", fmt.Sprint(diskPlans[0].ID)}
	diskPlanOutput, err = s.client.call(ctx, "disk-plan-read", readArgs...)
	if err != nil {
		return err
	}
	var diskPlan diskPlanSummary
	if err := json.Unmarshal(diskPlanOutput, &diskPlan); err != nil {
		return fmt.Errorf("decode DiskPlan read output: %w", err)
	}
	if !sameDiskPlan(diskPlan, diskPlans[0]) {
		return fmt.Errorf("DiskPlan read result does not match find result ID %d", diskPlans[0].ID)
	}
	return nil
}

func sameArchive(left, right archiveSummary) bool {
	if left.ID != right.ID || left.Name != right.Name ||
		left.Description != right.Description || left.DiskPlanID != right.DiskPlanID ||
		left.DiskPlanName != right.DiskPlanName || left.SizeMB != right.SizeMB {
		return false
	}
	leftTags, rightTags := slices.Clone(left.Tags), slices.Clone(right.Tags)
	slices.Sort(leftTags)
	slices.Sort(rightTags)
	return slices.Equal(leftTags, rightTags)
}

func sameDiskPlan(left, right diskPlanSummary) bool {
	if left.ID != right.ID || left.Name != right.Name ||
		left.StorageClass != right.StorageClass || left.Availability != right.Availability ||
		len(left.Size) != len(right.Size) {
		return false
	}
	for i := range left.Size {
		if left.Size[i] != right.Size[i] {
			return false
		}
	}
	return true
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	zone := flag.String("zone", "", "Target IaaS zone for read-only Archive and DiskPlan requests")
	confirmed := flag.Bool("confirm-read-only-live", false, "Confirm read-only Archive and DiskPlan API requests using the selected SDK profile")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *zone == "" || *zone == "all" {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/iaas-lookup --skr ./skr --zone ZONE --confirm-read-only-live")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stat skr binary:", err)
		return 1
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	recorder, err := evidence.New("iaas-lookup-api")
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

	fmt.Printf("This test only searches and reads Archive and DiskPlan resources in %s; it does not modify cloud resources.\n", *zone)
	fmt.Println("Review the selected SDK profile and credentials before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}, zone: *zone}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "IaaS lookup E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("IaaS lookup E2E passed; Archive and DiskPlan search/read results were verified.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
