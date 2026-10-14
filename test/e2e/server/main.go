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
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	zone           = "is1b"
	namePrefix     = "skr-e2e-server-"
	description    = "Temporary Server for skr cloud-init E2E"
	sshUser        = "skr-e2e"
	sshWaitTimeout = 20 * time.Minute
)

var errSSHEvidence = errors.New("record SSH evidence")

type serverInterface struct {
	IPAddress    string                     `json:"IPAddress"`
	UpstreamType types.EUpstreamNetworkType `json:"UpstreamType"`
}

type serverItem struct {
	ID             types.ID                    `json:"ID"`
	Name           string                      `json:"Name"`
	Description    string                      `json:"Description"`
	InstanceStatus types.EServerInstanceStatus `json:"InstanceStatus"`
	Interfaces     []serverInterface           `json:"Interfaces"`
}

type cli interface {
	call(context.Context, string, map[string]any) ([]byte, error)
}

type cliRunner struct {
	binary       string
	evidence     *evidence.Recorder
	userDataPath string
}

func (r cliRunner) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	args, err := serverArgs(step, request, r.userDataPath)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The caller selects a local skr binary; structured arguments are not passed through a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, request, stdout.String(), stderr.String(), runErr, false); err != nil {
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

func serverArgs(step string, request map[string]any, userDataPath string) ([]string, error) {
	if step == "profile-current" {
		return []string{"config", "current"}, nil
	}
	if request == nil {
		return nil, fmt.Errorf("%s: missing request", step)
	}
	requestZone, ok := request["Zone"].(string)
	if !ok || requestZone != zone {
		return nil, fmt.Errorf("%s: expected Zone %q", step, zone)
	}
	switch step {
	case "preflight-find-server", "cleanup-find-server", "cleanup-find-server-after":
		return findArgs("server", request)
	case "preflight-find-disk", "cleanup-find-disk-after":
		return findArgs("disk", request)
	case "test-create":
		name, err := requestString(request, "Name")
		if err != nil {
			return nil, err
		}
		archiveID, err := requestInt64(request, "CloudimgArchiveID")
		if err != nil {
			return nil, err
		}
		diskPlanID, err := requestInt64(request, "DiskPlanID")
		if err != nil {
			return nil, err
		}
		diskSizeGB, err := requestInt64(request, "DiskSizeGB")
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "server", "create",
			"--zone", zone, "--name", name, "--description", description,
			"--cpu", "1", "--memory-gb", "1",
			"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", archiveID, diskPlanID, diskSizeGB),
			"--nic", "upstream=shared",
		}, nil
	case "test-boot":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		requestPath, err := requestString(request, "UserDataPath")
		if err != nil {
			return nil, err
		}
		if userDataPath == "" || requestPath != userDataPath {
			return nil, errors.New("user-data file path is required")
		}
		return []string{"iaas-api", "server", "boot", "--zone", zone, "--id", id, "--user-data", "@" + userDataPath}, nil
	case "test-read", "cleanup-read":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "server", "read", "--zone", zone, "--id", id,
			"--query", "{ID,Name,Description,InstanceStatus,Interfaces}",
		}, nil
	case "cleanup-delete":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "server", "delete", "--zone", zone, "--id", id,
			"--with-disks", "--force", "--fail-if-not-found",
		}, nil
	default:
		return nil, fmt.Errorf("unknown Server E2E step %q", step)
	}
}

func findArgs(resource string, request map[string]any) ([]string, error) {
	name, err := requestString(request, "Name")
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]any{"Zone": zone, "Names": []string{name}})
	if err != nil {
		return nil, fmt.Errorf("encode %s find request: %w", resource, err)
	}
	return []string{"iaas-api", resource, "find", "--request", string(data)}, nil
}

func requestString(request map[string]any, key string) (string, error) {
	value, ok := request[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("missing %s", key)
	}
	return value, nil
}

func requestInt64(request map[string]any, key string) (int64, error) {
	switch value := request[key].(type) {
	case int:
		if value > 0 {
			return int64(value), nil
		}
	case int64:
		if value > 0 {
			return value, nil
		}
	}
	return 0, fmt.Errorf("%s must be a positive integer", key)
}

func requestID(request map[string]any) (string, error) {
	value, ok := request["ID"]
	if !ok {
		return "", errors.New("missing ID")
	}
	id := fmt.Sprint(value)
	if id == "" || id == "0" {
		return "", errors.New("invalid ID")
	}
	return id, nil
}

type scenario struct {
	client       cli
	archiveID    int64
	diskPlanID   int64
	diskSizeGB   int64
	userDataPath string
	privateKey   string
	knownHosts   string
	connect      func(context.Context, string, string, string, string) error
	pollInterval time.Duration
	sshTimeout   time.Duration
}

func (s scenario) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	return s.client.call(ctx, step, request)
}

func (s scenario) find(ctx context.Context, step, resource, name string) ([]serverItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "Name": name})
	if err != nil {
		return nil, err
	}
	if resource == "disk" {
		var disks []struct {
			ID   types.ID `json:"ID"`
			Name string   `json:"Name"`
		}
		if err := json.Unmarshal(output, &disks); err != nil {
			return nil, fmt.Errorf("%s: decode Disk list: %w", step, err)
		}
		if disks == nil {
			return nil, fmt.Errorf("%s: expected a JSON array", step)
		}
		items := make([]serverItem, 0, len(disks))
		for _, disk := range disks {
			items = append(items, serverItem{ID: disk.ID, Name: disk.Name})
		}
		return items, nil
	}
	var items []serverItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Server list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) read(ctx context.Context, step string, id types.ID) (serverItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "ID": id})
	if err != nil {
		return serverItem{}, err
	}
	var item serverItem
	if err := json.Unmarshal(output, &item); err != nil {
		return serverItem{}, fmt.Errorf("%s: decode Server: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return serverItem{}, fmt.Errorf("%s: Server ID is empty", step)
	}
	return item, nil
}

func checkServer(item serverItem, id types.ID, name string) error {
	if item.ID != id || item.Name != name || item.Description != description {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	return nil
}

func (s scenario) publicIP(ctx context.Context, id types.ID, name string) (string, error) {
	item, err := s.read(ctx, "test-read", id)
	if err != nil {
		return "", err
	}
	if err := checkServer(item, id, name); err != nil {
		return "", err
	}
	if !item.InstanceStatus.IsUp() {
		return "", nil
	}
	for _, iface := range item.Interfaces {
		if iface.UpstreamType != "shared" || net.ParseIP(iface.IPAddress) == nil {
			continue
		}
		return iface.IPAddress, nil
	}
	return "", nil
}

func (s scenario) waitForSSH(ctx context.Context, id types.ID, name string) error {
	if s.connect == nil {
		return errors.New("SSH connection check is not configured")
	}
	timeout := s.sshTimeout
	if timeout == 0 {
		timeout = sshWaitTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	pollInterval := s.pollInterval
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		ip, err := s.publicIP(ctx, id, name)
		if err != nil {
			return err
		}
		if ip != "" {
			err := s.connect(ctx, sshUser, ip, s.privateKey, s.knownHosts)
			if err == nil {
				return nil
			}
			if errors.Is(err, errSSHEvidence) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("SSH login check timed out: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("SSH login check timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func (s scenario) cleanup(name string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	id := knownID
	if id.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find-server", "server", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Server: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != description {
			return errors.New("cannot safely identify created Server; manual cleanup required")
		}
		id = matches[0].ID
	}
	current, err := s.read(ctx, "cleanup-read", id)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	if err := checkServer(current, id, name); err != nil {
		return fmt.Errorf("cleanup refused: Server identity changed; manual cleanup required: %w", err)
	}
	if _, err := s.call(ctx, "cleanup-delete", map[string]any{"Zone": zone, "ID": id}); err != nil {
		return fmt.Errorf("delete test Server %s: %w", id, err)
	}
	for _, resource := range []struct {
		step, kind string
	}{
		{"cleanup-find-server-after", "server"},
		{"cleanup-find-disk-after", "disk"},
	} {
		matches, err := s.find(ctx, resource.step, resource.kind, name)
		if err != nil {
			return err
		}
		if len(matches) != 0 {
			return fmt.Errorf("%s %q remains after Server cleanup", resource.kind, name)
		}
	}
	return nil
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	if _, err := s.call(ctx, "profile-current", nil); err != nil {
		return fmt.Errorf("selected SDK profile required: %w", err)
	}
	for _, resource := range []string{"server", "disk"} {
		matches, err := s.find(ctx, "preflight-find-"+resource, resource, name)
		if err != nil {
			return err
		}
		if len(matches) != 0 {
			return fmt.Errorf("test %s name %q already exists; refusing to modify it", resource, name)
		}
	}
	var id types.ID
	defer func() {
		if cleanupErr := s.cleanup(name, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()
	created, err := s.call(ctx, "test-create", map[string]any{
		"Zone": zone, "Name": name, "CloudimgArchiveID": s.archiveID,
		"DiskPlanID": s.diskPlanID, "DiskSizeGB": s.diskSizeGB,
	})
	if err != nil {
		return err
	}
	var server serverItem
	if err := json.Unmarshal(created, &server); err != nil {
		return fmt.Errorf("decode created Server: %w", err)
	}
	id = server.ID
	if id.IsEmpty() {
		return errors.New("created Server ID is empty")
	}
	if err := checkServer(server, id, name); err != nil {
		return err
	}
	if _, err := s.call(ctx, "test-boot", map[string]any{
		"Zone": zone, "ID": id, "UserDataPath": s.userDataPath,
	}); err != nil {
		return err
	}
	if err := s.waitForSSH(ctx, id, name); err != nil {
		return err
	}
	return nil
}

func createSSHMaterial(dir string) (userDataPath, privateKey, knownHosts string, err error) {
	privateKey = filepath.Join(dir, "ssh-key")
	keygen := exec.Command("ssh-keygen", "-t", "rsa", "-b", "3072", "-N", "", "-f", privateKey) //nolint:gosec // Generates an ephemeral local test key without a shell.
	keygen.Stdout = nil
	keygen.Stderr = nil
	if err := keygen.Run(); err != nil {
		return "", "", "", fmt.Errorf("generate temporary SSH key: %w", err)
	}
	if err := os.Chmod(privateKey, 0o600); err != nil {
		return "", "", "", fmt.Errorf("protect temporary private SSH key: %w", err)
	}
	//nolint:gosec // privateKey is created inside this runner's private temporary directory.
	publicKey, err := os.ReadFile(privateKey + ".pub")
	if err != nil {
		return "", "", "", fmt.Errorf("read temporary public SSH key: %w", err)
	}
	userDataPath = filepath.Join(dir, "user-data.yaml")
	userData := fmt.Sprintf("#cloud-config\nusers:\n  - default\n  - name: %s\n    shell: /bin/bash\n    ssh_authorized_keys:\n      - %s\n", sshUser, strings.TrimSpace(string(publicKey)))
	//nolint:gosec // userDataPath is inside the private temporary directory created by this runner.
	if err := os.WriteFile(userDataPath, []byte(userData), 0o600); err != nil {
		return "", "", "", fmt.Errorf("write temporary cloud-init user-data: %w", err)
	}
	return userDataPath, privateKey, filepath.Join(dir, "known_hosts"), nil
}

type sshRunner struct {
	evidence *evidence.Recorder
	run      func(context.Context, []string) (string, string, error)
}

func (r sshRunner) connect(ctx context.Context, user, ip, privateKey, knownHosts string) error {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{
		"-i", privateKey,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHosts,
		user + "@" + ip,
		"hostname",
	}
	stdout, stderr, runErr := r.run(timeout, args)
	checkErr := runErr
	if checkErr == nil && strings.TrimSpace(stdout) == "" {
		checkErr = errors.New("SSH login returned an empty hostname")
	}
	if err := r.evidence.Record("test-ssh-login", "ssh", args, nil, stdout, stderr, checkErr, false); err != nil {
		return errors.Join(errSSHEvidence, checkErr, err)
	}
	if checkErr != nil {
		return fmt.Errorf("SSH login attempt failed: %w (see evidence for output)", checkErr)
	}
	return nil
}

func runSSH(ctx context.Context, args []string) (string, string, error) {
	//nolint:gosec // Arguments are generated by this runner and passed directly without a shell.
	command := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	archiveID := flag.Int64("cloudimg-archive-id", 0, "cloud-init archive ID available in is1b")
	diskPlanID := flag.Int64("disk-plan-id", 0, "Disk plan ID available in is1b")
	diskSizeGB := flag.Int64("disk-size-gb", 20, "Cloud-init Disk size in GB")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating, booting, SSH-connecting to, and deleting a temporary Server and Disk in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *archiveID <= 0 || *diskPlanID <= 0 || *diskSizeGB <= 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/server --skr ./skr --cloudimg-archive-id ID --disk-plan-id ID [--disk-size-gb 20] --confirm-is1b-live")
		return 2
	}
	path, err := filepath.Abs(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fmt.Fprintln(os.Stderr, "skr must be an executable built binary")
		return 1
	}
	dir, err := os.MkdirTemp("", "skr-e2e-server-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temporary SSH material directory:", err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "remove temporary SSH material:", err)
			exitCode = 1
		}
	}()
	userDataPath, privateKey, knownHosts, err := createSSHMaterial(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	recorder, err := evidence.New("server-api")
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
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique E2E Server name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffix[:])
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	runner := cliRunner{binary: path, evidence: recorder, userDataPath: userDataPath}
	ssh := sshRunner{evidence: recorder, run: runSSH}
	test := scenario{
		client: runner, archiveID: *archiveID, diskPlanID: *diskPlanID, diskSizeGB: *diskSizeGB,
		userDataPath: userDataPath, privateKey: privateKey, knownHosts: knownHosts,
		connect: ssh.connect, pollInterval: 10 * time.Second,
	}
	if err := test.run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "Server E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Server E2E passed; cloud-init SSH login succeeded and the Server and Disk were deleted.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
