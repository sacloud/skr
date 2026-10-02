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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

type eventbusAPICommand struct {
	ProcessConfiguration processConfigurationCommands `cmd:"" name:"process-configuration" help:"Manage EventBus process configurations."`
	Schedule             resourceCommands             `cmd:"" help:"Manage EventBus schedules."`
	Trigger              resourceCommands             `cmd:"" help:"Manage EventBus triggers."`
}

type itemAPI interface {
	List(context.Context) ([]v1.CommonServiceItem, error)
	Read(context.Context, string) (*v1.CommonServiceItem, error)
	Create(context.Context, v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Update(context.Context, string, v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Delete(context.Context, string) error
}

type processConfigurationAPI interface {
	itemAPI
	UpdateSecret(context.Context, string, v1.SetSecretRequest) error
}

type itemAPIFactory func() (itemAPI, error)

type resourceCommands struct {
	List   itemListCommand   `cmd:"" help:"List this resource type."`
	Read   itemReadCommand   `cmd:"" help:"Read a resource by ID."`
	Create itemCreateCommand `cmd:"" help:"Create a resource from a JSON request."`
	Update itemUpdateCommand `cmd:"" help:"Update a resource from a JSON request."`
	Delete itemDeleteCommand `cmd:"" help:"Delete a resource by ID."`
}

type processConfigurationCommands struct {
	List         itemListCommand         `cmd:"" help:"List process configurations."`
	Read         itemReadCommand         `cmd:"" help:"Read a process configuration by ID."`
	Create       itemCreateCommand       `cmd:"" help:"Create a process configuration from a JSON request."`
	Update       itemUpdateCommand       `cmd:"" help:"Update a process configuration from a JSON request."`
	Delete       itemDeleteCommand       `cmd:"" help:"Delete a process configuration by ID."`
	UpdateSecret itemUpdateSecretCommand `cmd:"" help:"Update a process configuration's secret."`
}

type itemListCommand struct {
	factory itemAPIFactory
}

func (c *itemListCommand) Run(ctx *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	items, err := op.List(context.Background())
	if err != nil {
		return err
	}
	return writeJSON(ctx, items)
}

type itemReadCommand struct {
	ID      string `arg:"" help:"Resource ID."`
	factory itemAPIFactory
}

func (c *itemReadCommand) Run(ctx *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Read(context.Background(), c.ID)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type itemCreateCommand struct {
	Request       string `name:"request" required:"" help:"SDK CreateCommonServiceItemRequest JSON or @path-to-file."`
	factory       itemAPIFactory
	providerClass v1.ProviderClass
}

func (c *itemCreateCommand) Run(ctx *kong.Context) error {
	var request v1.CreateCommonServiceItemRequest
	if err := decodeCreateRequest(c.Request, c.providerClass, &request); err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type itemUpdateCommand struct {
	ID      string `arg:"" help:"Resource ID."`
	Request string `name:"request" required:"" help:"SDK UpdateCommonServiceItemRequest JSON or @path-to-file."`
	factory itemAPIFactory
}

func (c *itemUpdateCommand) Run(ctx *kong.Context) error {
	var request v1.UpdateCommonServiceItemRequest
	if err := decodeRequest(c.Request, &request); err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.Update(context.Background(), c.ID, request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type itemDeleteCommand struct {
	ID      string `arg:"" help:"Resource ID."`
	factory itemAPIFactory
}

func (c *itemDeleteCommand) Run(_ *kong.Context) error {
	op, err := c.factory()
	if err != nil {
		return err
	}
	return op.Delete(context.Background(), c.ID)
}

type itemUpdateSecretCommand struct {
	ID         string `arg:"" help:"Process configuration ID."`
	SecretFile string `name:"secret-file" required:"" help:"SDK SetSecretRequest JSON file, or - to read from standard input."`
}

func (c *itemUpdateSecretCommand) Run(_ *kong.Context) error {
	var secret v1.SetSecretRequestSecret
	data, err := readSecretFile(c.SecretFile)
	if err != nil {
		return err
	}
	if err := unmarshalRequest(data, &secret); err != nil {
		return err
	}
	op, err := newProcessConfigurationAPI()
	if err != nil {
		return err
	}
	request := v1.SetSecretRequest{Secret: secret}
	return op.UpdateSecret(context.Background(), c.ID, request)
}

func newCLI() cli {
	result := cli{}
	processConfigurationFactory := func() (itemAPI, error) {
		return newProcessConfigurationAPI()
	}
	result.EventbusAPI.ProcessConfiguration.setFactory(
		processConfigurationFactory,
		v1.ProviderClassEventbusprocessconfiguration,
	)
	result.EventbusAPI.Schedule.setFactory(func() (itemAPI, error) {
		client, err := newEventbusClient()
		if err != nil {
			return nil, err
		}
		return eventbus.NewScheduleOp(client), nil
	}, v1.ProviderClassEventbusschedule)
	result.EventbusAPI.Trigger.setFactory(func() (itemAPI, error) {
		client, err := newEventbusClient()
		if err != nil {
			return nil, err
		}
		return eventbus.NewTriggerOp(client), nil
	}, v1.ProviderClassEventbustrigger)
	return result
}

func (c *resourceCommands) setFactory(factory itemAPIFactory, providerClass v1.ProviderClass) {
	c.List.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Create.providerClass = providerClass
	c.Update.factory = factory
	c.Delete.factory = factory
}

func (c *processConfigurationCommands) setFactory(factory itemAPIFactory, providerClass v1.ProviderClass) {
	c.List.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Create.providerClass = providerClass
	c.Update.factory = factory
	c.Delete.factory = factory
}

func newProcessConfigurationAPI() (processConfigurationAPI, error) {
	client, err := newEventbusClient()
	if err != nil {
		return nil, err
	}
	return eventbus.NewProcessConfigurationOp(client), nil
}

func newEventbusClient() (*v1.Client, error) {
	var client saclient.Client
	if err := client.SetEnviron(os.Environ()); err != nil {
		return nil, err
	}
	return eventbus.NewClient(&client)
}

func decodeRequest(input string, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}
	return unmarshalRequest(data, destination)
}

func decodeCreateRequest(input string, providerClass v1.ProviderClass, destination any) error {
	data, err := requestData(input)
	if err != nil {
		return err
	}

	var request struct {
		CommonServiceItem json.RawMessage `json:"CommonServiceItem"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(request.CommonServiceItem, &item); err != nil {
		return fmt.Errorf("decode CommonServiceItem: %w", err)
	}
	if item == nil {
		return fmt.Errorf("decode CommonServiceItem: expected JSON object")
	}
	provider, err := json.Marshal(struct {
		Class v1.ProviderClass `json:"Class"`
	}{Class: providerClass})
	if err != nil {
		return fmt.Errorf("encode EventBus provider: %w", err)
	}
	item["Provider"] = provider
	request.CommonServiceItem, err = json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode CommonServiceItem: %w", err)
	}
	data, err = json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode create request: %w", err)
	}
	return unmarshalRequest(data, destination)
}

func requestData(input string) ([]byte, error) {
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // Path is explicitly provided by the user via --secret-file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}

func unmarshalRequest(data []byte, destination any) error {
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func writeJSON(ctx *kong.Context, value any) error {
	encoder := json.NewEncoder(ctx.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
