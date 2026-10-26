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
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/test/e2e/internal/evidence"
)

const (
	zone            = "is1b"
	namePrefix      = "skr-e2e-internet-"
	internetDesc    = "Temporary Internet for skr router-switch SSH E2E"
	serverDesc      = "Temporary Server for skr router-switch SSH E2E"
	sshUser         = "skr-e2e"
	commandTimeout  = 90 * time.Second
	createTimeout   = 35 * time.Minute
	cleanupTimeout  = 5 * time.Minute
	sshWaitTimeout  = 20 * time.Minute
	scenarioTimeout = 2*createTimeout + sshWaitTimeout + 10*time.Minute
	defaultDiskSize = 20
)

var errSSHEvidence = errors.New("record SSH evidence")

type internetItem struct {
	ID             types.ID `json:"ID"`
	Name           string   `json:"Name"`
	Description    string   `json:"Description"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
	BandWidthMbps  int      `json:"BandWidthMbps"`
	Switch         struct {
		ID types.ID `json:"ID"`
	} `json:"Switch"`
}

type subnetItem struct {
	ID             types.ID `json:"ID"`
	InternetID     types.ID `json:"InternetID"`
	DefaultRoute   string   `json:"DefaultRoute"`
	NextHop        string   `json:"NextHop"`
	StaticRoute    string   `json:"StaticRoute"`
	NetworkAddress string   `json:"NetworkAddress"`
	NetworkMaskLen int      `json:"NetworkMaskLen"`
	IPAddresses    []struct {
		IPAddress string `json:"IPAddress"`
	} `json:"IPAddresses"`
}

type serverInterface struct {
	ID           types.ID `json:"ID"`
	MACAddress   string   `json:"MACAddress"`
	IPAddress    string   `json:"IPAddress"`
	SwitchID     types.ID `json:"SwitchID"`
	UpstreamType string   `json:"UpstreamType"`
}

type serverItem struct {
	ID             types.ID                    `json:"ID"`
	Name           string                      `json:"Name"`
	Description    string                      `json:"Description"`
	InstanceStatus types.EServerInstanceStatus `json:"InstanceStatus"`
	Interfaces     []serverInterface           `json:"Interfaces"`
	Disks          []struct {
		ID types.ID `json:"ID"`
	} `json:"Disks"`
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
	args, err := internetArgs(step, request, r.userDataPath)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(ctx, timeoutForStep(step))
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

func timeoutForStep(step string) time.Duration {
	switch step {
	case "test-create-internet", "test-create-server":
		return createTimeout
	default:
		return commandTimeout
	}
}

func internetArgs(step string, request map[string]any, userDataPath string) ([]string, error) {
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
	case "preflight-find-internet", "cleanup-find-internet", "cleanup-find-internet-after":
		return findArgs("internet", request)
	case "test-create-internet":
		name, err := requestString(request, "Name")
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "internet", "create", "--zone", zone,
			"--name", name, "--description", internetDesc,
			"--network-mask-len", "28", "--band-width-mbps", "100",
		}, nil
	case "test-read-internet", "cleanup-read-internet":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "internet", "read", "--zone", zone, "--id", id,
			"--query", "{ID,Name,Description,NetworkMaskLen,BandWidthMbps,Switch}",
		}, nil
	case "test-update-internet", "test-update-internet-restore":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		name, err := requestString(request, "Name")
		if err != nil {
			return nil, err
		}
		desc, err := requestString(request, "Description")
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "internet", "update",
			"--zone", zone, "--id", id, "--name", name, "--description", desc,
		}, nil
	case "test-monitor-router":
		return monitorRouterArgs(request)
	case "test-add-subnet":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		maskLen, err := requestInt64(request, "NetworkMaskLen")
		if err != nil {
			return nil, err
		}
		nextHop, err := requestNextHop(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "internet", "add-subnet", "--zone", zone, "--id", id,
			"--network-mask-len", strconv.FormatInt(maskLen, 10), "--next-hop", nextHop,
		}, nil
	case "test-update-subnet", "test-delete-subnet":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		subnetID, err := requestInt64(request, "SubnetID")
		if err != nil {
			return nil, err
		}
		operation := "update-subnet"
		if step == "test-delete-subnet" {
			operation = "delete-subnet"
		}
		args := []string{
			"iaas-api", "internet", operation, "--zone", zone, "--id", id,
			"--subnet-id", strconv.FormatInt(subnetID, 10),
		}
		if step == "test-update-subnet" {
			nextHop, err := requestNextHop(request)
			if err != nil {
				return nil, err
			}
			args = append(args, "--next-hop", nextHop)
		}
		return args, nil
	case "test-enable-ipv6", "test-read-ipv6", "test-disable-ipv6":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		operation := strings.TrimPrefix(step, "test-")
		return []string{"iaas-api", "internet", operation, "--zone", zone, "--id", id}, nil
	case "test-list-subnet":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "internet", "list-subnet", "--zone", zone, "--id", id,
			"--query", "map({ID,InternetID,DefaultRoute,NextHop,StaticRoute,NetworkAddress,NetworkMaskLen,IPAddresses})",
		}, nil
	case "test-create-server":
		return serverCreateArgs(request)
	case "test-boot-server":
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
	case "test-read-server", "cleanup-read-server":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "server", "read", "--zone", zone, "--id", id,
			"--query", "{ID,Name,Description,InstanceStatus,Interfaces,Disks}",
		}, nil
	case "cleanup-delete-server":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		return []string{
			"iaas-api", "server", "delete", "--zone", zone, "--id", id,
			"--with-disks", "--force", "--fail-if-not-found",
		}, nil
	case "preflight-find-server", "cleanup-find-server", "cleanup-find-server-after":
		return findArgs("server", request)
	case "preflight-find-disk", "cleanup-find-disk-after":
		return findArgs("disk", request)
	case "cleanup-delete-internet":
		id, err := requestID(request)
		if err != nil {
			return nil, err
		}
		idValue, err := strconv.ParseInt(id, 10, 64)
		if err != nil || idValue <= 0 {
			return nil, errors.New("invalid ID")
		}
		return []string{"iaas-api", "internet", "delete", "--zone", zone, "--id", strconv.FormatInt(idValue, 10),
			"--force", "--fail-if-not-found"}, nil
	default:
		return nil, fmt.Errorf("unknown Internet E2E step %q", step)
	}
}

func serverCreateArgs(request map[string]any) ([]string, error) {
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
	switchID, err := requestInt64(request, "SwitchID")
	if err != nil {
		return nil, err
	}
	return []string{
		"iaas-api", "server", "create", "--zone", zone, "--name", name,
		"--description", serverDesc, "--cpu", "1", "--memory-gb", "1",
		"--disk", fmt.Sprintf("archive-id=%d,plan-id=%d,size-gb=%d", archiveID, diskPlanID, diskSizeGB),
		"--nic", "upstream=" + strconv.FormatInt(switchID, 10),
	}, nil
}

func findArgs(resource string, request map[string]any) ([]string, error) {
	name, err := requestString(request, "Name")
	if err != nil {
		return nil, err
	}
	return []string{"iaas-api", resource, "find", "--zone", zone, "--name", name}, nil
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

func requestNextHop(request map[string]any) (string, error) {
	value, err := requestString(request, "NextHop")
	if err != nil {
		return "", err
	}
	address := net.ParseIP(value)
	if address == nil || address.To4() == nil || address.To4().String() != value {
		return "", errors.New("NextHop must be an IPv4 address")
	}
	return value, nil
}

type sshRunner struct {
	evidence *evidence.Recorder
	run      func(context.Context, []string) (string, string, error)
}

func (r sshRunner) connect(ctx context.Context, ip, privateKey, knownHosts string) error {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{
		"-i", privateKey, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHosts,
		sshUser + "@" + ip, "hostname",
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

type scenario struct {
	client          cli
	archiveID       int64
	diskPlanID      int64
	diskSizeGB      int64
	userDataPath    string
	publicKey       string
	privateKey      string
	knownHosts      string
	connect         func(context.Context, string, string, string) error
	pollInterval    time.Duration
	sshTimeout      time.Duration
	recoveryTimeout time.Duration
}

func (s scenario) call(ctx context.Context, step string, request map[string]any) ([]byte, error) {
	return s.client.call(ctx, step, request)
}

func (s scenario) findInternet(ctx context.Context, step, name string) ([]internetItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "Name": name})
	if err != nil {
		return nil, err
	}
	var items []internetItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, fmt.Errorf("%s: decode Internet list: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON array", step)
	}
	return items, nil
}

func (s scenario) find(ctx context.Context, step, resource, name string) ([]serverItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "Name": name})
	if err != nil {
		return nil, err
	}
	if resource == "disk" {
		var items []struct {
			ID   types.ID `json:"ID"`
			Name string   `json:"Name"`
		}
		if err := json.Unmarshal(output, &items); err != nil {
			return nil, fmt.Errorf("%s: decode Disk list: %w", step, err)
		}
		if items == nil {
			return nil, fmt.Errorf("%s: expected a JSON array", step)
		}
		result := make([]serverItem, 0, len(items))
		for _, item := range items {
			result = append(result, serverItem{ID: item.ID, Name: item.Name})
		}
		return result, nil
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

func (s scenario) readInternet(ctx context.Context, step string, id types.ID) (internetItem, error) {
	output, err := s.call(ctx, step, map[string]any{"Zone": zone, "ID": id})
	if err != nil {
		return internetItem{}, err
	}
	var item internetItem
	if err := json.Unmarshal(output, &item); err != nil {
		return internetItem{}, fmt.Errorf("%s: decode Internet: %w", step, err)
	}
	if item.ID.IsEmpty() {
		return internetItem{}, fmt.Errorf("%s: Internet ID is empty", step)
	}
	return item, nil
}

func (s scenario) readServer(ctx context.Context, step string, id types.ID) (serverItem, error) {
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

func internetIdentity(item internetItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != internetDesc ||
		item.Switch.ID != switchID || item.NetworkMaskLen != 28 || item.BandWidthMbps != 100 {
		return fmt.Errorf("Internet identity mismatch for ID %s (expected name %q and E2E settings)", id, name)
	}
	return nil
}

func serverIdentity(item serverItem, id types.ID, name string, switchID types.ID) error {
	if item.ID != id || item.Name != name || item.Description != serverDesc {
		return fmt.Errorf("Server identity mismatch for ID %s (expected name %q and E2E description)", id, name)
	}
	if len(item.Interfaces) != 1 || item.Interfaces[0].SwitchID != switchID {
		return fmt.Errorf("Server NIC is not connected to the expected Router + Switch")
	}
	return nil
}

func (s scenario) network(ctx context.Context, internetID, switchID types.ID, server serverItem) (serverInterface, int, string, error) {
	output, err := s.call(ctx, "test-list-subnet", map[string]any{"Zone": zone, "ID": internetID})
	if err != nil {
		return serverInterface{}, 0, "", err
	}
	var subnets []subnetItem
	if err := json.Unmarshal(output, &subnets); err != nil {
		return serverInterface{}, 0, "", fmt.Errorf("decode Internet subnet list: %w", err)
	}
	if len(subnets) == 0 {
		return serverInterface{}, 0, "", errors.New("Internet has no IPv4 subnet")
	}
	var subnet *subnetItem
	for i := range subnets {
		if subnets[i].InternetID == internetID && subnets[i].StaticRoute == "" && subnets[i].NextHop == "" {
			if subnet != nil {
				return serverInterface{}, 0, "", errors.New("Internet has multiple primary IPv4 subnets")
			}
			subnet = &subnets[i]
		}
	}
	if subnet == nil {
		return serverInterface{}, 0, "", errors.New("Internet primary IPv4 subnet is missing")
	}
	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", subnet.NetworkAddress, subnet.NetworkMaskLen))
	if err != nil || subnet.NetworkMaskLen < 1 || subnet.NetworkMaskLen > 32 || prefix.Addr().Is6() {
		return serverInterface{}, 0, "", errors.New("Internet primary IPv4 subnet has invalid network settings")
	}
	gateway, err := netip.ParseAddr(subnet.DefaultRoute)
	if err != nil || !gateway.Is4() || !prefix.Contains(gateway) {
		return serverInterface{}, 0, "", errors.New("Internet primary IPv4 subnet has an invalid default route")
	}
	if len(server.Interfaces) == 0 {
		return serverInterface{}, 0, "", errors.New("Server has no NIC")
	}
	iface := server.Interfaces[0]
	if iface.SwitchID != switchID {
		return serverInterface{}, 0, "", errors.New("Server NIC is connected to a different Switch")
	}
	iface.IPAddress = ""
	for _, candidate := range subnet.IPAddresses {
		address, err := netip.ParseAddr(candidate.IPAddress)
		if err != nil || !address.Is4() || !prefix.Contains(address) ||
			address == gateway || address == prefix.Masked().Addr() || !prefix.Contains(address.Next()) {
			continue
		}
		iface.IPAddress = address.String()
		break
	}
	if iface.IPAddress == "" {
		return serverInterface{}, 0, "", errors.New("Internet subnet has no usable Server IPv4 address")
	}
	if _, err := net.ParseMAC(iface.MACAddress); err != nil {
		return serverInterface{}, 0, "", errors.New("Server NIC MAC address is invalid")
	}
	return iface, subnet.NetworkMaskLen, subnet.DefaultRoute, nil
}

func writeUserData(path, publicKey, mac, address string, prefix int, gateway string) error {
	if !strings.HasPrefix(publicKey, "ssh-") || strings.ContainsAny(publicKey, "\r\n") {
		return errors.New("invalid temporary SSH public key")
	}
	if _, err := net.ParseMAC(mac); err != nil {
		return errors.New("invalid Server NIC MAC address")
	}
	ip := net.ParseIP(address)
	route := net.ParseIP(gateway)
	if ip == nil || ip.To4() == nil || route == nil || route.To4() == nil || prefix < 1 || prefix > 32 {
		return errors.New("invalid Server NIC IPv4 network settings")
	}
	script := fmt.Sprintf(`set -eu
expected_mac=%s
address=%s
prefix=%d
gateway=%s
iface=
for path in /sys/class/net/*/address; do
  if [ "$(cat "$path")" = "$expected_mac" ]; then
    iface=${path%%/address}
    iface=${iface##*/}
    break
  fi
done
[ -n "$iface" ]
ip link set "$iface" up
ip address replace "$address/$prefix" dev "$iface"
ip route replace default via "$gateway" dev "$iface"
`, strconv.Quote(strings.ToLower(mac)), strconv.Quote(address), prefix, strconv.Quote(gateway))
	userData := fmt.Sprintf("#cloud-config\nusers:\n  - default\n  - name: %s\n    shell: /bin/bash\n    ssh_authorized_keys:\n      - %s\nruncmd:\n  - [sh, -c, %s]\n", sshUser, publicKey, strconv.Quote(script))
	if err := os.WriteFile(path, []byte(userData), 0o600); err != nil {
		return fmt.Errorf("write temporary cloud-init user-data: %w", err)
	}
	return nil
}

func (s scenario) waitForSSH(ctx context.Context, id types.ID, name string, switchID types.ID, address string) error {
	if s.connect == nil {
		return errors.New("SSH connection check is not configured")
	}
	timeout := s.sshTimeout
	if timeout <= 0 {
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
		item, err := s.readServer(ctx, "test-read-server", id)
		if err != nil {
			return err
		}
		if err := serverIdentity(item, id, name, switchID); err != nil {
			return err
		}
		if item.InstanceStatus.IsUp() {
			err := s.connect(ctx, address, s.privateKey, s.knownHosts)
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

func (s scenario) cleanup(name string, knownInternetID, knownSwitchID, knownServerID types.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	serverID := knownServerID
	if serverID.IsEmpty() {
		matches, err := s.find(ctx, "cleanup-find-server", "server", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Server: %w", err)
		}
		if len(matches) == 0 {
			disks, err := s.find(ctx, "cleanup-find-disk-after", "disk", name)
			if err != nil {
				return err
			}
			if len(disks) != 0 {
				return errors.New("cannot safely identify orphan Disk without its Server; manual cleanup required")
			}
			return s.cleanupInternet(ctx, name, knownInternetID, knownSwitchID)
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != serverDesc {
			return errors.New("cannot safely identify created Server; manual cleanup required")
		}
		serverID = matches[0].ID
	}
	current, err := s.readServer(ctx, "cleanup-read-server", serverID)
	if err != nil {
		return fmt.Errorf("cannot verify Server before cleanup: %w", err)
	}
	switchID := knownSwitchID
	if switchID.IsEmpty() && len(current.Interfaces) > 0 {
		switchID = current.Interfaces[0].SwitchID
	}
	if current.ID != serverID || current.Name != name || current.Description != serverDesc ||
		len(current.Interfaces) == 0 || current.Interfaces[0].SwitchID != switchID {
		return errors.New("cleanup refused: Server identity changed; manual cleanup required")
	}
	_, deleteErr := s.call(ctx, "cleanup-delete-server", map[string]any{"Zone": zone, "ID": serverID})
	servers, err := s.find(ctx, "cleanup-find-server-after", "server", name)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	if len(servers) != 0 {
		return errors.Join(deleteErr, fmt.Errorf("Server %q remains after cleanup; manual cleanup required", name))
	}
	disks, err := s.find(ctx, "cleanup-find-disk-after", "disk", name)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	if len(disks) != 0 {
		return errors.Join(deleteErr, fmt.Errorf("Disk %q remains after cleanup; manual cleanup required", name))
	}
	return errors.Join(deleteErr, s.cleanupInternet(ctx, name, knownInternetID, switchID))
}

func (s scenario) recoverInternet(name string) (internetItem, error) {
	timeout := s.recoveryTimeout
	if timeout <= 0 {
		timeout = createTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	interval := s.pollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastErr error
	for {
		items, err := s.findInternet(ctx, "cleanup-find-internet", name)
		switch {
		case err != nil:
			lastErr = err
		case len(items) > 1:
			return internetItem{}, errors.New("cannot safely identify created Internet: multiple matches; manual cleanup required")
		case len(items) == 1:
			candidate := items[0]
			if candidate.ID.IsEmpty() || candidate.Switch.ID.IsEmpty() {
				lastErr = errors.New("created Internet or Switch ID is not available yet")
			} else {
				if err := internetIdentity(candidate, candidate.ID, name, candidate.Switch.ID); err != nil {
					return internetItem{}, fmt.Errorf("Internet recovery refused: %w; manual cleanup required", err)
				}
				current, err := s.readInternet(ctx, "cleanup-read-internet", candidate.ID)
				if err != nil {
					lastErr = err
				} else {
					if err := internetIdentity(current, candidate.ID, name, candidate.Switch.ID); err != nil {
						return internetItem{}, fmt.Errorf("Internet recovery refused: %w; manual cleanup required", err)
					}
					return current, nil
				}
			}
		default:
			lastErr = errors.New("possibly created Internet is not visible yet")
		}
		select {
		case <-ctx.Done():
			return internetItem{}, errors.Join(
				fmt.Errorf("cannot recover possibly created Internet %q in %s within %s; manual cleanup required: %w", name, zone, timeout, ctx.Err()),
				lastErr,
			)
		case <-ticker.C:
		}
	}
}

func (s scenario) cleanupInternet(ctx context.Context, name string, knownID, knownSwitchID types.ID) error {
	id := knownID
	if id.IsEmpty() {
		matches, err := s.findInternet(ctx, "cleanup-find-internet", name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Internet: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() || matches[0].Description != internetDesc {
			return errors.New("cannot safely identify created Internet; manual cleanup required")
		}
		id = matches[0].ID
		if knownSwitchID.IsEmpty() {
			knownSwitchID = matches[0].Switch.ID
		}
	}
	current, err := s.readInternet(ctx, "cleanup-read-internet", id)
	if err != nil {
		return fmt.Errorf("cannot verify Internet before cleanup: %w", err)
	}
	if knownSwitchID.IsEmpty() {
		if current.ID != id || current.Name != name || current.Description != internetDesc ||
			current.NetworkMaskLen != 28 || current.BandWidthMbps != 100 || current.Switch.ID.IsEmpty() {
			return errors.New("cleanup refused: Internet identity cannot be verified; manual cleanup required")
		}
		knownSwitchID = current.Switch.ID
	}
	if err := internetIdentity(current, id, name, knownSwitchID); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	_, deleteErr := s.call(ctx, "cleanup-delete-internet", map[string]any{"Zone": zone, "ID": id})
	items, err := s.findInternet(ctx, "cleanup-find-internet-after", name)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	if len(items) != 0 {
		return errors.Join(deleteErr, fmt.Errorf("Internet %q remains after cleanup; manual cleanup required", name))
	}
	return deleteErr
}

func (s scenario) run(ctx context.Context, name string) (result error) {
	if _, err := s.call(ctx, "profile-current", nil); err != nil {
		return fmt.Errorf("selected SDK profile required: %w", err)
	}
	internetMatches, err := s.findInternet(ctx, "preflight-find-internet", name)
	if err != nil {
		return err
	}
	if len(internetMatches) != 0 {
		return fmt.Errorf("Internet name %q already exists; refusing to modify it", name)
	}
	for _, resource := range []string{"server", "disk"} {
		matches, err := s.find(ctx, "preflight-find-"+resource, resource, name)
		if err != nil {
			return err
		}
		if len(matches) != 0 {
			return fmt.Errorf("%s name %q already exists; refusing to modify it", resource, name)
		}
	}

	var internetID, switchID, serverID types.ID
	internetCreateUncertain := true
	defer func() {
		if internetCreateUncertain {
			recovered, err := s.recoverInternet(name)
			if err != nil {
				result = errors.Join(result, fmt.Errorf("cleanup failed: %w", err))
				return
			}
			internetID, switchID = recovered.ID, recovered.Switch.ID
		}
		if cleanupErr := s.cleanup(name, internetID, switchID, serverID); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()
	created, err := s.call(ctx, "test-create-internet", map[string]any{"Zone": zone, "Name": name})
	if err != nil {
		return err
	}
	var router internetItem
	if err := json.Unmarshal(created, &router); err != nil {
		return fmt.Errorf("decode created Internet: %w", err)
	}
	internetID, switchID = router.ID, router.Switch.ID
	if internetID.IsEmpty() || switchID.IsEmpty() {
		return errors.New("created Internet or Switch ID is empty")
	}
	if err := internetIdentity(router, internetID, name, switchID); err != nil {
		return err
	}
	internetCreateUncertain = false

	if err := s.updateInternetAndVerify(ctx, internetID, name, switchID); err != nil {
		return err
	}
	if err := s.monitorRouter(ctx, internetID); err != nil {
		return err
	}

	serverOutput, err := s.call(ctx, "test-create-server", map[string]any{
		"Zone": zone, "Name": name, "CloudimgArchiveID": s.archiveID,
		"DiskPlanID": s.diskPlanID, "DiskSizeGB": s.diskSizeGB, "SwitchID": int64(switchID),
	})
	if err != nil {
		return err
	}
	var server serverItem
	if err := json.Unmarshal(serverOutput, &server); err != nil {
		return fmt.Errorf("decode created Server: %w", err)
	}
	serverID = server.ID
	if serverID.IsEmpty() {
		return errors.New("created Server ID is empty")
	}
	if err := serverIdentity(server, serverID, name, switchID); err != nil {
		return err
	}
	if len(server.Interfaces) == 0 {
		return errors.New("created Server has no NIC")
	}
	iface, prefix, gateway, err := s.network(ctx, internetID, switchID, server)
	if err != nil {
		return err
	}
	if err := writeUserData(s.userDataPath, s.publicKey, iface.MACAddress, iface.IPAddress, prefix, gateway); err != nil {
		return err
	}
	if _, err := s.call(ctx, "test-boot-server", map[string]any{
		"Zone": zone, "ID": serverID, "UserDataPath": s.userDataPath,
	}); err != nil {
		return err
	}
	if err := s.waitForSSH(ctx, serverID, name, switchID, iface.IPAddress); err != nil {
		return err
	}
	if err := s.subnetScenario(ctx, internetID, iface.IPAddress); err != nil {
		return err
	}
	return s.ipv6Scenario(ctx, internetID)
}

func createSSHMaterial(dir string) (userDataPath, privateKey, knownHosts, publicKey string, err error) {
	privateKey = filepath.Join(dir, "ssh-key")
	keygen := exec.Command("ssh-keygen", "-t", "rsa", "-b", "3072", "-N", "", "-f", privateKey) //nolint:gosec // Generates an ephemeral local test key without a shell.
	keygen.Stdout = nil
	keygen.Stderr = nil
	if err := keygen.Run(); err != nil {
		return "", "", "", "", fmt.Errorf("generate temporary SSH key: %w", err)
	}
	if err := os.Chmod(privateKey, 0o600); err != nil {
		return "", "", "", "", fmt.Errorf("protect temporary private SSH key: %w", err)
	}
	//nolint:gosec // privateKey is created inside this runner's private temporary directory.
	data, err := os.ReadFile(privateKey + ".pub")
	if err != nil {
		return "", "", "", "", fmt.Errorf("read temporary public SSH key: %w", err)
	}
	return filepath.Join(dir, "user-data.yaml"), privateKey, filepath.Join(dir, "known_hosts"), strings.TrimSpace(string(data)), nil
}

func finalizeRun(exitCode int, cleanup func() error, saveResult func(string) error) (int, error) {
	cleanupErr := cleanup()
	if cleanupErr != nil {
		exitCode = 1
	}
	var resultErr error
	if saveResult != nil {
		result := "failed"
		if exitCode == 0 {
			result = "passed"
		}
		resultErr = saveResult(result)
		if resultErr != nil {
			exitCode = 1
		}
	}
	return exitCode, errors.Join(cleanupErr, resultErr)
}

func runMain() (exitCode int) {
	binary := flag.String("skr", "./skr", "Path to a built skr binary")
	archiveID := flag.Int64("cloudimg-archive-id", 0, "cloud-init archive ID available in is1b")
	diskPlanID := flag.Int64("disk-plan-id", 0, "Disk plan ID available in is1b")
	diskSizeGB := flag.Int64("disk-size-gb", defaultDiskSize, "Cloud-init Disk size in GB")
	confirmed := flag.Bool("confirm-is1b-live", false, "Confirm creating, booting, SSH-connecting to, and deleting a temporary Internet, Server, and Disk in is1b")
	flag.Parse()
	if flag.NArg() != 0 || !*confirmed || *archiveID <= 0 || *diskPlanID <= 0 || *diskSizeGB <= 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./test/e2e/internet --skr ./skr --cloudimg-archive-id ID --disk-plan-id ID [--disk-size-gb 20] --confirm-is1b-live")
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
	dir, err := os.MkdirTemp("", "skr-e2e-internet-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temporary SSH material directory:", err)
		return 1
	}
	var saveResult func(string) error
	defer func() {
		var err error
		exitCode, err = finalizeRun(exitCode, func() error {
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("remove temporary SSH material: %w", err)
			}
			return nil
		}, saveResult)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		if exitCode == 0 {
			fmt.Println("Internet E2E passed; the Router + Switch NIC was configured, SSH login succeeded, and the Server, Disk, and Internet were deleted.")
		}
	}()
	userDataPath, privateKey, knownHosts, publicKey, err := createSSHMaterial(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	recorder, err := evidence.New("internet-api", evidence.Scenario{
		Title:       "ルータ＋スイッチの作成・更新・サブネット・IPv6 操作",
		Description: "IaaS ルータ＋スイッチの作成・参照・更新・削除に加え、サブネット追加・更新・削除と IPv6 有効化・無効化、モニタリングが正しく動作することを確認します。",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Evidence (private, ignored by Git):", recorder.Dir())

	saveResult = func(result string) error {
		return recorder.SetResult(result, time.Now())
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fmt.Fprintln(os.Stderr, "generate unique E2E resource name:", err)
		return 1
	}
	name := namePrefix + hex.EncodeToString(suffix[:])
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, scenarioTimeout)
	defer cancel()
	runner := cliRunner{binary: path, evidence: recorder, userDataPath: userDataPath}
	ssh := sshRunner{evidence: recorder, run: runSSH}
	test := scenario{
		client: runner, archiveID: *archiveID, diskPlanID: *diskPlanID, diskSizeGB: *diskSizeGB,
		userDataPath: userDataPath, publicKey: publicKey, privateKey: privateKey, knownHosts: knownHosts,
		connect: ssh.connect, pollInterval: 10 * time.Second,
	}
	if err := test.run(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr, "Internet E2E failed:", err)
		fmt.Fprintln(os.Stderr, "Evidence retained at:", recorder.Dir())
		return 1
	}
	fmt.Println("Evidence retained at:", recorder.Dir())
	return 0
}

func main() {
	os.Exit(runMain())
}
