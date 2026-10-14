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
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	"github.com/sacloud/skr/internal/iaas/packetfilterapi"
)

func transformPacketFilterCreateRequest(command *packetfilterapi.CreateCommand, request *packetfilter.CreateRequest) error {
	expressions, err := decodePacketFilterExpressions(command.Expression)
	if err != nil {
		return err
	}
	if expressions != nil {
		request.Expression = expressions
	}
	return nil
}

func transformPacketFilterUpdateRequest(command *packetfilterapi.UpdateCommand, request *packetfilter.UpdateRequest) error {
	expressions, err := decodePacketFilterExpressions(command.Expression)
	if err != nil {
		return err
	}
	if expressions != nil {
		request.Expression = &expressions
	}
	return nil
}

func decodePacketFilterExpressions(inputs []string) ([]*iaas.PacketFilterExpression, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) != 1 {
		return nil, fmt.Errorf("--expressions は1回だけ指定できます")
	}
	var expressions []*iaas.PacketFilterExpression
	if err := decodeRequest(inputs[0], &expressions); err != nil {
		return nil, fmt.Errorf("--expressions: %w", err)
	}
	if expressions == nil {
		return nil, fmt.Errorf("--expressions は JSON 配列で指定してください")
	}
	return expressions, nil
}
