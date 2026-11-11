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
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/stretchr/testify/require"
)

const validDefinitionJSON = `{
	"cluster": "c1",
	"name": "app1",
	"cpu": 1000,
	"memory": 1024,
	"scalingMode": "manual",
	"fixedScale": 2,
	"image": "ghcr.io/example/app:v1",
	"cmd": ["serve"],
	"registryUsername": "user",
	"exposedPorts": [
		{
			"targetPort": 8080,
			"loadBalancerPort": 80,
			"useLetsEncrypt": true,
			"host": ["app.example.com"],
			"healthCheck": {"path": "/health", "intervalSeconds": 30, "timeoutSeconds": 5}
		}
	],
	"env": [
		{"key": "LOG_LEVEL", "value": "info", "secret": false},
		{"key": "API_TOKEN", "value": "token", "secret": true}
	]
}`

func TestParseDefinition(t *testing.T) {
	def, err := ParseDefinition([]byte(validDefinitionJSON))
	require.NoError(t, err)
	require.Equal(t, "c1", def.Cluster)
	require.Equal(t, "app1", def.Name)
	require.Equal(t, int64(1000), def.CPU)
	require.Equal(t, int64(1024), def.Memory)
	require.Equal(t, v1.ScalingModeManual, def.ScalingMode)
	require.Equal(t, ptrInt32(2), def.FixedScale)
	require.Equal(t, "ghcr.io/example/app:v1", def.Image)
	require.Equal(t, []string{"serve"}, def.Cmd)
	require.Equal(t, ptrString("user"), def.RegistryUsername)
	require.Nil(t, def.RegistryPassword)
	require.Len(t, def.ExposedPorts, 1)
	port := def.ExposedPorts[0]
	require.Equal(t, v1.Port(8080), port.TargetPort)
	require.Equal(t, ptrPort(80), port.LoadBalancerPort)
	require.True(t, port.UseLetsEncrypt)
	require.Equal(t, []string{"app.example.com"}, port.Host)
	require.NotNil(t, port.HealthCheck)
	require.Equal(t, "/health", port.HealthCheck.Path)
	require.Equal(t, int32(30), port.HealthCheck.IntervalSeconds)
	require.Equal(t, int32(5), port.HealthCheck.TimeoutSeconds)
	require.Len(t, def.Env, 2)
	require.Equal(t, "LOG_LEVEL", def.Env[0].Key)
	require.Equal(t, "info", *def.Env[0].Value)
	require.False(t, def.Env[0].Secret)
	require.True(t, def.Env[1].Secret)
}

func TestParseDefinitionEmpty(t *testing.T) {
	// すべてのフィールドを省略してもデコードできます。
	def, err := ParseDefinition([]byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, &ApplicationDefinition{}, def)
}

func TestParseDefinitionErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"未知のフィールド", `{"cluster":"c1","name":"app1","unknown":1}`},
		{"末尾の余分なデータ", `{"cluster":"c1","name":"app1"} trailing`},
		{"複数の JSON", `{"cluster":"c1","name":"app1"}{"cluster":"c2","name":"app2"}`},
		{"不正な JSON", `{`},
		{"空の入力", ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDefinition([]byte(tc.data))
			require.Error(t, err)
		})
	}
}

func TestValidateForDeploy(t *testing.T) {
	valid := manualDef("c1", "app1")
	require.NoError(t, valid.Validate())
	require.NoError(t, valid.ValidateForDeploy())

	cpuDef := manualDef("c1", "app1")
	cpuDef.ScalingMode = v1.ScalingModeCPU
	cpuDef.FixedScale = nil
	cpuDef.MinScale = ptrInt32(1)
	cpuDef.MaxScale = ptrInt32(3)
	require.NoError(t, cpuDef.Validate())
	require.NoError(t, cpuDef.ValidateForDeploy())

	tests := []struct {
		name    string
		mutate  func(*ApplicationDefinition)
		wantErr string
	}{
		{"cluster なし", func(d *ApplicationDefinition) { d.Cluster = "" }, "cluster は必須です"},
		{"name なし", func(d *ApplicationDefinition) { d.Name = "" }, "name は必須です"},
		{"cpu なし", func(d *ApplicationDefinition) { d.CPU = 0 }, "cpu には正の整数を指定してください"},
		{"memory なし", func(d *ApplicationDefinition) { d.Memory = 0 }, "memory には正の整数を指定してください"},
		{"scalingMode なし", func(d *ApplicationDefinition) { d.ScalingMode = "" }, "scalingMode は必須です"},
		{"scalingMode が不正", func(d *ApplicationDefinition) { d.ScalingMode = "fixed" }, `scalingMode は manual または cpu で指定してください`},
		{"manual で fixedScale なし", func(d *ApplicationDefinition) { d.FixedScale = nil }, "scalingMode が manual のときは fixedScale が必須です"},
		{"cpu で minScale なし", func(d *ApplicationDefinition) {
			d.ScalingMode = v1.ScalingModeCPU
			d.FixedScale = nil
			d.MinScale = nil
		}, "scalingMode が cpu のときは minScale と maxScale が必須です"},
		{"cpu で maxScale なし", func(d *ApplicationDefinition) {
			d.ScalingMode = v1.ScalingModeCPU
			d.FixedScale = nil
			d.MaxScale = nil
		}, "scalingMode が cpu のときは minScale と maxScale が必須です"},
		{"image なし", func(d *ApplicationDefinition) { d.Image = "" }, "image は必須です"},
		{"loadBalancerPort あり host なし", func(d *ApplicationDefinition) {
			d.ExposedPorts = []version.ExposedPort{{TargetPort: 8080, LoadBalancerPort: ptrPort(80)}}
		}, "exposedPorts[0]: loadBalancerPort を指定したときは host が 1 件以上必要です"},
		{"host あり loadBalancerPort なし", func(d *ApplicationDefinition) {
			d.ExposedPorts = []version.ExposedPort{{TargetPort: 8080, Host: []string{"app.example.com"}}}
		}, "exposedPorts[0]: loadBalancerPort を指定しないときは host を指定できません"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := manualDef("c1", "app1")
			tc.mutate(def)
			err := def.ValidateForDeploy()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateIdentifierOnly(t *testing.T) {
	// 識別子だけの定義 (Init の出力) は Validate に合格します。
	def := &ApplicationDefinition{Cluster: "c1", Name: "app1"}
	require.NoError(t, def.Validate())
	err := def.ValidateForDeploy()
	require.Error(t, err)
	require.Contains(t, err.Error(), "cpu には正の整数を指定してください")
}

func TestValidateForDeployMultipleErrors(t *testing.T) {
	// 複数の問題があっても、最初のエラーを返します。
	def := &ApplicationDefinition{Name: "app1", Image: "img"}
	err := def.ValidateForDeploy()
	require.Error(t, err)
	require.Contains(t, err.Error(), "cluster は必須です")
}

func TestCreateParams(t *testing.T) {
	def := manualDef("c1", "app1")
	def.Cmd = []string{"serve"}
	def.RegistryUsername = ptrString("user")
	def.RegistryPassword = ptrString("secret")
	def.ScaleInThreshold = ptrInt32(10)
	def.ScaleOutThreshold = ptrInt32(90)
	def.ExposedPorts = []version.ExposedPort{
		{
			TargetPort:       8080,
			LoadBalancerPort: ptrPort(80),
			UseLetsEncrypt:   true,
			Host:             []string{"app.example.com"},
			HealthCheck:      &v1.HealthCheck{Path: "/health", IntervalSeconds: 30, TimeoutSeconds: 5},
		},
	}
	def.Env = []version.EnvironmentVariable{
		{Key: "LOG_LEVEL", Value: ptrString("info")},
		{Key: "API_TOKEN", Value: ptrString("token"), Secret: true},
	}

	params := def.createParams(v1.RegistryPasswordActionNew)
	require.Equal(t, int64(1000), params.CPU)
	require.Equal(t, int64(1024), params.Memory)
	require.Equal(t, v1.ScalingModeManual, params.ScalingMode)
	require.Equal(t, ptrInt32(2), params.FixedScale)
	require.Nil(t, params.MinScale)
	require.Nil(t, params.MaxScale)
	require.Equal(t, ptrInt32(10), params.ScaleInThreshold)
	require.Equal(t, ptrInt32(90), params.ScaleOutThreshold)
	require.Equal(t, "ghcr.io/example/app:v1", params.Image)
	require.Equal(t, []string{"serve"}, params.Cmd)
	require.Equal(t, ptrString("user"), params.RegistryUsername)
	require.Equal(t, ptrString("secret"), params.RegistryPassword)
	require.Equal(t, v1.RegistryPasswordActionNew, params.RegistryPasswordAction)
	require.Len(t, params.ExposedPorts, 1)
	require.Equal(t, v1.Port(8080), params.ExposedPorts[0].TargetPort)
	require.Equal(t, ptrPort(80), params.ExposedPorts[0].LoadBalancerPort)
	require.True(t, params.ExposedPorts[0].UseLetsEncrypt)
	require.NotNil(t, params.ExposedPorts[0].HealthCheck)
	require.Len(t, params.EnvVars, 2)
	require.Equal(t, ptrString("info"), params.EnvVars[0].Value)
	require.False(t, params.EnvVars[0].Secret)
	require.Equal(t, ptrString("token"), params.EnvVars[1].Value)
	require.True(t, params.EnvVars[1].Secret)
}
