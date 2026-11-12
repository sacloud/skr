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

package apprundedicated

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"testing"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	autoscalinggroup "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/autoscalinggroup"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	workernode "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/workernode"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

// fakeApp はメモリ上のアプリケーションです。
type fakeApp struct {
	applicationID v1.ApplicationID
	name          string
	clusterID     v1.ClusterID
	clusterName   string
	activeVersion *int32
	desiredCount  *int32
	versions      map[v1.ApplicationVersionNumber]*version.VersionDetail
	nextVersion   v1.ApplicationVersionNumber
	containers    []application.Placement
	// deleteFailures は Delete がこの回数だけ失敗することを表します (再試行の検証用)。
	deleteFailures int
	deleted        bool
	// lingeringReads は削除後も参照がこの回数だけ存在を返すことを表します (遅延の再現用)。
	lingeringReads int
}

func (a *fakeApp) readApplication() v1.ReadApplication {
	return v1.ReadApplication{
		ApplicationID: a.applicationID,
		Name:          a.name,
		ClusterID:     a.clusterID,
		ClusterName:   a.clusterName,
	}
}

// fakeCluster はメモリ上のクラスタです。
type fakeCluster struct {
	clusterID v1.ClusterID
	name      string
	deleted   bool
}

// fakeAutoScalingGroup はメモリ上のオートスケーリンググループです。
type fakeAutoScalingGroup struct {
	clusterID              v1.ClusterID
	autoScalingGroupID     v1.AutoScalingGroupID
	name                   string
	zone                   string
	workerServiceClassPath string
	minNodes               int32
	maxNodes               int32
	deleted                bool
}

func (a *fakeAutoScalingGroup) readAutoScalingGroup() v1.ReadAutoScalingGroup {
	return v1.ReadAutoScalingGroup{
		AutoScalingGroupID:     a.autoScalingGroupID,
		Name:                   a.name,
		Zone:                   a.zone,
		WorkerServiceClassPath: a.workerServiceClassPath,
		MinNodes:               a.minNodes,
		MaxNodes:               a.maxNodes,
	}
}

// fakeWorkerNode はメモリ上のワーカノードです。
type fakeWorkerNode struct {
	clusterID          v1.ClusterID
	autoScalingGroupID v1.AutoScalingGroupID
	workerNodeID       v1.WorkerNodeID
	healthy            bool
	deleted            bool
}

// fakeSDK は application・cluster・version の操作をメモリ上で再現する fake です。
//
// application.ApplicationAPI と cluster.ClusterAPI は同じ名前で異なるシグネチャの
// メソッドを持つため、それぞれ fakeApplicationAPI と fakeClusterAPI で公開します。
// ページングと変更操作の呼び出し記録を提供します。
type fakeSDK struct {
	clusters    []*fakeCluster
	apps        []*fakeApp
	asgs        []*fakeAutoScalingGroup
	workerNodes []*fakeWorkerNode

	// deactivatedDesiredCount はアクティブバージョン解除後の DesiredCount です。
	deactivatedDesiredCount *int32

	createApplicationCalls []struct {
		name      string
		clusterID v1.ClusterID
	}
	updateApplicationCalls []struct {
		id        v1.ApplicationID
		toVersion *int32
	}
	deleteApplicationCalls []v1.ApplicationID
	createVersionCalls     []version.CreateParams

	listErr, readErr, createApplicationErr, updateErr, createVersionErr error

	// containersErrCount は Containers がこの回数だけ失敗することを表します (再試行の検証用)。
	containersErrCount int
	// containersCalls は Containers の呼び出し回数です。
	containersCalls int

	app     *fakeApplicationAPI
	cluster *fakeClusterAPI

	nextID byte
}

func newFakeSDK(_ *testing.T) *fakeSDK {
	zero := int32(0)
	f := &fakeSDK{deactivatedDesiredCount: &zero}
	f.app = &fakeApplicationAPI{sdk: f}
	f.cluster = &fakeClusterAPI{sdk: f}
	return f
}

func (f *fakeSDK) addCluster(t *testing.T, name string) *fakeCluster {
	t.Helper()
	c := &fakeCluster{clusterID: v1.ClusterID(f.newID()), name: name}
	f.clusters = append(f.clusters, c)
	return c
}

func (f *fakeSDK) addApp(t *testing.T, clusterName, name string) *fakeApp {
	t.Helper()
	var c *fakeCluster
	for _, e := range f.clusters {
		if e.name == clusterName {
			c = e
			break
		}
	}
	if c == nil {
		panic("テストの準備に失敗しました: クラスタ " + clusterName + " がありません")
	}
	app := &fakeApp{
		applicationID: v1.ApplicationID(f.newID()),
		name:          name,
		clusterID:     c.clusterID,
		clusterName:   c.name,
		versions:      map[v1.ApplicationVersionNumber]*version.VersionDetail{},
		nextVersion:   1,
	}
	f.apps = append(f.apps, app)
	return app
}

func (f *fakeSDK) addAutoScalingGroup(t *testing.T, clusterName, name, zone, workerServiceClassPath string, minNodes, maxNodes int32) *fakeAutoScalingGroup {
	t.Helper()
	var c *fakeCluster
	for _, e := range f.clusters {
		if e.name == clusterName {
			c = e
			break
		}
	}
	if c == nil {
		panic("テストの準備に失敗しました: クラスタ " + clusterName + " がありません")
	}
	asg := &fakeAutoScalingGroup{
		clusterID:              c.clusterID,
		autoScalingGroupID:     v1.AutoScalingGroupID(f.newID()),
		name:                   name,
		zone:                   zone,
		workerServiceClassPath: workerServiceClassPath,
		minNodes:               minNodes,
		maxNodes:               maxNodes,
	}
	f.asgs = append(f.asgs, asg)
	return asg
}

func (f *fakeSDK) addWorkerNode(t *testing.T, clusterName, asgName string, healthy bool) *fakeWorkerNode {
	t.Helper()
	var c *fakeCluster
	for _, e := range f.clusters {
		if e.name == clusterName {
			c = e
			break
		}
	}
	if c == nil {
		panic("テストの準備に失敗しました: クラスタ " + clusterName + " がありません")
	}
	var asg *fakeAutoScalingGroup
	for _, a := range f.asgs {
		if a.clusterID == c.clusterID && a.name == asgName {
			asg = a
			break
		}
	}
	if asg == nil {
		panic("テストの準備に失敗しました: オートスケーリンググループ " + asgName + " がありません")
	}
	node := &fakeWorkerNode{
		clusterID:          c.clusterID,
		autoScalingGroupID: asg.autoScalingGroupID,
		workerNodeID:       v1.WorkerNodeID(f.newID()),
		healthy:            healthy,
	}
	f.workerNodes = append(f.workerNodes, node)
	return node
}

// newID は重複しない識別子を返します。255 を超えると下位 8 ビットが折り返しますが、
// テストで作成するリソースの数では重複しません。
func (f *fakeSDK) newID() [16]byte {
	f.nextID++
	var id [16]byte
	id[0] = f.nextID
	return id
}

func notFoundError(msg string) error {
	return saclient.NewError(http.StatusNotFound, msg, errors.New(msg))
}

// --- application.ApplicationAPI ---

// fakeApplicationAPI は application.ApplicationAPI の fake です。
type fakeApplicationAPI struct {
	sdk *fakeSDK
}

func (f *fakeApplicationAPI) List(ctx context.Context, maxItems int64, cursor *v1.ApplicationID) ([]v1.ReadApplication, *v1.ApplicationID, error) {
	if f.sdk.listErr != nil {
		return nil, nil, f.sdk.listErr
	}
	var items []v1.ReadApplication
	for _, a := range f.sdk.apps {
		if !a.deleted {
			items = append(items, a.readApplication())
		}
	}
	return pageItems(items, maxItems, cursor, func(r v1.ReadApplication) v1.ApplicationID { return r.ApplicationID })
}

func (f *fakeApplicationAPI) Create(ctx context.Context, name string, clusterID v1.ClusterID) (*v1.ApplicationIdentifier, error) {
	if f.sdk.createApplicationErr != nil {
		return nil, f.sdk.createApplicationErr
	}
	f.sdk.createApplicationCalls = append(f.sdk.createApplicationCalls, struct {
		name      string
		clusterID v1.ClusterID
	}{name: name, clusterID: clusterID})
	var clusterName string
	for _, c := range f.sdk.clusters {
		if c.clusterID == clusterID {
			clusterName = c.name
			break
		}
	}
	app := &fakeApp{
		applicationID: v1.ApplicationID(f.sdk.newID()),
		name:          name,
		clusterID:     clusterID,
		clusterName:   clusterName,
		versions:      map[v1.ApplicationVersionNumber]*version.VersionDetail{},
		nextVersion:   1,
	}
	f.sdk.apps = append(f.sdk.apps, app)
	return &v1.ApplicationIdentifier{ApplicationID: app.applicationID}, nil
}

func (f *fakeApplicationAPI) Read(ctx context.Context, id v1.ApplicationID) (*application.ApplicationDetail, error) {
	if f.sdk.readErr != nil {
		return nil, f.sdk.readErr
	}
	app, ok := f.sdk.findApp(id)
	if !ok {
		// 削除済みのアプリケーションが参照の遅延で存在を返すケースを再現します。
		if a, found := f.sdk.findAppIncludingDeleted(id); found && a.lingeringReads > 0 {
			a.lingeringReads--
			app, ok = a, true
		}
	}
	if !ok {
		return nil, notFoundError("application not found")
	}
	return &application.ApplicationDetail{
		ApplicationID: app.applicationID,
		Name:          app.name,
		ClusterID:     app.clusterID,
		ClusterName:   app.clusterName,
		ActiveVersion: app.activeVersion,
		DesiredCount:  app.desiredCount,
	}, nil
}

func (f *fakeApplicationAPI) Update(ctx context.Context, id v1.ApplicationID, toVersion *int32) error {
	if f.sdk.updateErr != nil {
		return f.sdk.updateErr
	}
	app, ok := f.sdk.findApp(id)
	if !ok {
		return notFoundError("application not found")
	}
	f.sdk.updateApplicationCalls = append(f.sdk.updateApplicationCalls, struct {
		id        v1.ApplicationID
		toVersion *int32
	}{id: id, toVersion: toVersion})
	if toVersion == nil {
		app.activeVersion = nil
		app.desiredCount = f.sdk.deactivatedDesiredCount
		return nil
	}
	v := v1.ApplicationVersionNumber(*toVersion)
	detail, ok := app.versions[v]
	if !ok {
		return saclient.NewError(http.StatusBadRequest, "version not found", errors.New("version not found"))
	}
	app.activeVersion = toVersion
	switch detail.ScalingMode {
	case v1.ScalingModeManual:
		app.desiredCount = detail.FixedScale
	case v1.ScalingModeCPU:
		app.desiredCount = detail.MinScale
	}
	return nil
}

func (f *fakeApplicationAPI) Delete(ctx context.Context, id v1.ApplicationID) error {
	app, ok := f.sdk.findApp(id)
	if !ok {
		return notFoundError("application not found")
	}
	if app.deleteFailures > 0 {
		app.deleteFailures--
		return saclient.NewError(http.StatusConflict, "conflict", errors.New("application has running containers"))
	}
	f.sdk.deleteApplicationCalls = append(f.sdk.deleteApplicationCalls, id)
	app.deleted = true
	return nil
}

func (f *fakeApplicationAPI) Containers(ctx context.Context, id v1.ApplicationID) ([]application.Placement, error) {
	f.sdk.containersCalls++
	if f.sdk.containersErrCount > 0 {
		f.sdk.containersErrCount--
		return nil, errors.New("一時的なエラー")
	}
	app, ok := f.sdk.findApp(id)
	if !ok {
		return nil, notFoundError("application not found")
	}
	return app.containers, nil
}

// --- cluster.ClusterAPI ---

// fakeClusterAPI は cluster.ClusterAPI の fake です。
type fakeClusterAPI struct {
	sdk *fakeSDK
}

func (f *fakeClusterAPI) List(ctx context.Context, maxItems int64, cursor *v1.ClusterID) ([]cluster.ClusterDetail, *v1.ClusterID, error) {
	if f.sdk.listErr != nil {
		return nil, nil, f.sdk.listErr
	}
	var items []cluster.ClusterDetail
	for _, c := range f.sdk.clusters {
		if c.deleted {
			continue
		}
		items = append(items, cluster.ClusterDetail{ClusterID: c.clusterID, Name: c.name})
	}
	return pageItems(items, maxItems, cursor, func(c cluster.ClusterDetail) v1.ClusterID { return c.ClusterID })
}

func (f *fakeClusterAPI) Create(ctx context.Context, params cluster.CreateParams) (*v1.ClusterIdentifier, error) {
	return nil, errors.New("fakeSDK はクラスタの作成に対応しません")
}

func (f *fakeClusterAPI) Read(ctx context.Context, id v1.ClusterID) (*cluster.ClusterDetail, error) {
	for _, c := range f.sdk.clusters {
		if c.clusterID == id && !c.deleted {
			return &cluster.ClusterDetail{ClusterID: c.clusterID, Name: c.name}, nil
		}
	}
	return nil, notFoundError("cluster not found")
}

func (f *fakeClusterAPI) Update(ctx context.Context, id v1.ClusterID, params cluster.UpdateParams) error {
	return errors.New("fakeSDK はクラスタの更新に対応しません")
}

func (f *fakeClusterAPI) Delete(ctx context.Context, id v1.ClusterID) error {
	for _, c := range f.sdk.clusters {
		if c.clusterID == id {
			if c.deleted {
				return notFoundError("cluster not found")
			}
			c.deleted = true
			return nil
		}
	}
	return notFoundError("cluster not found")
}

func (f *fakeClusterAPI) ReadLoadBalancer(ctx context.Context, id v1.ClusterID) (*v1.ReadClusterLoadBalancer, error) {
	return nil, errors.New("fakeSDK はロードバランサの参照に対応しません")
}

func (f *fakeClusterAPI) UpdateLoadBalancer(ctx context.Context, id v1.ClusterID, params cluster.UpdateLoadBalancerParams) error {
	return errors.New("fakeSDK はロードバランサの更新に対応しません")
}

// --- autoscalinggroup.AutoScalingGroupAPI ---

// fakeAutoScalingGroupAPI は autoscalinggroup.AutoScalingGroupAPI の fake です。
type fakeAutoScalingGroupAPI struct {
	sdk       *fakeSDK
	clusterID v1.ClusterID
}

func (f *fakeAutoScalingGroupAPI) List(ctx context.Context, maxItems int64, cursor *v1.AutoScalingGroupID) ([]v1.ReadAutoScalingGroup, *v1.AutoScalingGroupID, error) {
	var items []v1.ReadAutoScalingGroup
	for _, a := range f.sdk.asgs {
		if a.clusterID != f.clusterID || a.deleted {
			continue
		}
		items = append(items, a.readAutoScalingGroup())
	}
	return pageItems(items, maxItems, cursor, func(r v1.ReadAutoScalingGroup) v1.AutoScalingGroupID { return r.AutoScalingGroupID })
}

func (f *fakeAutoScalingGroupAPI) Create(ctx context.Context, params autoscalinggroup.CreateParams) (*v1.AutoScalingGroupIdentifier, error) {
	asg := &fakeAutoScalingGroup{
		clusterID:              f.clusterID,
		autoScalingGroupID:     v1.AutoScalingGroupID(f.sdk.newID()),
		name:                   params.Name,
		zone:                   params.Zone,
		workerServiceClassPath: params.WorkerServiceClassPath,
		minNodes:               params.MinNodes,
		maxNodes:               params.MaxNodes,
	}
	f.sdk.asgs = append(f.sdk.asgs, asg)
	return &v1.AutoScalingGroupIdentifier{AutoScalingGroupID: asg.autoScalingGroupID}, nil
}

func (f *fakeAutoScalingGroupAPI) Read(ctx context.Context, id v1.AutoScalingGroupID) (*autoscalinggroup.AutoScalingGroupDetail, error) {
	for _, a := range f.sdk.asgs {
		if a.autoScalingGroupID != id {
			continue
		}
		if a.deleted {
			return nil, notFoundError("auto scaling group not found")
		}
		return &autoscalinggroup.AutoScalingGroupDetail{
			AutoScalingGroupID:     a.autoScalingGroupID,
			Name:                   a.name,
			Zone:                   a.zone,
			WorkerServiceClassPath: a.workerServiceClassPath,
			MinNodes:               a.minNodes,
			MaxNodes:               a.maxNodes,
		}, nil
	}
	return nil, notFoundError("auto scaling group not found")
}

func (f *fakeAutoScalingGroupAPI) Delete(ctx context.Context, id v1.AutoScalingGroupID) error {
	for _, a := range f.sdk.asgs {
		if a.autoScalingGroupID != id {
			continue
		}
		if a.deleted {
			return notFoundError("auto scaling group not found")
		}
		a.deleted = true
		return nil
	}
	return notFoundError("auto scaling group not found")
}

// --- workernode.WorkerNodeAPI ---

// fakeWorkerNodeAPI は workernode.WorkerNodeAPI の fake です。
type fakeWorkerNodeAPI struct {
	sdk                *fakeSDK
	clusterID          v1.ClusterID
	autoScalingGroupID v1.AutoScalingGroupID
}

func (f *fakeWorkerNodeAPI) List(ctx context.Context, maxItems int64, cursor *v1.WorkerNodeID) ([]workernode.WorkerNodeDetail, *v1.WorkerNodeID, error) {
	var items []workernode.WorkerNodeDetail
	for _, n := range f.sdk.workerNodes {
		if n.clusterID != f.clusterID || n.autoScalingGroupID != f.autoScalingGroupID || n.deleted {
			continue
		}
		items = append(items, workernode.WorkerNodeDetail{WorkerNodeID: n.workerNodeID, Healthy: n.healthy})
	}
	return pageItems(items, maxItems, cursor, func(r workernode.WorkerNodeDetail) v1.WorkerNodeID { return r.WorkerNodeID })
}

func (f *fakeWorkerNodeAPI) Read(ctx context.Context, id v1.WorkerNodeID) (*workernode.WorkerNodeDetail, error) {
	for _, n := range f.sdk.workerNodes {
		if n.workerNodeID != id {
			continue
		}
		if n.deleted {
			return nil, notFoundError("worker node not found")
		}
		return &workernode.WorkerNodeDetail{
			WorkerNodeID: n.workerNodeID,
			Healthy:      n.healthy,
		}, nil
	}
	return nil, notFoundError("worker node not found")
}

func (f *fakeWorkerNodeAPI) Update(ctx context.Context, id v1.WorkerNodeID, draining bool) error {
	return errors.New("fakeSDK はワーカノードの更新に対応しません")
}

// --- version.VersionAPI ---

// fakeVersionAPI は version.VersionAPI の fake です。
type fakeVersionAPI struct {
	sdk *fakeSDK
	app *fakeApp
}

func (f *fakeSDK) newVersionAPI(id v1.ApplicationID) version.VersionAPI {
	app, _ := f.findApp(id)
	return &fakeVersionAPI{sdk: f, app: app}
}

func (f *fakeSDK) newAutoScalingGroupAPI(clusterID v1.ClusterID) autoscalinggroup.AutoScalingGroupAPI {
	return &fakeAutoScalingGroupAPI{sdk: f, clusterID: clusterID}
}

func (f *fakeSDK) newWorkerNodeAPI(clusterID v1.ClusterID, autoScalingGroupID v1.AutoScalingGroupID) workernode.WorkerNodeAPI {
	return &fakeWorkerNodeAPI{sdk: f, clusterID: clusterID, autoScalingGroupID: autoScalingGroupID}
}

func (v *fakeVersionAPI) List(ctx context.Context, maxItems int64, cursor *v1.ApplicationVersionNumber) ([]v1.ApplicationVersionSummary, *v1.ApplicationVersionNumber, error) {
	numbers := make([]v1.ApplicationVersionNumber, 0, len(v.app.versions))
	for n := range v.app.versions {
		numbers = append(numbers, n)
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] > numbers[j] })
	var items []v1.ApplicationVersionSummary
	for _, n := range numbers {
		detail := v.app.versions[n]
		items = append(items, v1.ApplicationVersionSummary{
			Version:         detail.Version,
			Image:           detail.Image,
			ActiveNodeCount: detail.ActiveNodeCount,
			Created:         detail.Created,
		})
	}
	return pageItems(items, maxItems, cursor, func(s v1.ApplicationVersionSummary) v1.ApplicationVersionNumber { return s.Version })
}

func (v *fakeVersionAPI) Create(ctx context.Context, params version.CreateParams) (*v1.ApplicationVersionIdentifier, error) {
	if v.sdk.createVersionErr != nil {
		return nil, v.sdk.createVersionErr
	}
	v.sdk.createVersionCalls = append(v.sdk.createVersionCalls, params)
	n := v.app.nextVersion
	v.app.nextVersion++
	v.app.versions[n] = &version.VersionDetail{
		Version:           n,
		CPU:               params.CPU,
		Memory:            params.Memory,
		ScalingMode:       params.ScalingMode,
		FixedScale:        params.FixedScale,
		MinScale:          params.MinScale,
		MaxScale:          params.MaxScale,
		ScaleInThreshold:  params.ScaleInThreshold,
		ScaleOutThreshold: params.ScaleOutThreshold,
		Image:             params.Image,
		Cmd:               params.Cmd,
		RegistryUsername:  params.RegistryUsername,
		ActiveNodeCount:   1,
		Created:           1700000000,
		ExposedPorts:      params.ExposedPorts,
		EnvVars:           params.EnvVars,
	}
	return &v1.ApplicationVersionIdentifier{Version: n}, nil
}

func (v *fakeVersionAPI) Read(ctx context.Context, id v1.ApplicationVersionNumber) (*version.VersionDetail, error) {
	detail, ok := v.app.versions[id]
	if !ok {
		return nil, notFoundError("version not found")
	}
	copied := *detail
	return &copied, nil
}

func (v *fakeVersionAPI) Delete(ctx context.Context, id v1.ApplicationVersionNumber) error {
	if _, ok := v.app.versions[id]; !ok {
		return notFoundError("version not found")
	}
	delete(v.app.versions, id)
	return nil
}

// --- 共通ヘルパー ---

func (f *fakeSDK) findApp(id v1.ApplicationID) (*fakeApp, bool) {
	for _, a := range f.apps {
		if a.applicationID == id && !a.deleted {
			return a, true
		}
	}
	return nil, false
}

// findAppIncludingDeleted は削除済みを含めて識別子でアプリケーションを検索します。
func (f *fakeSDK) findAppIncludingDeleted(id v1.ApplicationID) (*fakeApp, bool) {
	for _, a := range f.apps {
		if a.applicationID == id {
			return a, true
		}
	}
	return nil, false
}

// pageItems は maxItems と cursor によるページングを再現します。
func pageItems[T any, ID comparable](items []T, maxItems int64, cursor *ID, id func(T) ID) ([]T, *ID, error) {
	start := 0
	if cursor != nil {
		for i, item := range items {
			if id(item) == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + int(maxItems)
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	var next *ID
	if end < len(items) {
		nextID := id(page[len(page)-1])
		next = &nextID
	}
	return page, next, nil
}

func ptrInt32(v int32) *int32    { return &v }
func ptrString(v string) *string { return &v }

func ptrVersion(v int32) *v1.ApplicationVersionNumber {
	n := v1.ApplicationVersionNumber(v)
	return &n
}

func ptrPort(v uint16) *v1.Port {
	p := v1.Port(v)
	return &p
}
