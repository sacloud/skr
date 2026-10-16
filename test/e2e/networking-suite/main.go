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
	zone        = "is1b"
	namePrefix  = "skr-e2e-networking-suite-"
	description = "Temporary Networking Suite E2E resource"
	sshUser     = "ubuntu"
	tcpPort     = 38172
)

type cli interface {
	call(context.Context, string, []string, any) ([]byte, error)
}

type cliRunner struct {
	binary   string
	evidence *evidence.Recorder
}

func (r cliRunner) call(ctx context.Context, step string, args []string, request any) ([]byte, error) {
	if err := evidence.ValidateStep(step); err != nil {
		return nil, err
	}
	timeoutDuration := 90 * time.Second
	if strings.HasPrefix(step, "create-server-") || strings.HasPrefix(step, "cleanup-delete-server-") {
		timeoutDuration = 15 * time.Minute
	}
	timeout, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()
	command := exec.CommandContext(timeout, r.binary, args...) //nolint:gosec // The path is an explicitly selected local skr binary; structured arguments are not passed through a shell.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := r.evidence.Record(step, r.binary, args, request, stdout.String(), stderr.String(), runErr, false); err != nil {
		return nil, errors.Join(runErr, err)
	}
	if runErr != nil {
		return nil, fmt.Errorf("%s: skr failed: %w (see private evidence for stderr)", step, runErr)
	}
	return stdout.Bytes(), nil
}

type groupItem struct {
	SRN                  string
	Name                 string
	Description          string
	IPv4AddressRangeCIDR string
	Region               struct{ Code string }
}

type subnetItem struct {
	SRN                  string
	Name                 string
	Description          string
	IPv4AddressRangeCIDR string
	Zone                 struct{ Code string }
	SubnetGroup          struct{ SRN string }
}

type connectionItem struct {
	SRN                  string
	EphemeralIPv4Address string
}

type addressItem struct {
	SRN       string
	IPAddress string
	Subnet    struct{ SRN string }
}

type archiveItem struct {
	ID          types.ID
	Name        string
	Description string
}

type serverInterface struct {
	ID           types.ID
	IPAddress    string
	MACAddress   string
	UpstreamType string
}

type diskItem struct {
	ID              types.ID
	Name            string
	ServerID        types.ID
	DiskPlanID      types.ID
	SourceArchiveID types.ID
	SizeMB          int64
	Availability    string
}

type serverItem struct {
	ID             types.ID
	Name           string
	Description    string
	InstanceStatus types.EServerInstanceStatus
	Interfaces     []serverInterface
	Disks          []diskItem
}

type scenario struct {
	client     cli
	archiveID  int64
	diskPlanID int64
	diskSizeGB int64
	keyPath    string
	knownHosts string
	userDataA  string
	userDataB  string
	tempDir    string
	publicIP   string
	bAddress   string
	group      groupItem
	subnets    [2]subnetItem
	servers    [2]serverItem
	connects   [2]connectionItem
	booted     [2]bool
}

func (s *scenario) call(ctx context.Context, step string, args []string, request any, out any) error {
	data, err := s.client.call(ctx, step, args, request)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: decode CLI JSON: %w", step, err)
	}
	return nil
}

func (s *scenario) listGroups(ctx context.Context, step string) ([]groupItem, error) {
	var items []groupItem
	err := s.call(ctx, step, []string{
		"networking-suite-api", "subnet-group", "list", "--query",
		"map({SRN,Name,Description,IPv4AddressRangeCIDR,Region})",
	}, nil, &items)
	if err == nil && items == nil {
		err = fmt.Errorf("%s: expected JSON array", step)
	}
	return items, err
}

func (s *scenario) server(ctx context.Context, step, name string, id types.ID) (serverItem, error) {
	var item serverItem
	args := []string{"iaas-api", "server", "read", "--zone", zone, "--id", id.String(), "--query",
		"{ID,Name,Description,InstanceStatus,Interfaces,Disks}"}
	if err := s.call(ctx, step, args, map[string]any{"ID": id, "Name": name}, &item); err != nil {
		return serverItem{}, err
	}
	if item.ID != id || item.Name != name || item.Description != description {
		return serverItem{}, fmt.Errorf("%s: Server identity mismatch for ID %s", step, id)
	}
	return item, nil
}

func (s *scenario) disk(ctx context.Context, step string, server serverItem) (diskItem, error) {
	if len(server.Disks) != 1 || server.Disks[0].ID.IsEmpty() {
		return diskItem{}, fmt.Errorf("%s: expected exactly one Disk with an ID", step)
	}
	var item diskItem
	id := server.Disks[0].ID
	if err := s.call(ctx, step, []string{
		"iaas-api", "disk", "read", "--zone", zone, "--id", id.String(),
		"--query", "{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB,Availability}",
	}, map[string]any{"ID": id, "ServerID": server.ID}, &item); err != nil {
		return diskItem{}, err
	}
	return item, nil
}

func (s *scenario) waitForDisk(ctx context.Context, step string, server serverItem) (diskItem, error) {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		item, err := s.disk(ctx, step, server)
		if err != nil {
			return diskItem{}, err
		}
		if err := checkDisk(item, server.ID, server.Name, s.archiveID, s.diskPlanID, s.diskSizeGB); err != nil {
			return diskItem{}, err
		}
		if item.Availability == "available" {
			return item, nil
		}
		if time.Now().After(deadline) {
			return diskItem{}, fmt.Errorf("%s: Disk %s did not become available", step, item.ID)
		}
		select {
		case <-ctx.Done():
			return diskItem{}, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func findInterface(item serverItem, upstream string) (serverInterface, error) {
	var found *serverInterface
	expected := upstream
	if strings.EqualFold(expected, "disconnected") {
		expected = "none"
	}
	for i := range item.Interfaces {
		if strings.EqualFold(item.Interfaces[i].UpstreamType, expected) {
			if found != nil {
				return serverInterface{}, fmt.Errorf("Server %s has multiple %s interfaces", item.ID, upstream)
			}
			found = &item.Interfaces[i]
		}
	}
	if found == nil || found.ID.IsEmpty() {
		return serverInterface{}, fmt.Errorf("Server %s has no %s interface", item.ID, upstream)
	}
	return *found, nil
}

func interfaceSRN(id types.ID) string {
	return fmt.Sprintf("srnv1:sakura-%s:sakura.iaas.interface:%s", zone, id)
}

func (s *scenario) stopServers(ctx context.Context) error {
	for i := range s.servers {
		if s.servers[i].ID.IsEmpty() {
			continue
		}
		item, err := s.server(ctx, fmt.Sprintf("cleanup-check-server-%d", i), s.servers[i].Name, s.servers[i].ID)
		if err != nil {
			return err
		}
		s.booted[i] = s.booted[i] || item.InstanceStatus.IsUp()
		if i == 0 && s.publicIP == "" {
			for _, iface := range item.Interfaces {
				if strings.EqualFold(iface.UpstreamType, "shared") && net.ParseIP(iface.IPAddress) != nil {
					s.publicIP = iface.IPAddress
					break
				}
			}
		}
	}
	if !s.booted[0] && !s.booted[1] {
		return nil
	}
	var shutdownErr error
	if s.booted[1] && s.bAddress != "" && s.publicIP != "" {
		if _, _, err := s.ssh(ctx, "networking-suite-stop-peer", s.publicIP,
			pythonTCPCommand(s.bAddress, "shutdown")); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("request peer Server shutdown: %w", err))
		}
	}
	if s.booted[0] && s.publicIP != "" {
		_, _, err := s.ssh(ctx, "networking-suite-stop-entry", s.publicIP,
			"sudo sh -c 'sleep 2; systemctl poweroff' >/dev/null 2>&1 </dev/null &")
		if err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("request entry Server shutdown: %w", err))
		}
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		allStopped := true
		for i := range s.servers {
			if !s.booted[i] || s.servers[i].ID.IsEmpty() {
				continue
			}
			item, err := s.server(ctx, fmt.Sprintf("cleanup-read-server-%d", i), s.servers[i].Name, s.servers[i].ID)
			if err != nil {
				return err
			}
			if !item.InstanceStatus.IsDown() {
				allStopped = false
			}
		}
		if allStopped {
			s.booted = [2]bool{}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return errors.Join(shutdownErr, errors.New("timed out waiting for temporary Servers to stop"))
}

func (s *scenario) ssh(ctx context.Context, step, ip, remoteCommand string) (string, string, error) {
	timeout, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{
		"-i", s.keyPath, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + s.knownHosts, sshUser + "@" + ip, remoteCommand,
	}
	command := exec.CommandContext(timeout, "ssh", args...) //nolint:gosec // SSH arguments are assembled from validated addresses and fixed commands.
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if err := s.clientEvidence(step, "ssh", args, stdout.String(), stderr.String(), runErr); err != nil {
		return stdout.String(), stderr.String(), errors.Join(runErr, err)
	}
	if runErr != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("%s: SSH failed (see private evidence): %w", step, runErr)
	}
	return stdout.String(), stderr.String(), nil
}

func (s *scenario) clientEvidence(step, binary string, args []string, stdout, stderr string, runErr error) error {
	recorder, ok := s.client.(interface {
		record(string, string, []string, any, string, string, error) error
	})
	if !ok {
		return nil
	}
	return recorder.record(step, binary, args, nil, stdout, stderr, runErr)
}

func pythonTCPCommand(ip, command string) string {
	return pythonTCPCommandAt(ip, tcpPort, command)
}

func pythonTCPCommandAt(ip string, port int, command string) string {
	return "python3 -c " + shellQuote(pythonTCPCode(ip, port, command))
}

func pythonTCPCode(ip string, port int, command string) string {
	return fmt.Sprintf(
		"import socket; s=socket.create_connection((%q,%d),timeout=10); s.sendall(%q.encode()); r=s.makefile('rb'); print(r.readline().decode().strip()); s.close()",
		ip, port, command+"\n",
	)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *scenario) run(ctx context.Context, name string) (result error) {
	if _, err := s.client.call(ctx, "profile-current", []string{"config", "current"}, nil); err != nil {
		return fmt.Errorf("選択中の SDK profile を確認できません: %w", err)
	}
	endpoint := os.Getenv("SAKURA_ENDPOINTS_NETWORKING_SUITE")
	if !strings.Contains(endpoint, "/zone/is1b/") {
		return errors.New("SAKURA_ENDPOINTS_NETWORKING_SUITE must point to the is1b API endpoint")
	}
	var archive archiveItem
	if err := s.call(ctx, "preflight-read-cloudimg-archive", []string{
		"iaas-api", "archive", "read", "--zone", zone, "--id", fmt.Sprint(s.archiveID),
		"--query", "{ID,Name,Description}",
	}, nil, &archive); err != nil {
		return err
	}
	if err := checkCloudimgArchive(archive, s.archiveID); err != nil {
		return err
	}
	groups, err := s.listGroups(ctx, "preflight-list-groups")
	if err != nil {
		return err
	}
	for _, item := range groups {
		if item.Name == name {
			return fmt.Errorf("test subnet group %q already exists; refusing to modify it", name)
		}
	}
	regionGroups := 0
	for _, item := range groups {
		if item.Region.Code == "is1" {
			regionGroups++
		}
	}
	if regionGroups >= 3 {
		return errors.New("is1 already has the maximum of three subnet groups; refusing to create another")
	}
	for _, serverName := range []string{name + "-entry", name + "-peer"} {
		output, err := s.client.call(ctx, "preflight-find-"+strings.TrimPrefix(serverName, name+"-"),
			[]string{"iaas-api", "server", "find", "--zone", zone, "--name", serverName, "--query", "map({ID,Name,Description})"}, nil)
		if err != nil {
			return err
		}
		var items []serverItem
		if err := json.Unmarshal(output, &items); err != nil {
			return fmt.Errorf("decode preflight Server list: %w", err)
		}
		if items == nil {
			return errors.New("preflight Server search returned a non-array")
		}
		for _, item := range items {
			if item.Name == serverName {
				return fmt.Errorf("test Server %q already exists; refusing to modify it", serverName)
			}
		}
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if cleanupErr := s.cleanup(cleanupCtx, name); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	if err := s.call(ctx, "create-subnet-group", []string{
		"networking-suite-api", "subnet-group", "create", "--name", name, "--description", description,
		"--ipv4-address-range-cidr", "10.0.0.0/16", "--region", "is1",
	}, map[string]any{"Name": name, "Description": description, "IPv4AddressRangeCIDR": "10.0.0.0/16", "Region": "is1"}, &s.group); err != nil {
		return err
	}
	if err := checkGroup(s.group, name); err != nil {
		return err
	}
	updatedGroupName := name + "-updated"
	if err := s.call(ctx, "update-subnet-group", []string{
		"networking-suite-api", "subnet-group", "update", s.group.SRN,
		"--name", updatedGroupName, "--description", description,
	}, map[string]any{"SRN": s.group.SRN, "Name": updatedGroupName, "Description": description}, nil); err != nil {
		return err
	}
	s.group.Name = updatedGroupName
	var readGroup groupItem
	if err := s.call(ctx, "read-subnet-group", []string{
		"networking-suite-api", "subnet-group", "read", s.group.SRN,
		"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Region}",
	}, nil, &readGroup); err != nil {
		return err
	}
	if err := checkGroup(readGroup, updatedGroupName); err != nil {
		return err
	}

	for i, cidr := range []string{"10.0.1.0/24", "10.0.2.0/24"} {
		subnetName := fmt.Sprintf("%s-subnet-%d", name, i+1)
		if err := s.call(ctx, fmt.Sprintf("create-subnet-%d", i), []string{
			"networking-suite-api", "subnet", "create", "--name", subnetName, "--description", description,
			"--ipv4-address-range-cidr", cidr, "--zone", zone, "--subnet-group-srn", s.group.SRN,
		}, map[string]any{"Name": subnetName, "Description": description, "IPv4AddressRangeCIDR": cidr, "Zone": zone, "SubnetGroupSRN": s.group.SRN}, &s.subnets[i]); err != nil {
			return err
		}
		if err := checkSubnet(s.subnets[i], subnetName, cidr, s.group.SRN); err != nil {
			return err
		}
	}
	updatedSubnetName := name + "-subnet-1-updated"
	if err := s.call(ctx, "update-subnet", []string{
		"networking-suite-api", "subnet", "update", s.subnets[0].SRN,
		"--name", updatedSubnetName, "--description", description,
	}, map[string]any{"SRN": s.subnets[0].SRN, "Name": updatedSubnetName, "Description": description}, nil); err != nil {
		return err
	}
	s.subnets[0].Name = updatedSubnetName
	var subnets []subnetItem
	if err := s.call(ctx, "list-subnets", []string{
		"networking-suite-api", "subnet", "list", s.group.SRN,
		"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
	}, nil, &subnets); err != nil {
		return err
	}
	if !containsSubnet(subnets, s.subnets[0]) || !containsSubnet(subnets, s.subnets[1]) {
		return errors.New("subnet list did not return both temporary subnets")
	}
	var readSubnet subnetItem
	if err := s.call(ctx, "read-subnet", []string{
		"networking-suite-api", "subnet", "read", s.subnets[0].SRN,
		"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup}",
	}, nil, &readSubnet); err != nil {
		return err
	}
	if err := checkSubnet(readSubnet, updatedSubnetName, "10.0.1.0/24", s.group.SRN); err != nil {
		return err
	}

	for i, serverName := range []string{name + "-entry", name + "-peer"} {
		nics := serverNICArgs(i == 0)
		args := []string{
			"iaas-api", "server", "create", "--zone", zone, "--name", serverName, "--description", description,
			"--cpu", "1", "--memory-gb", "1",
			"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", s.archiveID, s.diskPlanID, s.diskSizeGB),
		}
		args = append(args, nics...)
		if err := s.call(ctx, fmt.Sprintf("create-server-%d", i), args,
			map[string]any{"Name": serverName, "Description": description, "CloudimgArchiveID": s.archiveID, "DiskPlanID": s.diskPlanID, "DiskSizeGB": s.diskSizeGB, "NICs": nics}, &s.servers[i]); err != nil {
			return err
		}
		if s.servers[i].ID.IsEmpty() || s.servers[i].Name != serverName || s.servers[i].Description != description {
			return fmt.Errorf("created Server %d does not match its temporary identity", i)
		}
		s.servers[i], err = s.server(ctx, fmt.Sprintf("read-server-%d", i), serverName, s.servers[i].ID)
		if err != nil {
			return err
		}
		if _, err := s.waitForDisk(ctx, fmt.Sprintf("wait-server-disk-%d", i), s.servers[i]); err != nil {
			return err
		}
		nic, err := findInterface(s.servers[i], "disconnected")
		if err != nil {
			return err
		}
		if err := s.call(ctx, fmt.Sprintf("create-interface-connection-%d", i), []string{
			"networking-suite-api", "interface-connection", "create",
			"--interface-srn", interfaceSRN(nic.ID), "--subnet-srn", s.subnets[i].SRN,
		}, map[string]any{"InterfaceSRN": interfaceSRN(nic.ID), "SubnetSRN": s.subnets[i].SRN}, &s.connects[i]); err != nil {
			return err
		}
		if s.connects[i].SRN == "" || net.ParseIP(s.connects[i].EphemeralIPv4Address) == nil {
			return fmt.Errorf("interface connection %d returned an invalid SRN or IPv4 address", i)
		}
	}
	s.bAddress = s.connects[1].EphemeralIPv4Address
	addresses, err := s.readAddresses(ctx, 0)
	if err != nil {
		return err
	}
	if !containsAddress(addresses, s.connects[0], s.subnets[0].SRN) {
		return errors.New("address list did not return the entry Server address")
	}
	addresses, err = s.readAddresses(ctx, 1)
	if err != nil {
		return err
	}
	if !containsAddress(addresses, s.connects[1], s.subnets[1].SRN) {
		return errors.New("address list did not return the peer Server address")
	}

	tempDir, err := os.MkdirTemp("", "skr-e2e-networking-suite-")
	if err != nil {
		return err
	}
	s.tempDir = tempDir
	s.keyPath = filepath.Join(tempDir, "ssh-key")
	s.knownHosts = filepath.Join(tempDir, "known-hosts")
	publicKey, err := createKey(s.keyPath)
	if err != nil {
		return err
	}
	s.userDataA = filepath.Join(tempDir, "entry-user-data.yaml")
	s.userDataB = filepath.Join(tempDir, "peer-user-data.yaml")
	entryNIC, err := findInterface(s.servers[0], "disconnected")
	if err != nil {
		return err
	}
	contentA, err := entryUserData(publicKey, entryNIC.MACAddress)
	if err != nil {
		return err
	}
	contentB := peerUserData
	if err := writePrivateFile(s.userDataA, contentA); err != nil {
		return err
	}
	if err := writePrivateFile(s.userDataB, contentB); err != nil {
		return err
	}
	for i, path := range []string{s.userDataA, s.userDataB} {
		content := contentA
		if i == 1 {
			content = contentB
		}
		request := evidence.RequestFileInput{Path: path, Content: content}
		if err := s.call(ctx, fmt.Sprintf("boot-server-%d", i), []string{
			"iaas-api", "server", "boot", "--zone", zone, "--id", s.servers[i].ID.String(), "--user-data", "@" + path,
		}, request, nil); err != nil {
			return err
		}
		s.booted[i] = true
	}
	s.publicIP, err = s.waitForEntrySSH(ctx)
	if err != nil {
		return err
	}
	var response string
	for attempt := 0; attempt < 30; attempt++ {
		stdout, _, err := s.ssh(ctx, fmt.Sprintf("verify-cross-subnet-tcp-%02d", attempt), s.publicIP, pythonTCPCommand(s.bAddress, "ping"))
		if err == nil {
			response = strings.TrimSpace(stdout)
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	if response != "PONG networking-suite" {
		return fmt.Errorf("peer returned unexpected TCP response %q", response)
	}
	return nil
}

const peerUserData = `#cloud-config
hostname: nstest6-peer
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
write_files:
  - path: /usr/local/bin/skr-networking-suite-listener.py
    permissions: "0755"
    content: |
      #!/usr/bin/python3
      import socket
      import subprocess

      with socket.socket() as listener:
          listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
          listener.bind(("0.0.0.0", 38172))
          listener.listen()
          while True:
              conn, _ = listener.accept()
              with conn:
                  command = conn.makefile("r").readline().strip()
                  if command == "ping":
                      conn.sendall(b"PONG networking-suite\n")
                  elif command == "shutdown":
                      conn.sendall(b"stopping\n")
                      subprocess.Popen(["/usr/bin/systemctl", "poweroff"])
                      break
  - path: /etc/systemd/system/skr-networking-suite-listener.service
    permissions: "0644"
    content: |
      [Unit]
      Description=Temporary Networking Suite E2E listener
      After=network-online.target
      [Service]
      ExecStart=/usr/bin/python3 /usr/local/bin/skr-networking-suite-listener.py
      Restart=always
      [Install]
      WantedBy=multi-user.target
runcmd:
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, skr-networking-suite-listener.service]
`

func entryUserData(publicKey, nwsNICMAC string) (string, error) {
	mac, err := net.ParseMAC(nwsNICMAC)
	if err != nil {
		return "", fmt.Errorf("invalid Networking Suite NIC MAC address %q: %w", nwsNICMAC, err)
	}
	return fmt.Sprintf(`#cloud-config
hostname: nstest6
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
packages:
  - openssh-server
users:
  - default
ssh_authorized_keys:
  - %s
write_files:
  - path: /etc/netplan/60-networking-suite.yaml
    permissions: "0600"
    content: |
      network:
        version: 2
        ethernets:
          networking-suite:
            match:
              macaddress: "%s"
            dhcp4: true
            dhcp4-overrides:
              route-metric: 200
runcmd:
  - [netplan, generate]
  - [netplan, apply]
`, publicKey, strings.ToLower(mac.String())), nil
}

func (s *scenario) waitForEntrySSH(ctx context.Context) (string, error) {
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		item, err := s.server(ctx, "poll-entry-server", s.servers[0].Name, s.servers[0].ID)
		if err != nil {
			return "", err
		}
		s.servers[0] = item
		if item.InstanceStatus.IsUp() {
			for _, nic := range item.Interfaces {
				if strings.EqualFold(nic.UpstreamType, "shared") && net.ParseIP(nic.IPAddress) != nil {
					s.publicIP = nic.IPAddress
					if _, _, err := s.ssh(ctx, "wait-entry-ssh", s.publicIP, "hostname"); err == nil {
						return s.publicIP, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return "", errors.New("timed out waiting for SSH access to entry Server")
}

func (s *scenario) readAddresses(ctx context.Context, index int) ([]addressItem, error) {
	var items []addressItem
	connection := s.connects[index]
	err := s.call(ctx, fmt.Sprintf("list-addresses-%d", index), []string{
		"networking-suite-api", "address", "list", s.subnets[index].SRN,
		"--interface-connection-srn", connection.SRN, "--query", "map({SRN,IPAddress,Subnet})",
	}, map[string]any{"SubnetSRN": s.subnets[index].SRN, "InterfaceConnectionSRN": connection.SRN}, &items)
	if err != nil {
		return nil, err
	}
	if items == nil || len(items) != 1 {
		return nil, fmt.Errorf("subnet %d returned %d assigned addresses; want exactly one", index, len(items))
	}
	var address addressItem
	if err := s.call(ctx, fmt.Sprintf("read-address-%d", index), []string{
		"networking-suite-api", "address", "read", items[0].SRN, "--query", "{SRN,IPAddress,Subnet}",
	}, map[string]any{"SRN": items[0].SRN}, &address); err != nil {
		return nil, err
	}
	if address.IPAddress != connection.EphemeralIPv4Address {
		return nil, fmt.Errorf("address %s differs from interface connection address", address.SRN)
	}
	return items, nil
}

func (s *scenario) cleanup(ctx context.Context, name string) (result error) {
	defer func() {
		if s.tempDir != "" {
			if err := os.RemoveAll(s.tempDir); err != nil {
				result = errors.Join(result, fmt.Errorf("remove temporary SSH material: %w", err))
			}
		}
	}()
	if s.group.SRN == "" {
		groups, err := s.listGroups(ctx, "cleanup-find-subnet-group")
		if err != nil {
			result = errors.Join(result, err)
		} else {
			for _, item := range groups {
				if (item.Name == name || item.Name == name+"-updated") &&
					item.Description == description && item.IPv4AddressRangeCIDR == "10.0.0.0/16" &&
					item.Region.Code == "is1" {
					if s.group.SRN != "" {
						return errors.Join(result, errors.New("multiple matching temporary subnet groups; manual cleanup required"))
					}
					s.group = item
				}
			}
		}
	}
	if s.group.SRN != "" {
		for i := range s.subnets {
			if s.subnets[i].SRN != "" {
				continue
			}
			var items []subnetItem
			if err := s.call(ctx, fmt.Sprintf("cleanup-find-subnet-%d", i), []string{
				"networking-suite-api", "subnet", "list", s.group.SRN,
				"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
			}, nil, &items); err != nil {
				result = errors.Join(result, err)
				continue
			}
			expectedNames := []string{fmt.Sprintf("%s-subnet-%d", name, i+1)}
			if i == 0 {
				expectedNames = append(expectedNames, name+"-subnet-1-updated")
			}
			for _, item := range items {
				nameMatches := false
				for _, expectedName := range expectedNames {
					nameMatches = nameMatches || item.Name == expectedName
				}
				expectedCIDR := []string{"10.0.1.0/24", "10.0.2.0/24"}[i]
				if nameMatches && item.Description == description && item.IPv4AddressRangeCIDR == expectedCIDR &&
					item.Zone.Code == zone && item.SubnetGroup.SRN == s.group.SRN {
					if s.subnets[i].SRN != "" {
						return errors.Join(result, errors.New("multiple matching temporary subnets; manual cleanup required"))
					}
					s.subnets[i] = item
				}
			}
		}
	}
	for i := range s.servers {
		if !s.servers[i].ID.IsEmpty() {
			continue
		}
		serverName := name + "-entry"
		if i == 1 {
			serverName = name + "-peer"
		}
		item, err := s.findTemporaryServer(ctx, i, serverName)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if !item.ID.IsEmpty() {
			s.servers[i] = item
		}
	}
	stopErr := s.stopServers(ctx)
	if stopErr != nil {
		result = errors.Join(result, fmt.Errorf("graceful Server shutdown failed; force-deleting only verified temporary Servers: %w", stopErr))
	} else {
		for i := range s.connects {
			connection := s.connects[i]
			if connection.SRN == "" {
				continue
			}
			if _, err := s.client.call(ctx, fmt.Sprintf("cleanup-delete-connection-%d", i), []string{
				"networking-suite-api", "interface-connection", "delete", connection.SRN,
			}, map[string]any{"SRN": connection.SRN}); err != nil {
				result = errors.Join(result, err)
			}
		}
	}
	for i := range s.servers {
		item := s.servers[i]
		if item.ID.IsEmpty() {
			continue
		}
		current, err := s.server(ctx, fmt.Sprintf("cleanup-read-server-%d", i), item.Name, item.ID)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if current.Name != name+"-entry" && current.Name != name+"-peer" {
			result = errors.Join(result, errors.New("refusing to delete Server with unexpected identity"))
			continue
		}
		if _, err := s.waitForDisk(ctx, fmt.Sprintf("cleanup-wait-server-disk-%d", i), current); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if !sameInterfaceIDs(item.Interfaces, current.Interfaces) {
			result = errors.Join(result, fmt.Errorf("refusing to delete Server %s after its NICs changed", item.ID))
			continue
		}
		if _, err := s.client.call(ctx, fmt.Sprintf("cleanup-delete-server-%d", i), []string{
			"iaas-api", "server", "delete", "--zone", zone, "--id", item.ID.String(),
			"--with-disks", "--force", "--fail-if-not-found",
		}, map[string]any{"ID": item.ID, "Name": item.Name, "Description": item.Description}); err != nil {
			result = errors.Join(result, err)
			continue
		}
		var remaining []serverItem
		if err := s.call(ctx, fmt.Sprintf("cleanup-find-server-after-%d", i), []string{
			"iaas-api", "server", "find", "--zone", zone, "--name", item.Name, "--query", "map({ID,Name,Description})",
		}, nil, &remaining); err != nil {
			result = errors.Join(result, err)
		} else {
			for _, found := range remaining {
				if found.ID == item.ID {
					result = errors.Join(result, fmt.Errorf("temporary Server %s remains after deletion", item.ID))
				}
			}
		}
		var remainingDisks []diskItem
		if err := s.call(ctx, fmt.Sprintf("cleanup-find-disk-after-%d", i), []string{
			"iaas-api", "disk", "find", "--zone", zone, "--name", item.Name, "--query", "map({ID,Name,ServerID})",
		}, nil, &remainingDisks); err != nil {
			result = errors.Join(result, err)
		} else {
			for _, found := range remainingDisks {
				if found.Name == item.Name {
					result = errors.Join(result, fmt.Errorf("temporary Disk %s remains after deletion", found.ID))
				}
			}
		}
	}
	for i := len(s.subnets) - 1; i >= 0; i-- {
		item := s.subnets[i]
		if item.SRN == "" {
			continue
		}
		var addresses []addressItem
		if err := s.call(ctx, fmt.Sprintf("cleanup-list-addresses-%d", i), []string{
			"networking-suite-api", "address", "list", item.SRN, "--query", "map({SRN,IPAddress,Subnet})",
		}, nil, &addresses); err != nil {
			result = errors.Join(result, fmt.Errorf("list addresses for subnet %s; refusing cleanup: %w", item.SRN, err))
			continue
		}
		if len(addresses) != 0 {
			result = errors.Join(result, fmt.Errorf("subnet %s still has assigned addresses; refusing cleanup", item.SRN))
			continue
		}
		var current subnetItem
		if err := s.call(ctx, fmt.Sprintf("cleanup-read-subnet-%d", i), []string{
			"networking-suite-api", "subnet", "read", item.SRN,
			"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup}",
		}, nil, &current); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := checkSubnet(current, item.Name, item.IPv4AddressRangeCIDR, s.group.SRN); err != nil {
			result = errors.Join(result, fmt.Errorf("refusing to delete subnet: %w", err))
			continue
		}
		if _, err := s.client.call(ctx, fmt.Sprintf("cleanup-delete-subnet-%d", i), []string{
			"networking-suite-api", "subnet", "delete", item.SRN,
		}, map[string]any{"SRN": item.SRN}); err != nil {
			result = errors.Join(result, err)
		}
	}
	subnetsEmpty := false
	if s.group.SRN != "" {
		var remaining []subnetItem
		if err := s.call(ctx, "cleanup-list-subnets-after", []string{
			"networking-suite-api", "subnet", "list", s.group.SRN,
			"--query", "map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})",
		}, nil, &remaining); err != nil {
			result = errors.Join(result, err)
		} else if len(remaining) != 0 {
			result = errors.Join(result, errors.New("temporary subnet group still has subnets"))
		} else {
			subnetsEmpty = true
		}
	}
	if s.group.SRN != "" && subnetsEmpty {
		var current groupItem
		if err := s.call(ctx, "cleanup-read-subnet-group", []string{
			"networking-suite-api", "subnet-group", "read", s.group.SRN,
			"--query", "{SRN,Name,Description,IPv4AddressRangeCIDR,Region}",
		}, nil, &current); err != nil {
			result = errors.Join(result, err)
		} else if err := checkGroup(current, s.group.Name); err != nil {
			result = errors.Join(result, fmt.Errorf("refusing to delete subnet group: %w", err))
		} else if _, err := s.client.call(ctx, "cleanup-delete-subnet-group", []string{
			"networking-suite-api", "subnet-group", "delete", s.group.SRN,
		}, map[string]any{"SRN": s.group.SRN}); err != nil {
			result = errors.Join(result, err)
		} else {
			groups, err := s.listGroups(ctx, "cleanup-list-groups-after")
			if err != nil {
				result = errors.Join(result, err)
			} else {
				for _, found := range groups {
					if found.SRN == s.group.SRN {
						result = errors.Join(result, fmt.Errorf("temporary subnet group %s remains after deletion", s.group.SRN))
					}
				}
			}
		}
	}
	return result
}

func (s *scenario) findTemporaryServer(ctx context.Context, index int, name string) (serverItem, error) {
	var items []serverItem
	if err := s.call(ctx, fmt.Sprintf("cleanup-find-server-%d", index), []string{
		"iaas-api", "server", "find", "--zone", zone, "--name", name,
		"--query", "map({ID,Name,Description,Interfaces})",
	}, nil, &items); err != nil {
		return serverItem{}, err
	}
	var found serverItem
	for _, item := range items {
		if item.Name == name && item.Description == description && !item.ID.IsEmpty() {
			if !found.ID.IsEmpty() {
				return serverItem{}, errors.New("multiple matching temporary Servers; manual cleanup required")
			}
			found = item
		}
	}
	return found, nil
}

func checkGroup(item groupItem, name string) error {
	if item.SRN == "" || item.Name != name || item.Description != description ||
		item.IPv4AddressRangeCIDR != "10.0.0.0/16" || item.Region.Code != "is1" {
		return fmt.Errorf("subnet group identity mismatch for %q", name)
	}
	return nil
}

func checkSubnet(item subnetItem, name, cidr, groupSRN string) error {
	if item.SRN == "" || item.Name != name || item.Description != description ||
		item.IPv4AddressRangeCIDR != cidr || item.Zone.Code != zone || item.SubnetGroup.SRN != groupSRN {
		return fmt.Errorf("subnet identity mismatch for %q", name)
	}
	return nil
}

func checkDisk(disk diskItem, serverID types.ID, serverName string, archiveID, diskPlanID, diskSizeGB int64) error {
	if disk.ID.IsEmpty() || disk.Name != serverName || disk.ServerID != serverID ||
		disk.DiskPlanID != types.ID(diskPlanID) || disk.SourceArchiveID != types.ID(archiveID) ||
		diskSizeGB <= 0 || disk.SizeMB != diskSizeGB*1024 {
		return fmt.Errorf("Server %s Disk does not match the E2E request", serverID)
	}
	return nil
}

func sameInterfaceIDs(expected, actual []serverInterface) bool {
	if len(expected) != len(actual) {
		return false
	}
	ids := make(map[types.ID]int, len(expected))
	for _, iface := range expected {
		ids[iface.ID]++
	}
	for _, iface := range actual {
		if ids[iface.ID] == 0 {
			return false
		}
		ids[iface.ID]--
	}
	return true
}

func checkCloudimgArchive(item archiveItem, expectedID int64) error {
	if int64(item.ID) != expectedID {
		return fmt.Errorf("Archive read returned ID %s, want %d", item.ID, expectedID)
	}
	if !strings.Contains(strings.ToLower(item.Name+" "+item.Description), "cloudimg") {
		return fmt.Errorf("Archive %s (%q) is not marked cloudimg; select a cloud-init cloudimg Archive", item.ID, item.Name)
	}
	return nil
}

func serverNICArgs(entry bool) []string {
	if entry {
		return []string{"--nic", "upstream=shared", "--nic", "upstream=disconnected"}
	}
	return []string{"--nic", "upstream=disconnected"}
}

func containsSubnet(items []subnetItem, expected subnetItem) bool {
	for _, item := range items {
		if item.SRN == expected.SRN && item.Name == expected.Name && item.Description == description &&
			item.IPv4AddressRangeCIDR == expected.IPv4AddressRangeCIDR && item.SubnetGroup.SRN == expected.SubnetGroup.SRN {
			return true
		}
	}
	return false
}

func containsAddress(items []addressItem, connection connectionItem, subnetSRN string) bool {
	for _, item := range items {
		if item.IPAddress == connection.EphemeralIPv4Address && item.Subnet.SRN == subnetSRN {
			return true
		}
	}
	return false
}

func createKey(path string) (string, error) {
	command := exec.Command("ssh-keygen", "-t", "rsa", "-b", "3072", "-N", "", "-f", path) //nolint:gosec // Generates an ephemeral local test key without a shell.
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("generate temporary SSH key: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path + ".pub") //nolint:gosec // The public key is created inside the private temporary directory.
	if err != nil {
		return "", fmt.Errorf("read temporary public SSH key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func writePrivateFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write temporary user-data: %w", err)
	}
	return nil
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	archiveID := flag.Int64("cloudimg-archive-id", 0, "Cloud-init Archive ID available in is1b")
	diskPlanID := flag.Int64("disk-plan-id", 0, "Disk plan ID available in is1b")
	diskSizeGB := flag.Int64("disk-size-gb", 20, "Temporary Disk size in GB")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating and deleting a subnet group, two subnets, and two temporary Servers in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *archiveID <= 0 || *diskPlanID <= 0 || *diskSizeGB <= 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/networking-suite --skr ./skr --cloudimg-archive-id ID --disk-plan-id ID [--disk-size-gb 20] --confirm-is1b-live")
		return 2
	}
	endpoint := os.Getenv("SAKURA_ENDPOINTS_NETWORKING_SUITE")
	if !strings.Contains(endpoint, "/zone/is1b/") {
		fmt.Fprintln(os.Stderr, "SAKURA_ENDPOINTS_NETWORKING_SUITE must point to the is1b API endpoint")
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
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique E2E resource name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffix[:])
	recorder, err := evidence.New("networking-suite-api")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	runner := cliRunner{binary: path, evidence: recorder}
	scenario := &scenario{client: runner, archiveID: *archiveID, diskPlanID: *diskPlanID, diskSizeGB: *diskSizeGB}
	if err := scenario.run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "Networking Suite E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Networking Suite E2E passed; cross-subnet TCP response was verified and temporary resources were removed.")
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func (r cliRunner) record(step, binary string, args []string, request any, stdout, stderr string, runErr error) error {
	return r.evidence.Record(step, binary, args, request, stdout, stderr, runErr, false)
}

func main() {
	os.Exit(runMain())
}
