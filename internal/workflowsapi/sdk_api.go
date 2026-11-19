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

package workflowsapi

import (
	"github.com/sacloud/sacloud-sdk-go/api/workflows"
	v1 "github.com/sacloud/sacloud-sdk-go/api/workflows/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

func newClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return workflows.NewClient(client)
}

func NewWorkflowAPI(trace bool) (WorkflowAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return workflows.NewWorkflowOp(client), nil
}

func NewRevisionAPI(trace bool) (WorkflowRevisionAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return workflows.NewRevisionOp(client), nil
}

func NewExecutionAPI(trace bool) (WorkflowExecutionAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return workflows.NewExecutionOp(client), nil
}

func NewSubscriptionAPI(trace bool) (WorkflowSubscriptionAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return workflows.NewSubscriptionOp(client), nil
}
