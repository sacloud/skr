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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/apigen"
	"github.com/sacloud/skr/internal/iaas/internetplanapi"
	"github.com/sacloud/skr/internal/iaas/licenseinfoapi"
	"github.com/sacloud/skr/internal/iaas/privatehostplanapi"
)

func TestIaaSProductPlanGeneratedCodeMatchesConfig(t *testing.T) {
	for _, test := range []struct {
		name   string
		config string
		output string
	}{
		{name: "InternetPlan", config: "iaas-product-internet", output: "internal/iaas/internetplanapi/internetplan_api_generated.go"},
		{name: "LicenseInfo", config: "iaas-product-license", output: "internal/iaas/licenseinfoapi/licenseinfo_api_generated.go"},
		{name: "PrivateHostPlan", config: "iaas-product-privatehost", output: "internal/iaas/privatehostplanapi/privatehostplan_api_generated.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			configData, err := os.ReadFile(repositoryPath("api/commands/" + test.config + ".json"))
			if err != nil {
				t.Fatal(err)
			}
			config, err := apigen.DecodeConfig(configData)
			if err != nil {
				t.Fatal(err)
			}
			want, err := apigen.Generate(config)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(repositoryPath(test.output))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("generated %s commands are stale; regenerate from %s", test.name, test.config)
			}
		})
	}
}

func TestIaaSProductPlanHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "product", "--help"}, want: []string{"disk", "internet", "license", "privatehost", "server"}},
		{args: []string{"iaas-api", "product", "internet", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "product", "internet", "find", "--help"}, want: []string{"--zone", "--count", "--from", "--request", "Filter", "InternetPlans"}},
		{args: []string{"iaas-api", "product", "internet", "read", "--help"}, want: []string{"--zone", "--id", "@path.json"}},
		{args: []string{"iaas-api", "product", "license", "--help"}, want: []string{"find"}},
		{args: []string{"iaas-api", "product", "license", "find", "--help"}, want: []string{"--count", "--from", "--request", "省略時は一覧"}},
		{args: []string{"iaas-api", "product", "privatehost", "--help"}, want: []string{"find", "read"}},
		{args: []string{"iaas-api", "product", "privatehost", "find", "--help"}, want: []string{"--zone", "--request", "CPU", "メモリ"}},
		{args: []string{"iaas-api", "product", "privatehost", "read", "--help"}, want: []string{"--zone", "--id"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%v) = %d; stderr: %s", test.args, code, stderr.String())
			}
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, text := range test.want {
				if !strings.Contains(help, text) {
					t.Errorf("help does not contain %q: %s", text, help)
				}
			}
		})
	}
}

func TestIaaSProductPlanCommandsUseSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	caller := &productPlanAPICaller{}
	commandLine := newCLI()
	setInternetPlanAPIFactory(commandLine, func() (internetplanapi.API, error) {
		return internetplanapi.NewSDKAPI(caller), nil
	})
	setLicenseInfoAPIFactory(commandLine, func() (licenseinfoapi.API, error) {
		return licenseinfoapi.NewSDKAPI(caller), nil
	})
	setPrivateHostPlanAPIFactory(commandLine, func() (privatehostplanapi.API, error) {
		return privatehostplanapi.NewSDKAPI(caller), nil
	})

	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "product"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	output, stderr, code := run("internet", "find", "--zone", "is1b", "--count", "1", "--from", "0")
	if code != 0 || stderr != "" {
		t.Fatalf("InternetPlan Find: code %d, stderr %q", code, stderr)
	}
	var internetFind internetplanapi.FindResult
	if err := json.Unmarshal([]byte(output), &internetFind); err != nil {
		t.Fatal(err)
	}
	if internetFind.Total != 1 || len(internetFind.InternetPlans) != 1 ||
		internetFind.InternetPlans[0].ID != types.ID(11) || internetFind.InternetPlans[0].BandWidthMbps != 100 {
		t.Fatalf("InternetPlan Find result = %+v", internetFind)
	}

	_, stderr, code = run("internet", "find", "--request", `{"Zone":"is1b","Filter":{"Name":"internet"}}`)
	if code != 0 || stderr != "" {
		t.Fatalf("InternetPlan JSON Find: code %d, stderr %q", code, stderr)
	}
	output, stderr, code = run("internet", "read", "--zone", "is1b", "--id", "11")
	if code != 0 || stderr != "" || !strings.Contains(output, `"BandWidthMbps": 100`) {
		t.Fatalf("InternetPlan Read: code %d, stderr %q, output %q", code, stderr, output)
	}

	output, stderr, code = run("license", "find")
	if code != 0 || stderr != "" {
		t.Fatalf("LicenseInfo empty-input Find: code %d, stderr %q", code, stderr)
	}
	var licenseFind licenseinfoapi.FindResult
	if err := json.Unmarshal([]byte(output), &licenseFind); err != nil {
		t.Fatal(err)
	}
	if licenseFind.Total != 1 || len(licenseFind.LicenseInfo) != 1 ||
		licenseFind.LicenseInfo[0].ID != types.ID(42) {
		t.Fatalf("LicenseInfo Find result = %+v", licenseFind)
	}
	_, stderr, code = run("license", "find", "--request", `{"Filter":{"Name":"license"}}`)
	if code != 0 || stderr != "" {
		t.Fatalf("LicenseInfo JSON Find: code %d, stderr %q", code, stderr)
	}
	var licenseCondition struct {
		Filter map[string]json.RawMessage
	}
	if err := json.Unmarshal(caller.calls[len(caller.calls)-1].body, &licenseCondition); err != nil {
		t.Fatal(err)
	}
	if got := string(licenseCondition.Filter["Name"]); got != `"license"` {
		t.Fatalf("LicenseInfo SDK Filter Name = %s, want partial-match string", got)
	}

	output, stderr, code = run("privatehost", "find", "--zone", "is1b", "--count", "1", "--from", "0")
	if code != 0 || stderr != "" {
		t.Fatalf("PrivateHostPlan Find: code %d, stderr %q", code, stderr)
	}
	var privateHostFind privatehostplanapi.FindResult
	if err := json.Unmarshal([]byte(output), &privateHostFind); err != nil {
		t.Fatal(err)
	}
	if privateHostFind.Total != 1 || len(privateHostFind.PrivateHostPlans) != 1 ||
		privateHostFind.PrivateHostPlans[0].ID != types.ID(8) || privateHostFind.PrivateHostPlans[0].MemoryMB != 229376 {
		t.Fatalf("PrivateHostPlan Find result = %+v", privateHostFind)
	}
	output, stderr, code = run("privatehost", "read", "--zone", "is1b", "--id", "8")
	if code != 0 || stderr != "" || !strings.Contains(output, `"Dedicated": true`) {
		t.Fatalf("PrivateHostPlan Read: code %d, stderr %q, output %q", code, stderr, output)
	}

	if got, want := len(caller.calls), 7; got != want {
		t.Fatalf("API calls = %d, want %d: %+v", got, want, caller.calls)
	}
}

func TestIaaSProductPlanRejectsMixedInputsAndPropagatesErrors(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	failure := errors.New("license API unavailable")
	commandLine := newCLI()
	setLicenseInfoAPIFactory(commandLine, func() (licenseinfoapi.API, error) {
		return nil, failure
	})
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "product", "license", "find", "--count", "1", "--request", `{"Count":1}`}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--request と個別フラグは併用できません") {
		t.Fatalf("mixed input: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLI([]string{"iaas-api", "product", "license", "find"}, &stdout, &stderr, commandLine)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), failure.Error()) {
		t.Fatalf("API error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

type productPlanAPICaller struct {
	calls []productPlanAPICall
}

type productPlanAPICall struct {
	method string
	uri    string
	body   []byte
}

func (caller *productPlanAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	caller.calls = append(caller.calls, productPlanAPICall{method: method, uri: uri, body: data})
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/internet"):
		return []byte(`{"Total":1,"From":0,"Count":1,"InternetPlans":[{"ID":"11","Name":"internet-plan","BandWidthMbps":100,"Availability":"available"}]}`), nil
	case strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/internet/11"):
		return []byte(`{"InternetPlan":{"ID":"11","Name":"internet-plan","BandWidthMbps":100,"Availability":"available"}}`), nil
	case strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/license"):
		return []byte(`{"Total":1,"From":0,"Count":1,"LicenseInfo":[{"ID":"42","Name":"license-info","TermsOfUse":"terms"}]}`), nil
	case strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/privatehost"):
		return []byte(`{"Total":1,"From":0,"Count":1,"PrivateHostPlans":[{"ID":"8","Name":"private-host-plan","Class":"standard","CPU":20,"CPUModel":"model","Dedicated":true,"MemoryMB":229376,"Availability":"available"}]}`), nil
	case strings.HasSuffix(parsed.Path, "/api/cloud/1.1/product/privatehost/8"):
		return []byte(`{"PrivateHostPlan":{"ID":"8","Name":"private-host-plan","Class":"standard","CPU":20,"CPUModel":"model","Dedicated":true,"MemoryMB":229376,"Availability":"available"}}`), nil
	default:
		return nil, fmt.Errorf("unexpected API path %s", parsed.Path)
	}
}

var _ iaas.APICaller = (*productPlanAPICaller)(nil)
