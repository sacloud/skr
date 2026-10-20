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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const serverDescription = "Temporary GSLB health-check server created by skr live E2E"

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

type diskItem struct {
	ID              types.ID `json:"ID"`
	Name            string   `json:"Name"`
	ServerID        types.ID `json:"ServerID"`
	DiskPlanID      types.ID `json:"DiskPlanID"`
	SourceArchiveID types.ID `json:"SourceArchiveID"`
	SizeMB          int64    `json:"SizeMB"`
}

func (server serverItem) sharedIP() (string, error) {
	if !server.InstanceStatus.IsUp() {
		return "", fmt.Errorf("server %s is not up", server.ID)
	}
	for _, iface := range server.Interfaces {
		if iface.UpstreamType == "shared" && net.ParseIP(iface.IPAddress) != nil {
			return iface.IPAddress, nil
		}
	}
	return "", fmt.Errorf("server %s has no shared IPv4 address", server.ID)
}

func (s scenario) findServer(ctx context.Context, step, name string) ([]serverItem, error) {
	query := "map({ID,Name,Description,InstanceStatus,Interfaces})"
	args := []string{"iaas-api", "server", "find", "--zone", zone, "--name", name, "--query", query}
	data, err := s.call(ctx, step, args, map[string]any{"Zone": zone, "Name": name, "Query": query})
	if err != nil {
		return nil, err
	}
	var items []serverItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Server list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) findDisk(ctx context.Context, step, name string) ([]diskItem, error) {
	query := "map({ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB})"
	args := []string{"iaas-api", "disk", "find", "--zone", zone, "--name", name, "--query", query}
	data, err := s.call(ctx, step, args, map[string]any{"Zone": zone, "Name": name, "Query": query})
	if err != nil {
		return nil, err
	}
	var items []diskItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Disk list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) prepareServer(ctx context.Context, gslbName string) error {
	if s.serverName == "" || s.archiveID <= 0 || s.diskPlanID <= 0 || s.diskSizeGB <= 0 {
		return errors.New("server E2E requires a name, cloud-init archive, disk plan, and positive disk size")
	}
	if s.lookupDNS == nil || s.connectSSH == nil || s.shutdownSSH == nil {
		return errors.New("DNS and SSH checks are required for server health-check E2E")
	}
	args := []string{"iaas-api", "archive", "read", "--zone", zone,
		"--id", fmt.Sprint(s.archiveID), "--query", "{ID,Tags,SizeMB}"}
	data, err := s.call(ctx, "archive-preflight-read", args, map[string]any{"Zone": zone, "ID": s.archiveID})
	if err != nil {
		return err
	}
	var archive struct {
		ID     types.ID
		Tags   []string
		SizeMB int64
	}
	if err := json.Unmarshal(data, &archive); err != nil {
		return fmt.Errorf("decode source Archive: %w", err)
	}
	cloudInit := false
	for _, tag := range archive.Tags {
		cloudInit = cloudInit || tag == "cloud-init"
	}
	if archive.ID != types.ID(s.archiveID) || !cloudInit || archive.SizeMB <= 0 ||
		archive.SizeMB > s.diskSizeGB*1024 {
		return errors.New("source Archive must have the cloud-init tag and fit the requested disk size")
	}
	servers, err := s.findServer(ctx, "server-before-find", s.serverName)
	if err != nil {
		return err
	}
	if len(servers) != 0 {
		return fmt.Errorf("server name %q already exists; refusing to modify it", s.serverName)
	}
	disks, err := s.findDisk(ctx, "disk-before-find", s.serverName)
	if err != nil {
		return err
	}
	if len(disks) != 0 {
		return fmt.Errorf("disk name %q already exists; refusing to modify it", s.serverName)
	}
	gslbs, err := s.find(ctx, "before-find", gslbName)
	if err != nil {
		return err
	}
	if len(gslbs) != 0 {
		return fmt.Errorf("GSLB name %q already exists; refusing to modify it", gslbName)
	}
	return nil
}

func (s scenario) createServer(ctx context.Context, name string) (serverItem, error) {
	marker := serverDescription + " (" + name + ")"
	disk := fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", s.archiveID, s.diskPlanID, s.diskSizeGB)
	args := []string{
		"iaas-api", "server", "create", "--zone", zone, "--name", s.serverName,
		"--description", marker, "--cpu", "1", "--memory-gb", "1",
		"--disk", disk, "--nic", "upstream=shared",
	}
	data, err := s.call(ctx, "server-create", args, map[string]any{
		"Zone": zone, "Name": s.serverName, "Description": marker, "CPU": 1, "MemoryGB": 1,
		"CloudimgArchiveID": s.archiveID, "DiskPlanID": s.diskPlanID, "DiskSizeGB": s.diskSizeGB,
		"NetworkInterfaces": []string{"upstream=shared"},
	})
	if err != nil {
		return serverItem{}, err
	}
	var server serverItem
	if err := json.Unmarshal(data, &server); err != nil {
		return serverItem{}, fmt.Errorf("decode created Server: %w", err)
	}
	if server.ID.IsEmpty() || server.Name != s.serverName || server.Description != marker {
		return serverItem{}, errors.New("created Server identity does not match the E2E request")
	}
	return server, nil
}

func (s scenario) readServer(ctx context.Context, step string, id types.ID) (serverItem, error) {
	query := "{ID,Name,Description,InstanceStatus,Interfaces}"
	args := []string{"iaas-api", "server", "read", "--zone", zone, "--id", id.String(), "--query", query}
	data, err := s.call(ctx, step, args, map[string]any{"Zone": zone, "ID": id, "Query": query})
	if err != nil {
		return serverItem{}, err
	}
	var server serverItem
	if err := json.Unmarshal(data, &server); err != nil {
		return serverItem{}, fmt.Errorf("%s: decode Server: %w", step, err)
	}
	if server.ID != id {
		return serverItem{}, fmt.Errorf("%s: expected Server %s, got %s", step, id, server.ID)
	}
	return server, nil
}

func (s scenario) bootServer(ctx context.Context, id types.ID, withUserData bool) error {
	args := []string{"iaas-api", "server", "boot", "--zone", zone, "--id", id.String()}
	request := map[string]any{"Zone": zone, "ID": id}
	if withUserData {
		if s.userDataPath == "" {
			return errors.New("cloud-init user-data path is missing")
		}
		args = append(args, "--user-data", "@"+s.userDataPath)
		request["UserDataPath"] = s.userDataPath
	}
	_, err := s.call(ctx, "server-boot", args, request)
	return err
}

func (s scenario) bootAndWaitForSSH(ctx context.Context, id types.ID) error {
	if err := s.bootServer(ctx, id, true); err != nil {
		return err
	}
	return s.waitForSSH(ctx, id)
}

func (s scenario) waitForSSH(ctx context.Context, id types.ID) error {
	timeout := s.serverTimeout
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	poll := s.pollInterval
	if poll <= 0 {
		poll = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var lastSSHError error
	for {
		server, err := s.readServer(ctx, "server-read-boot", id)
		if err != nil {
			return err
		}
		if server.InstanceStatus.IsUp() {
			ip, err := server.sharedIP()
			if err == nil {
				if err := s.connectSSH(ctx, "server-ssh-ready", ip, s.privateKey, s.knownHosts); err == nil {
					return nil
				} else {
					lastSSHError = err
				}
			} else {
				lastSSHError = err
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("server %s did not become SSH-ready (last check: %v): %w", id, lastSSHError, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s scenario) waitForServerDown(ctx context.Context, id types.ID) error {
	timeout := s.serverTimeout
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	poll := s.pollInterval
	if poll <= 0 {
		poll = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		server, err := s.readServer(ctx, "server-read-stopped", id)
		if err != nil {
			return err
		}
		if server.InstanceStatus.IsDown() {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("server %s did not stop: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s scenario) waitForDNS(ctx context.Context, step, fqdn, ip string) error {
	timeout := s.dnsTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	poll := s.pollInterval
	if poll <= 0 {
		poll = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		answers, err := s.lookupDNS(ctx, step, fqdn)
		if err == nil && len(answers) == 1 && answers[0] == ip {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("DNS answers for %s did not become exactly [%s]: got %v, last query error: %v: %w",
				fqdn, ip, answers, err, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s scenario) cleanupServer(name string, knownID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	id := knownID
	if id.IsEmpty() {
		items, err := s.findServer(ctx, "server-cleanup-find", s.serverName)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Server: %w", err)
		}
		if len(items) == 0 {
			return s.cleanupOrphanDisk(ctx)
		}
		if len(items) != 1 || items[0].ID.IsEmpty() || !s.ownsServer(items[0], name) {
			return errors.New("cannot safely identify created Server; manual cleanup required")
		}
		id = items[0].ID
	}
	current, err := s.readServer(ctx, "server-cleanup-read", id)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	if !s.ownsServer(current, name) {
		return errors.New("Server identity changed; cleanup refused")
	}
	args := []string{
		"iaas-api", "server", "delete", "--zone", zone, "--id", id.String(),
		"--force", "--fail-if-not-found",
	}
	_, deleteErr := s.call(ctx, "server-cleanup-delete", args, map[string]any{
		"Zone": zone, "ID": id, "Force": true, "FailIfNotFound": true,
	})
	remaining, err := s.findServer(ctx, "server-cleanup-find-after", s.serverName)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	for _, item := range remaining {
		if item.ID == id {
			return errors.Join(deleteErr, fmt.Errorf("Server %s remains after cleanup", id))
		}
	}
	return errors.Join(deleteErr, s.cleanupOrphanDisk(ctx))
}

func (s scenario) ownsServer(server serverItem, name string) bool {
	return server.Name == s.serverName && server.Description == serverDescription+" ("+name+")"
}

func (s *scenario) captureDisk(ctx context.Context, serverID types.ID) error {
	disks, err := s.findDisk(ctx, "disk-created-find", s.serverName)
	if err != nil {
		return err
	}
	if len(disks) != 1 || disks[0].ID.IsEmpty() || disks[0].ServerID != serverID ||
		!s.matchesDisk(disks[0]) {
		return errors.New("cannot safely identify created Disk; manual cleanup required")
	}
	s.diskID = disks[0].ID
	return nil
}

func (s scenario) matchesDisk(disk diskItem) bool {
	return disk.Name == s.serverName && disk.DiskPlanID == types.ID(s.diskPlanID) &&
		disk.SourceArchiveID == types.ID(s.archiveID) && disk.SizeMB == s.diskSizeGB*1024
}

func (s scenario) cleanupOrphanDisk(ctx context.Context) error {
	disks, err := s.findDisk(ctx, "disk-cleanup-find-after", s.serverName)
	if err != nil {
		return err
	}
	if len(disks) == 0 {
		if !s.diskID.IsEmpty() {
			return errors.New("created Disk not found by its recorded name; manual cleanup required")
		}
		return nil
	}
	if len(disks) != 1 || s.diskID.IsEmpty() || disks[0].ID != s.diskID ||
		!disks[0].ServerID.IsEmpty() || !s.matchesDisk(disks[0]) {
		return errors.New("cannot safely identify orphan Disk; manual cleanup required")
	}
	disk := disks[0]
	query := "{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB}"
	args := []string{"iaas-api", "disk", "read", "--zone", zone, "--id", disk.ID.String(), "--query", query}
	data, err := s.call(ctx, "disk-cleanup-read", args, map[string]any{"Zone": zone, "ID": disk.ID, "Query": query})
	if err != nil {
		return err
	}
	var current diskItem
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode orphan Disk: %w", err)
	}
	if current != disk {
		return errors.New("orphan Disk identity changed; cleanup refused")
	}
	args = []string{"iaas-api", "disk", "delete", "--zone", zone, "--id", disk.ID.String(), "--fail-if-not-found"}
	if _, err := s.call(ctx, "disk-cleanup-delete", args, map[string]any{"Zone": zone, "ID": disk.ID}); err != nil {
		return err
	}
	remaining, err := s.findDisk(ctx, "disk-cleanup-find-final", s.serverName)
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return errors.New("orphan Disk remains after cleanup")
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
		return "", "", "", fmt.Errorf("protect temporary SSH key: %w", err)
	}
	publicKey, err := os.ReadFile(privateKey + ".pub") //nolint:gosec // The public key is created inside the runner's private temporary directory.
	if err != nil {
		return "", "", "", fmt.Errorf("read temporary SSH public key: %w", err)
	}
	userDataPath = filepath.Join(dir, "user-data.yaml")
	userData := fmt.Sprintf(
		"#cloud-config\nusers:\n  - default\n  - name: skr-e2e\n    shell: /bin/bash\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    ssh_authorized_keys:\n      - %s\n",
		strings.TrimSpace(string(publicKey)),
	)
	//nolint:gosec // userDataPath is inside the private temporary directory created by this runner.
	if err := os.WriteFile(userDataPath, []byte(userData), 0o600); err != nil {
		return "", "", "", fmt.Errorf("write temporary cloud-init user-data: %w", err)
	}
	return userDataPath, privateKey, filepath.Join(dir, "known_hosts"), nil
}

type sshRunner struct {
	evidence *evidence.Recorder
}

func (r sshRunner) connect(ctx context.Context, step, ip, privateKey, knownHosts string) error {
	return r.run(ctx, step, ip, privateKey, knownHosts, "hostname")
}

func (r sshRunner) shutdown(ctx context.Context, step, ip, privateKey, knownHosts string) error {
	return r.run(ctx, step, ip, privateKey, knownHosts, "sudo -n systemctl poweroff --no-block")
}

func (r sshRunner) run(ctx context.Context, step, ip, privateKey, knownHosts, remoteCommand string) error {
	timeout, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{
		"-i", privateKey, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHosts,
		"skr-e2e@" + ip, remoteCommand,
	}
	command := exec.CommandContext(timeout, "ssh", args...) //nolint:gosec // Arguments target the temporary server with a fixed remote command, without a local shell.
	output, runErr := command.CombinedOutput()
	if err := r.evidence.Record(step, "ssh", args, nil, string(output), "", runErr, false); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return fmt.Errorf("%s: SSH command failed (see evidence): %w", step, runErr)
	}
	return nil
}

type dnsRunner struct {
	evidence *evidence.Recorder
}

func (r dnsRunner) lookup(ctx context.Context, step, fqdn string) ([]string, error) {
	args := []string{"+noall", "+comments", "+answer", "+time=3", "+tries=1", fqdn, "A"}
	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(timeout, "dig", args...) //nolint:gosec // The API-returned FQDN is passed as a structured argument without a shell.
	output, runErr := command.CombinedOutput()
	var answers []string
	if runErr == nil {
		answers, runErr = parseDNSAnswers(string(output))
	}
	if err := r.evidence.Record(step, "dig", args, map[string]string{"FQDN": fqdn}, string(output), "", runErr, false); err != nil {
		return nil, errors.Join(runErr, err)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: DNS query failed (see evidence): %w", step, runErr)
	}
	return answers, nil
}

func parseDNSAnswers(output string) ([]string, error) {
	var answers []string
	headerSeen := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, ";; ->>HEADER<<-") {
			if headerSeen {
				return nil, errors.New("DNS query returned multiple response headers")
			}
			headerSeen = true
			fields := strings.Fields(line)
			status := ""
			for i, field := range fields {
				if field == "status:" && i+1 < len(fields) {
					status = strings.TrimSuffix(fields[i+1], ",")
					break
				}
			}
			if status != "NOERROR" {
				return nil, fmt.Errorf("DNS query returned status %q, want NOERROR", status)
			}
		}
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[2] == "IN" && fields[3] == "A" {
			ip := net.ParseIP(fields[4])
			if ip == nil || ip.To4() == nil {
				return nil, errors.New("DNS query returned an invalid A record")
			}
			answers = append(answers, ip.String())
		}
	}
	if !headerSeen {
		return nil, errors.New("DNS query returned no response header")
	}
	return answers, nil
}
