// Copyright 2022-2026 The sacloud/skr Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/sacloud/skr/internal/objectstorageapi"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

type accountCLI struct {
	fakeCLI
	exists bool
	id     string
	deny   bool
}

func notFound(method string) error {
	return &commandError{step: "test", stderr: "object-storage: " + method + ": API Error 404: Not found", cause: errors.New("exit status 1")}
}

func (f *accountCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = map[string][]string{}
	}
	f.args[step] = append([]string(nil), args...)
	switch step {
	case "account-site-plans":
		return []byte(`[{"service_class_path":"objectstorage/arc02/bucket/1t","type":"archive","cluster_id":"arc02"}]`), nil
	case "account-preflight", "account-identity-check", "account-verify-delete":
		if f.deny {
			return nil, &commandError{step: step, stderr: "object-storage: Accounts.Read: API Error 403: Access denied", cause: errors.New("denied")}
		}
		if !f.exists {
			return nil, notFound("Accounts.Read")
		}
		return json.Marshal(account{ResourceID: f.id})
	case "account-create":
		f.exists = true
		f.id = "created-account-1"
		return json.Marshal(account{ResourceID: f.id})
	case "account-delete":
		f.exists = false
		return nil, nil
	case "account-cleanup-buckets", "account-cleanup-permissions", "account-cleanup-keys",
		"account-key-preflight", "account-key-cleanup-check", "account-key-verify-delete":
		if output, ok := f.responses[step]; ok {
			return output, nil
		}
		if strings.HasPrefix(step, "account-key") && f.key {
			return []byte(`[{"id":"account-key-1"}]`), nil
		}
		return []byte(`[]`), nil
	case "account-key-create":
		f.key = true
		return []byte(`{"id":"account-key-1","secret":"test-only-secret"}`), nil
	case "account-key-read":
		return []byte(`{"id":"account-key-1","secret":""}`), nil
	case "account-key-delete":
		f.key = false
		return nil, nil
	}
	return f.fakeCLI.call(ctx, step, args...)
}

func TestAccountLifecycleAndSafety(t *testing.T) {
	t.Run("new account and key", func(t *testing.T) {
		fake := &accountCLI{}
		if err := (scenario{client: fake, siteID: "arc02"}).runAccount(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fake.exists || fake.key {
			t.Fatal("new account or key remains")
		}
	})
	for _, test := range []struct {
		name string
		fake accountCLI
	}{
		{"existing", accountCLI{exists: true, id: "existing-account"}},
		{"forbidden is not absence", accountCLI{deny: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := (scenario{client: &test.fake, siteID: "arc02"}).runAccount(context.Background())
			if err == nil {
				t.Fatal("expected preflight refusal")
			}
			for _, step := range test.fake.calls {
				if step == "account-create" || step == "account-delete" {
					t.Fatal("account modified after failing preflight")
				}
			}
		})
	}
	t.Run("different ID", func(t *testing.T) {
		fake := &accountCLI{exists: true, id: "replacement"}
		err := (scenario{client: fake, siteID: "arc02"}).deleteAccount(context.Background(), account{ResourceID: "created"})
		if err == nil || !strings.Contains(err.Error(), "ID changed") {
			t.Fatalf("error = %v", err)
		}
		if _, ok := fake.args["account-delete"]; ok {
			t.Fatal("deleted replaced account")
		}
	})
	t.Run("nonempty account", func(t *testing.T) {
		fake := &accountCLI{exists: true, id: "created", fakeCLI: fakeCLI{responses: map[string][]byte{
			"account-cleanup-buckets": []byte(`[{"name":"someone-elses-bucket","resource_id":"other"}]`),
		}}}
		err := (scenario{client: fake, siteID: "arc02"}).deleteAccount(context.Background(), account{ResourceID: "created"})
		if err == nil || !strings.Contains(err.Error(), "contains resources") {
			t.Fatalf("error = %v", err)
		}
		if _, ok := fake.args["account-delete"]; ok {
			t.Fatal("deleted nonempty account")
		}
	})
}

type advancedCLI struct {
	fakeCLI
	encryption  string
	replication bool
}

func (f *advancedCLI) call(ctx context.Context, step string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, step)
	if f.args == nil {
		f.args = map[string][]string{}
	}
	f.args[step] = append([]string(nil), args...)
	switch step {
	case "encryption-read":
		if f.encryption == "" {
			return nil, notFound("BucketExtra.ReadEncryption")
		}
		return json.Marshal(map[string]string{"kms_key_id": f.encryption})
	case "encryption-enable":
		f.encryption = args[len(args)-1]
		return nil, nil
	case "encryption-disable":
		f.encryption = ""
		return nil, nil
	case "replication-read":
		if !f.replication {
			return nil, notFound("BucketExtra.ReadReplication")
		}
		return []byte(fakeReplication), nil
	case "replication-enable":
		f.replication = true
		return []byte(fakeReplication), nil
	case "replication-disable":
		f.replication = false
		return nil, nil
	}
	return f.fakeCLI.call(ctx, step, args...)
}

const fakeReplication = `{"source_bucket":{"name":"source","cluster_id":"isk01"},"dest_bucket":{"name":"target","cluster_id":"tky01"},"config_status":"created"}`

func TestAdvancedLifecycle(t *testing.T) {
	fake := &advancedCLI{}
	s := scenario{client: fake, siteID: "tky01"}
	if err := s.runEncryption(context.Background(), "new-bucket", "kms-key-1"); err != nil {
		t.Fatal(err)
	}
	s.siteID = "isk01"
	if err := s.runReplication(context.Background(), "source", "target", "tky01"); err != nil {
		t.Fatal(err)
	}
	if fake.encryption != "" || fake.replication {
		t.Fatal("settings remain after cleanup")
	}
	if err := checkReplication([]byte(fakeReplication), "source", "isk01", "different", "tky01"); err == nil {
		t.Fatal("accepted wrong replication target")
	}
}

func TestArchivePlanValidation(t *testing.T) {
	fake := &fakeCLI{}
	s := scenario{client: fake, siteID: "arc02", plan: "1t"}
	if err := s.checkSite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.checkBucketPlan(bucket{Plan: json.RawMessage(`{"type":"archive","service_class_path":"objectstorage/arc02/bucket/1t"}`)}); err != nil {
		t.Fatal(err)
	}
	s.plan = "unavailable"
	if err := s.checkSite(context.Background()); err == nil {
		t.Fatal("accepted unavailable plan")
	}
}

func TestFailureCleanupAndSecretFileRemoval(t *testing.T) {
	for _, step := range []string{"permission-key-read", "permission-update", "bucket-usage", "bucket-metering"} {
		t.Run(step, func(t *testing.T) {
			fake := &fakeCLI{failStep: step}
			s := scenario{client: fake, siteID: "isk01"}
			s.extra = s.readMetrics
			err := s.run(context.Background(), "skr-e2e-0123456789abcdef")
			if err == nil {
				t.Fatal("expected failure")
			}
			if fake.permission || fake.key || !fake.bucketGone {
				t.Fatalf("resources remain after %s: permission %v key %v bucket %v", step, fake.permission, fake.key, !fake.bucketGone)
			}
			if args := fake.args["permission-create"]; len(args) > 0 {
				path := strings.TrimPrefix(args[len(args)-1], "@")
				if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("private temporary directory remains: %v", err)
				}
			}
		})
	}
}

func TestBucketListMayOmitPlan(t *testing.T) {
	fake := &fakeCLI{verifyCreate: []byte(`[{"name":"skr-e2e-0123456789abcdef","resource_id":"bucket-resource","plan":null}]`)}
	if err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRefusesChangedPermissionAndRetainsErrors(t *testing.T) {
	t.Run("different permission contents", func(t *testing.T) {
		fake := &fakeCLI{responses: map[string][]byte{
			"permission-cleanup-presence": []byte(`[{"id":1001,"display_name":"replaced","bucket_controls":[]}]`),
			"permission-cleanup-check":    []byte(`[{"id":1001,"display_name":"replaced","bucket_controls":[]}]`),
		}}
		expected := permission{ID: 1001, DisplayName: "ours", Controls: []bucketControl{{BucketName: "ours", CanRead: true}}}
		err := (scenario{client: fake, siteID: "isk01"}).deletePermission(context.Background(), expected)
		if err == nil {
			t.Fatal("expected identity refusal")
		}
		if _, ok := fake.args["permission-delete"]; ok {
			t.Fatal("deleted changed permission")
		}
	})
	t.Run("key cleanup failure", func(t *testing.T) {
		fake := &fakeCLI{failStep: "permission-key-delete"}
		err := (scenario{client: fake, siteID: "isk01"}).run(context.Background(), "skr-e2e-0123456789abcdef")
		if err == nil || !strings.Contains(err.Error(), "injected CLI failure") {
			t.Fatalf("cleanup failure was lost: %v", err)
		}
		if _, ok := fake.args["permission-delete"]; ok {
			t.Fatal("deleted permission while key cleanup failed")
		}
	})
	t.Run("unknown account status is not not-found", func(t *testing.T) {
		for _, stderr := range []string{
			"object-storage: Accounts.Read: API Error 403: denied",
			"object-storage: Accounts.Read: API Error: decode response: unexpected status code: 404",
			"object-storage: Accounts.Read: API Error 4040: invalid",
		} {
			if accountNotFound(&commandError{stderr: stderr, cause: errors.New("failed")}) {
				t.Fatalf("treated ambiguous error as absence: %s", stderr)
			}
		}
		if accountNotFound(&commandError{stderr: "object-storage: Accounts.Read: API Error 404: missing", cause: errors.New("failed"), recordFailed: true}) {
			t.Fatal("accepted absence after evidence write failure")
		}
	})
}

func TestScenarioOptionsRequireExplicitDependenciesAndConfirmations(t *testing.T) {
	for _, test := range []struct {
		mode      string
		site      string
		plan      string
		bucket    bool
		account   bool
		wantError bool
	}{
		{"tutorial", "isk01", "", true, false, false},
		{"metrics", "isk01", "", false, false, false},
		{"metrics", "tky01", "", true, true, false},
		{"metrics", "isk01", "", false, true, true},
		{"metrics", "isk01", "1t", true, false, true},
		{"tutorial", "isk01", "", false, false, true},
		{"archive", "arc02", "1t", true, false, false},
		{"archive", "arc02", "", true, false, true},
		{"archive", "arc02", "bad/path", true, false, true},
		{"account", "arc02", "", false, true, false},
		{"account", "arc02", "", false, false, true},
		{"account", "arc02", "", true, true, true},
		{"unknown", "isk01", "", true, false, true},
	} {
		err := validateOptions(test.mode, test.site, test.plan, test.bucket, test.account)
		if (err != nil) != test.wantError {
			t.Errorf("%+v: error = %v", test, err)
		}
	}
	for _, test := range []struct {
		mode, site, key, target string
		wantError               bool
	}{
		{"encryption", "tky01", "key-id", "", false},
		{"encryption", "isk01", "key-id", "", true},
		{"encryption", "tky01", "", "", true},
		{"replication", "isk01", "", "tky01", false},
		{"replication", "isk01", "", "isk01", true},
		{"replication", "isk01", "", "", true},
		{"tutorial", "isk01", "key-id", "", true},
	} {
		err := validateDependencies(test.mode, test.site, test.key, test.target)
		if (err != nil) != test.wantError {
			t.Errorf("%+v: error = %v", test, err)
		}
	}
}

func TestMetricsTargetAndReadOnlyIdentity(t *testing.T) {
	for _, test := range []struct {
		mode, name, id string
		create, bad    bool
	}{
		{"metrics", "existing", "bucket-resource", false, false},
		{"metrics", "", "", true, false},
		{"metrics", "", "", false, true},
		{"metrics", "existing", "", false, true},
		{"metrics", "", "bucket-resource", false, true},
		{"metrics", "existing", "bucket-resource", true, true},
		{"tutorial", "existing", "bucket-resource", false, true},
	} {
		if err := validateMetricsTarget(test.mode, test.name, test.id, test.create); (err != nil) != test.bad {
			t.Errorf("%+v: error = %v", test, err)
		}
	}
	for _, id := range []string{"bucket-resource", "replacement"} {
		fake := &fakeCLI{responses: map[string][]byte{
			"metrics-bucket-identity": []byte(`[{"name":"existing","resource_id":"bucket-resource"}]`),
		}}
		err := (scenario{client: fake, siteID: "isk01"}).runMetrics(context.Background(), "existing", id)
		if (err != nil) != (id == "replacement") {
			t.Fatalf("id %s: error = %v", id, err)
		}
		for _, args := range fake.args {
			for _, arg := range args {
				if arg == "create" || arg == "delete" || arg == "update" {
					t.Fatalf("read-only metrics modified resources: %v", args)
				}
			}
		}
		if _, called := fake.args["bucket-usage"]; called != (id == "bucket-resource") {
			t.Fatal("metrics must only run after identity matches")
		}
	}
}

func TestStrictArrayResponsesAndCancellation(t *testing.T) {
	for _, input := range []string{"null", "{}", `{"items":[]}`, `[] {}`, `[null`, `[null]`, `["not a bucket"]`} {
		if _, err := decodeArray[bucket]([]byte(input), "bucket list"); err == nil {
			t.Fatalf("accepted invalid array response: %s", input)
		}
	}
	if _, err := decodeArray[bucket]([]byte("[]"), "bucket list"); err != nil {
		t.Fatal(err)
	}
	fake := &accountCLI{exists: true, id: "created", fakeCLI: fakeCLI{responses: map[string][]byte{
		"account-cleanup-buckets": []byte(`null`),
	}}}
	if err := (scenario{client: fake, siteID: "arc02"}).deleteAccount(context.Background(), account{ResourceID: "created"}); err == nil {
		t.Fatal("treated null bucket list as an empty account")
	}
	if _, deleted := fake.args["account-delete"]; deleted {
		t.Fatal("deleted account after invalid list")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitFor(ctx, func(context.Context) (bool, error) {
		t.Fatal("called API after cancellation")
		return true, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestMetricsCollectsFailuresWithoutSkippingOtherCommands(t *testing.T) {
	fake := &fakeCLI{failStep: "bucket-penalty"}
	err := (scenario{client: fake, siteID: "tky01"}).readMetrics(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("metrics failure was lost")
	}
	for _, step := range []string{"site-status-read", "site-quota", "bucket-penalty", "bucket-usage", "bucket-quota", "bucket-metering"} {
		if _, called := fake.args[step]; !called {
			t.Errorf("skipped metrics step %s", step)
		}
	}
	for _, invalid := range []string{"null", "[]", `[{"year_month":"invalid"}]`, `[{"year_month":"2000-01"},{"year_month":"2000-01"}]`} {
		fake := &fakeCLI{responses: map[string][]byte{"bucket-metering": []byte(invalid)}}
		if err := (scenario{client: fake, siteID: "tky01"}).readMetrics(context.Background(), "test-bucket"); err == nil {
			t.Fatalf("accepted invalid metering: %s", invalid)
		}
	}
}

func TestAllObjectStorageLeafCommandsHaveScenarioCalls(t *testing.T) {
	basic := &fakeCLI{}
	s := scenario{client: basic, siteID: "isk01"}
	s.extra = s.readMetrics
	if err := s.run(context.Background(), "skr-e2e-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	archive := &fakeCLI{}
	if err := (scenario{client: archive, siteID: "arc02", plan: "1t"}).checkSite(context.Background()); err != nil {
		t.Fatal(err)
	}
	account := &accountCLI{}
	if err := (scenario{client: account, siteID: "arc02"}).runAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	extra := &advancedCLI{}
	if err := (scenario{client: extra, siteID: "tky01"}).runEncryption(context.Background(), "new-bucket", "key-1"); err != nil {
		t.Fatal(err)
	}
	if err := (scenario{client: extra, siteID: "isk01"}).runReplication(context.Background(), "source", "target", "tky01"); err != nil {
		t.Fatal(err)
	}
	commands := objectstorageapi.NewCommands(objectstorageapi.Runtime{}, nil)
	parser, err := kong.New(&commands)
	if err != nil {
		t.Fatal(err)
	}
	var leaves []string
	var visit func(*kong.Node, []string)
	visit = func(node *kong.Node, path []string) {
		for _, child := range node.Children {
			if child.Type == kong.CommandNode {
				visit(child, append(append([]string{}, path...), child.Name))
			}
		}
		if node.Leaf() && node.Type == kong.CommandNode {
			leaves = append(leaves, strings.Join(path, " "))
		}
	}
	visit(parser.Model.Node, nil)
	if len(leaves) < 30 {
		t.Fatalf("unexpected command tree size: %d", len(leaves))
	}
	t.Logf("checking scenario coverage for %d leaf commands", len(leaves))
	for _, leaf := range leaves {
		found := false
		for _, calls := range []map[string][]string{basic.args, archive.args, account.args, extra.args} {
			for _, args := range calls {
				if strings.HasPrefix(strings.Join(args, " ")+" ", "object-storage-api "+leaf+" ") {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no successful scenario call for object-storage-api %s", leaf)
		}
	}
}

func TestCLIHelperSecretOutput(t *testing.T) {
	if os.Getenv("SKR_TEST_SECRET_HELPER") != "1" {
		return
	}
	if _, err := fmt.Fprint(os.Stdout, `{"id":"key-1","secret":"test-only-secret"}`); err != nil {
		os.Exit(2)
	}
	if _, err := fmt.Fprint(os.Stderr, "test-only-secret"); err != nil {
		os.Exit(2)
	}
	os.Exit(1)
}

func TestAccessKeyEvidenceRedactsBothStreams(t *testing.T) {
	t.Setenv("SKR_TEST_SECRET_HELPER", "1")
	recorder, err := evidence.CreateAt(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runner := cliRunner{binary: os.Args[0], evidence: recorder}
	_, err = runner.call(context.Background(), "permission-key-create", "-test.run=TestCLIHelperSecretOutput", "access-key")
	if err == nil || strings.Contains(err.Error(), "test-only-secret") {
		t.Fatalf("error missing or leaked secret: %v", err)
	}
	if err := recorder.SetResult("failed", time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(recorder.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(recorder.Dir(), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "test-only-secret") {
			t.Fatalf("secret leaked into %s", entry.Name())
		}
	}
}
