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

package saclient

import (
	"fmt"
	"net/http"
	"os"
	"runtime"

	sacloudsdk "github.com/sacloud/sacloud-sdk-go"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/sacloud/skr/version"
)

func New(trace bool) (*saclient.Client, error) {
	return newClient(trace, userAgent("skr"))
}

func NewHTTP(trace bool) (*saclient.Client, error) {
	client, err := newClient(trace, "")
	if err != nil {
		return nil, err
	}
	if err := client.SetWith(saclient.WithMiddleware(suppressEmptyUserAgent)); err != nil {
		return nil, err
	}
	return client, nil
}

func HTTPUserAgent() string {
	return userAgent("skr-http")
}

func newClient(trace bool, userAgent string) (*saclient.Client, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	if userAgent != "" {
		if err := client.SetWith(saclient.WithUserAgent(userAgent)); err != nil {
			return nil, err
		}
	}
	if trace {
		if err := client.SetWith(saclient.WithTraceMode("all")); err != nil {
			return nil, err
		}
	}
	return &client, nil
}

func userAgent(product string) string {
	return product + "/v" + version.Version + " (" + runtime.GOOS + "/" + runtime.GOARCH + "; sacloud-sdk-go/v" + sacloudsdk.Version + ")"
}

func suppressEmptyUserAgent(request *http.Request, pull func() (saclient.Middleware, bool)) (*http.Response, error) {
	headerKey := http.CanonicalHeaderKey("User-Agent")
	values, explicitlySet := request.Header[headerKey]
	explicitlyEmpty := explicitlySet
	for _, value := range values {
		if value != "" {
			explicitlyEmpty = false
			break
		}
	}
	if !explicitlyEmpty {
		next, ok := pull()
		if !ok {
			return nil, fmt.Errorf("SDK HTTP middleware chain is empty")
		}
		return next(request, pull)
	}

	setHeader, ok := pull()
	if !ok {
		return nil, fmt.Errorf("SDK HTTP middleware chain is empty")
	}
	pullAfterSetHeader := func() (saclient.Middleware, bool) {
		next, ok := pull()
		if !ok {
			return nil, false
		}
		return func(request *http.Request, pull func() (saclient.Middleware, bool)) (*http.Response, error) {
			request.Header[headerKey] = []string{""}
			return next(request, pull)
		}, true
	}
	return setHeader(request, pullAfterSetHeader)
}
