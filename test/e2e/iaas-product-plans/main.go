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
	"io"
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
	productPlanZone      = "is1b"
	internetPlanQuery    = "{Total,From,Count,InternetPlans:(if (.InternetPlans | type) == \"array\" then (.InternetPlans | map({ID,Name,BandWidthMbps,Availability})) else error(\"InternetPlans must be an array\") end)}"
	licenseInfoQuery     = "{Total,From,Count,LicenseInfo:(if (.LicenseInfo | type) == \"array\" then (.LicenseInfo | map({ID,Name})) else error(\"LicenseInfo must be an array\") end)}"
	privateHostPlanQuery = "{Total,From,Count,PrivateHostPlans:(if (.PrivateHostPlans | type) == \"array\" then (.PrivateHostPlans | map({ID,Name,Class,CPU,CPUModel,Dedicated,MemoryMB,Availability})) else error(\"PrivateHostPlans must be an array\") end)}"
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
	timeout, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The selected binary and structured arguments are not a shell command.
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, nil, stdout.String(), "", runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: skr failed: %w (stderr is omitted from private evidence)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w", step, runErr)
	}
	return stdout.Bytes(), nil
}

type internetPlanFindSummary struct {
	Total         *int                   `json:"Total"`
	From          *int                   `json:"From"`
	Count         *int                   `json:"Count"`
	InternetPlans *[]internetPlanSummary `json:"InternetPlans"`
}

type internetPlanSummary struct {
	ID            int64
	Name          string
	BandWidthMbps int
	Availability  string
}

type licenseInfoSummary struct {
	ID   int64
	Name string
}

type licenseInfoFindSummary struct {
	Total       *int                  `json:"Total"`
	From        *int                  `json:"From"`
	Count       *int                  `json:"Count"`
	LicenseInfo *[]licenseInfoSummary `json:"LicenseInfo"`
}

type privateHostPlanFindSummary struct {
	Total            *int                      `json:"Total"`
	From             *int                      `json:"From"`
	Count            *int                      `json:"Count"`
	PrivateHostPlans *[]privateHostPlanSummary `json:"PrivateHostPlans"`
}

type privateHostPlanSummary struct {
	ID           int64
	Name         string
	Class        string
	CPU          int
	CPUModel     string
	Dedicated    bool
	MemoryMB     int
	Availability string
}

type scenario struct {
	client cli
}

func (s scenario) run(ctx context.Context) error {
	profile, err := s.client.call(ctx, "profile-current", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}

	output, err := s.client.call(ctx, "internetplan-find", "iaas-api", "product", "internet", "find",
		"--zone", productPlanZone, "--count", "1", "--from", "0", "--query", internetPlanQuery)
	if err != nil {
		return err
	}
	var internetFind internetPlanFindSummary
	if err := json.Unmarshal(output, &internetFind); err != nil {
		return fmt.Errorf("decode InternetPlan find output: %w", err)
	}
	if err := validateFindResult(internetFind.Total, internetFind.From, internetFind.Count, sliceLength(internetFind.InternetPlans), 0, 1); err != nil {
		return fmt.Errorf("InternetPlan Find returned an inconsistent result: %w", err)
	}
	internetPlans := *internetFind.InternetPlans
	if len(internetPlans) != 1 || internetPlans[0].ID <= 0 || internetPlans[0].Name == "" {
		return fmt.Errorf("InternetPlan Find returned %d records or an invalid ID/name; want one plan", len(internetPlans))
	}
	internetPlan := internetPlans[0]
	output, err = s.client.call(ctx, "internetplan-read", "iaas-api", "product", "internet", "read",
		"--zone", productPlanZone, "--id", fmt.Sprint(internetPlan.ID))
	if err != nil {
		return err
	}
	var internetRead internetPlanSummary
	if err := json.Unmarshal(output, &internetRead); err != nil {
		return fmt.Errorf("decode InternetPlan read output: %w", err)
	}
	if internetRead != internetPlan {
		return fmt.Errorf("InternetPlan Read did not match Find result for ID %d", internetPlan.ID)
	}

	output, err = s.client.call(ctx, "licenseinfo-find", "iaas-api", "product", "license", "find",
		"--count", "1", "--from", "0", "--query", licenseInfoQuery)
	if err != nil {
		return err
	}
	var licenseFind licenseInfoFindSummary
	if err := json.Unmarshal(output, &licenseFind); err != nil {
		return fmt.Errorf("decode LicenseInfo find output: %w", err)
	}
	if err := validateFindResult(licenseFind.Total, licenseFind.From, licenseFind.Count, sliceLength(licenseFind.LicenseInfo), 0, 1); err != nil {
		return fmt.Errorf("LicenseInfo Find returned an inconsistent result: %w", err)
	}
	for _, license := range *licenseFind.LicenseInfo {
		if license.ID <= 0 || license.Name == "" {
			return errors.New("LicenseInfo Find returned an item without a valid ID or name")
		}
	}

	output, err = s.client.call(ctx, "privatehostplan-find", "iaas-api", "product", "privatehost", "find",
		"--zone", productPlanZone, "--count", "1", "--from", "0", "--query", privateHostPlanQuery)
	if err != nil {
		return err
	}
	var privateHostFind privateHostPlanFindSummary
	if err := json.Unmarshal(output, &privateHostFind); err != nil {
		return fmt.Errorf("decode PrivateHostPlan find output: %w", err)
	}
	if err := validateFindResult(privateHostFind.Total, privateHostFind.From, privateHostFind.Count, sliceLength(privateHostFind.PrivateHostPlans), 0, 1); err != nil {
		return fmt.Errorf("PrivateHostPlan Find returned an inconsistent result: %w", err)
	}
	privateHostPlans := *privateHostFind.PrivateHostPlans
	if len(privateHostPlans) != 1 || privateHostPlans[0].ID <= 0 || privateHostPlans[0].Name == "" {
		return fmt.Errorf("PrivateHostPlan Find returned %d records or an invalid ID/name; want one plan", len(privateHostPlans))
	}
	privateHostPlan := privateHostPlans[0]
	output, err = s.client.call(ctx, "privatehostplan-read", "iaas-api", "product", "privatehost", "read",
		"--zone", productPlanZone, "--id", fmt.Sprint(privateHostPlan.ID))
	if err != nil {
		return err
	}
	var privateHostRead privateHostPlanSummary
	if err := json.Unmarshal(output, &privateHostRead); err != nil {
		return fmt.Errorf("decode PrivateHostPlan read output: %w", err)
	}
	if privateHostRead != privateHostPlan {
		return fmt.Errorf("PrivateHostPlan Read did not match Find result for ID %d", privateHostPlan.ID)
	}
	return nil
}

func sliceLength[T any](items *[]T) int {
	if items == nil {
		return -1
	}
	return len(*items)
}

func validateFindResult(total, from, count *int, itemCount, expectedFrom, requestedCount int) error {
	if total == nil || from == nil || count == nil || itemCount < 0 {
		return errors.New("required pagination fields or result array are missing or null")
	}
	if *total < 0 || *from != expectedFrom || *count < 0 || *count > requestedCount ||
		*from > *total || *count > *total-*from || itemCount != *count ||
		(*count == 0 && *total > *from) {
		return fmt.Errorf("pagination metadata does not match the requested page (total=%d, from=%d, count=%d, items=%d)", *total, *from, *count, itemCount)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	confirmed := flag.Bool("confirm-read-only-live", false, "Confirm read-only InternetPlan, LicenseInfo, and PrivateHostPlan API requests")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/iaas-product-plans --skr ./skr --confirm-read-only-live")
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
	recorder, err := evidence.New("iaas-product-plans-api", evidence.Scenario{
		Title:       "IaaS 製品プランの検索",
		Description: "IaaS API でインターネット、ライセンス、プライベートホストの製品プラン検索・参照が正しく動作することを確認します。",
	})
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

	fmt.Printf("This test only searches and reads InternetPlan and PrivateHostPlan in %s and searches LicenseInfo; it does not modify cloud resources.\n", productPlanZone)
	fmt.Println("LicenseInfo does not require a zone. Review the selected SDK profile and credentials before proceeding.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := (scenario{client: cliRunner{binary: path, evidence: recorder}}).run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "IaaS product-plan E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("IaaS product-plan E2E passed; Find results matched Read for InternetPlan and PrivateHostPlan, and LicenseInfo returned a valid Find response.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
