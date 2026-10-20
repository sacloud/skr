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
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	zone            = "is1b"
	namePrefix      = "skr-e2e-gslb-"
	description     = "Temporary GSLB created by skr live E2E"
	updatedDesc     = "Temporary GSLB updated by skr live E2E"
	projectionQuery = "map({ID,Name,Description,FQDN,DelayLoop,HealthCheck,DestinationServers,SorryServer})"
)

type gslbItem struct {
	ID                 types.ID
	Name               string
	Description        string
	FQDN               string
	DelayLoop          int
	SorryServer        string
	HealthCheck        *iaas.GSLBHealthCheck
	DestinationServers iaas.GSLBServers
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args []string, request any) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeoutDuration := 60 * time.Second
	if strings.HasPrefix(step, "server-") || strings.HasPrefix(step, "sorry-server-") {
		timeoutDuration = 20 * time.Minute
	}
	timeout, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The runner only executes the explicitly selected local skr binary.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, request, stdout.String(), stderr.String(), runErr, false); err != nil {
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("%s: CLI failed: %w (see evidence for stderr)", step, runErr), err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: CLI failed: %w (see evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type scenario struct {
	call          func(context.Context, string, []string, any) ([]byte, error)
	lookupDNS     func(context.Context, string, string) ([]string, error)
	connectSSH    func(context.Context, string, string, string, string) error
	shutdownSSH   func(context.Context, string, string, string, string) error
	serverName    string
	archiveID     int64
	diskPlanID    int64
	diskSizeGB    int64
	diskID        types.ID
	userDataPath  string
	privateKey    string
	knownHosts    string
	pollInterval  time.Duration
	serverTimeout time.Duration
	dnsTimeout    time.Duration
}

func (s scenario) find(ctx context.Context, step, name string) ([]gslbItem, error) {
	args := []string{"gslb-api", "find", "--name", name, "--query", projectionQuery}
	data, err := s.call(ctx, step, args, map[string]any{"Name": name, "Query": projectionQuery})
	if err != nil {
		return nil, err
	}
	var items []gslbItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode GSLB list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) item(ctx context.Context, step string, args []string, request any) (gslbItem, error) {
	data, err := s.call(ctx, step, args, request)
	if err != nil {
		return gslbItem{}, err
	}
	var item gslbItem
	if err := json.Unmarshal(data, &item); err != nil {
		return gslbItem{}, fmt.Errorf("%s: decode GSLB: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return gslbItem{}, fmt.Errorf("%s: GSLB ID is empty", step)
	}
	return item, nil
}

func (s scenario) read(ctx context.Context, step string, id types.ID) (gslbItem, error) {
	return s.item(ctx, step, []string{"gslb-api", "read", "--id", id.String()},
		map[string]any{"ID": id})
}

func checkIdentity(item gslbItem, id types.ID, name string, descriptions ...string) error {
	if item.ID != id || item.Name != name {
		return fmt.Errorf("GSLB identity mismatch for ID %s", id)
	}
	for _, description := range descriptions {
		if item.Description == description {
			return nil
		}
	}
	return fmt.Errorf("GSLB %s has an unexpected E2E marker description", id)
}

func (s scenario) cleanup(name string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id := knownID
	if id.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created GSLB: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() ||
			checkIdentity(matches[0], matches[0].ID, name, description, updatedDesc) != nil {
			return errors.New("cannot safely identify created GSLB; manual cleanup required")
		}
		id = matches[0].ID
	}

	current, err := s.read(ctx, "cleanup-read", id)
	if err != nil {
		return fmt.Errorf("cannot verify GSLB before cleanup: %w", err)
	}
	if err := checkIdentity(current, id, name, description, updatedDesc); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	args := []string{"gslb-api", "delete", "--id", id.String(), "--fail-if-not-found"}
	if _, err := s.call(ctx, "cleanup-delete", args, map[string]any{"ID": id, "FailIfNotFound": true}); err != nil {
		return fmt.Errorf("delete test GSLB %s: %w", id, err)
	}
	remaining, err := s.find(ctx, "cleanup-find-after", name)
	if err != nil {
		return err
	}
	for _, item := range remaining {
		if item.ID == id {
			return fmt.Errorf("GSLB %s remains after delete", id)
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	if _, err := s.call(ctx, "profile-current", []string{"config", "current"}, nil); err != nil {
		return fmt.Errorf("selected profile required: %w", err)
	}
	if err := s.prepareServer(ctx, name); err != nil {
		return err
	}
	var serverID types.ID
	defer func() {
		if cleanupErr := s.cleanupServer(name, serverID); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("server cleanup failed: %w", cleanupErr))
		}
	}()
	server, err := s.createServer(ctx, name)
	if err != nil {
		return err
	}
	serverID = server.ID
	if err := s.captureDisk(ctx, serverID); err != nil {
		return err
	}
	if err := s.bootAndWaitForSSH(ctx, server.ID); err != nil {
		return err
	}
	server, err = s.readServer(ctx, "server-read-up", server.ID)
	if err != nil {
		return err
	}
	serverIP, err := server.sharedIP()
	if err != nil {
		return err
	}

	sorry := s
	sorry.diskID = 0
	sorry.serverName = s.serverName + "-sorry"
	sorry.call = func(ctx context.Context, step string, args []string, request any) ([]byte, error) {
		return s.call(ctx, "sorry-"+step, args, request)
	}
	if err := sorry.prepareServer(ctx, name); err != nil {
		return err
	}
	var sorryID types.ID
	defer func() {
		if cleanupErr := sorry.cleanupServer(name, sorryID); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("sorry server cleanup failed: %w", cleanupErr))
		}
	}()
	sorryServer, err := sorry.createServer(ctx, name)
	if err != nil {
		return err
	}
	sorryID = sorryServer.ID
	if err := sorry.captureDisk(ctx, sorryID); err != nil {
		return err
	}
	if err := sorry.bootAndWaitForSSH(ctx, sorryID); err != nil {
		return err
	}
	sorryServer, err = sorry.readServer(ctx, "server-read-up", sorryID)
	if err != nil {
		return err
	}
	sorryIP, err := sorryServer.sharedIP()
	if err != nil {
		return err
	}
	if sorryIP == serverIP {
		return errors.New("primary and sorry server must have different IP addresses")
	}

	before, err := s.find(ctx, "before-find", name)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return fmt.Errorf("GSLB name %q already exists; refusing to modify it", name)
	}

	var id types.ID
	defer func() {
		if cleanupErr := s.cleanup(name, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	createArgs := []string{
		"gslb-api", "create", "--name", name, "--description", description,
		"--health-check-protocol", "ping", "--delay-loop", "10", "--tags", "skr-e2e",
		"--destination-server", "ip-address=" + serverIP,
		"--sorry-server", sorryIP,
	}
	created, err := s.item(ctx, "test-create", createArgs, map[string]any{
		"Name": name, "Description": description, "HealthCheck": map[string]string{"Protocol": "ping"},
		"DelayLoop": 10, "Tags": []string{"skr-e2e"}, "DestinationServers": []string{serverIP}, "SorryServer": sorryIP,
	})
	if err != nil {
		return err
	}
	id = created.ID
	if err := checkIdentity(created, id, name, description); err != nil {
		return err
	}
	if created.FQDN == "" || created.DelayLoop != 10 || created.HealthCheck == nil ||
		created.HealthCheck.Protocol != types.GSLBHealthCheckProtocols.Ping || len(created.DestinationServers) != 1 ||
		created.DestinationServers[0].IPAddress != serverIP || created.SorryServer != sorryIP {
		return fmt.Errorf("created GSLB %s does not match the requested settings", id)
	}
	if err := s.waitForDNS(ctx, "test-dns-healthy", created.FQDN, serverIP); err != nil {
		return err
	}

	found, err := s.find(ctx, "test-find", name)
	if err != nil {
		return err
	}
	if len(found) != 1 || checkIdentity(found[0], id, name, description) != nil || found[0].FQDN != created.FQDN {
		return fmt.Errorf("find did not return exactly the created GSLB %s", id)
	}

	read, err := s.read(ctx, "test-read", id)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, id, name, description); err != nil {
		return err
	}
	if read.FQDN != created.FQDN || read.DelayLoop != 10 || read.HealthCheck == nil ||
		read.HealthCheck.Protocol != types.GSLBHealthCheckProtocols.Ping || read.SorryServer != sorryIP {
		return fmt.Errorf("read settings do not match created GSLB %s", id)
	}

	if s.shutdownSSH == nil {
		return errors.New("SSH shutdown check is not configured")
	}
	if err := s.shutdownSSH(ctx, "server-shutdown", serverIP, s.privateKey, s.knownHosts); err != nil {
		return err
	}
	if err := s.waitForServerDown(ctx, server.ID); err != nil {
		return err
	}
	if err := sorry.waitForSSH(ctx, sorryID); err != nil {
		return err
	}
	if err := s.waitForDNS(ctx, "test-dns-unhealthy", created.FQDN, sorryIP); err != nil {
		return err
	}
	if err := s.bootServer(ctx, server.ID, false); err != nil {
		return err
	}
	if err := s.waitForSSH(ctx, server.ID); err != nil {
		return err
	}
	if err := s.waitForDNS(ctx, "test-dns-recovered", created.FQDN, serverIP); err != nil {
		return err
	}

	updateArgs := []string{"gslb-api", "update", "--id", id.String(), "--description", updatedDesc, "--delay-loop", "20"}
	updated, err := s.item(ctx, "test-update", updateArgs, map[string]any{
		"ID": id, "Description": updatedDesc, "DelayLoop": 20,
	})
	if err != nil {
		return err
	}
	if err := checkIdentity(updated, id, name, updatedDesc); err != nil {
		return err
	}
	if updated.DelayLoop != 20 || updated.FQDN != created.FQDN || updated.SorryServer != sorryIP {
		return fmt.Errorf("update result does not match requested GSLB settings for %s", id)
	}

	read, err = s.read(ctx, "test-read-updated", id)
	if err != nil {
		return err
	}
	if err := checkIdentity(read, id, name, updatedDesc); err != nil {
		return err
	}
	if read.DelayLoop != 20 || read.FQDN != created.FQDN || read.SorryServer != sorryIP {
		return fmt.Errorf("read did not return updated GSLB %s", id)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	archiveID := flag.Int64("cloudimg-archive-id", 0, "Cloud-init archive ID available in is1b")
	diskPlanID := flag.Int64("disk-plan-id", 0, "Disk plan ID available in is1b")
	diskSizeGB := flag.Int64("disk-size-gb", 20, "Temporary server disk size in GB")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating two temporary Servers with disks and a GSLB in is1b, stopping/restarting the primary, and deleting all test resources")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *archiveID <= 0 || *diskPlanID <= 0 || *diskSizeGB <= 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/gslb --skr ./skr --cloudimg-archive-id ID --disk-plan-id ID [--disk-size-gb 20] --confirm-is1b-live")
		return 2
	}
	for _, program := range []string{"ssh", "ssh-keygen", "dig"} {
		if _, err := exec.LookPath(program); err != nil {
			fmt.Fprintf(os.Stderr, "%s is required for GSLB live E2E: %v\n", program, err)
			return 1
		}
	}

	materialDir, err := os.MkdirTemp("", "skr-e2e-gslb-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temporary SSH material directory:", err)
		return 1
	}
	var recorder *evidence.Recorder
	defer func() {
		exitCode = finalizeRun(exitCode, func() error { return os.RemoveAll(materialDir) }, func(result string) error {
			if recorder == nil {
				return nil
			}
			return recorder.SetResult(result, time.Now())
		}, os.Stderr)
	}()
	userDataPath, privateKey, knownHosts, err := createSSHMaterial(materialDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique GSLB name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffix[:])
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	recorder, err = evidence.New("gslb-api")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := cliRunner{binary: path, evidence: recorder}
	dns := dnsRunner{evidence: recorder}
	ssh := sshRunner{evidence: recorder}
	test := scenario{
		call: runner.call, lookupDNS: dns.lookup, connectSSH: ssh.connect, shutdownSSH: ssh.shutdown,
		serverName: name + "-server", archiveID: *archiveID, diskPlanID: *diskPlanID,
		diskSizeGB: *diskSizeGB, userDataPath: userDataPath, privateKey: privateKey,
		knownHosts: knownHosts, pollInterval: 10 * time.Second,
		serverTimeout: 20 * time.Minute, dnsTimeout: 10 * time.Minute,
	}
	if err := test.run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "GSLB E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("GSLB E2E passed; DNS switched primary -> sorry server -> primary, and GSLB, both Servers and their Disks were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func finalizeRun(exitCode int, cleanup func() error, saveResult func(string) error, stderr io.Writer) int {
	if err := cleanup(); err != nil {
		_, _ = fmt.Fprintln(stderr, "remove temporary SSH material:", err)
		exitCode = 1
	}
	result := "failed"
	if exitCode == 0 {
		result = "passed"
	}
	if err := saveResult(result); err != nil {
		_, _ = fmt.Fprintln(stderr, "save E2E result:", err)
		exitCode = 1
	}
	return exitCode
}

func main() {
	os.Exit(runMain())
}
