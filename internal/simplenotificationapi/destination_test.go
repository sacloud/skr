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

package simplenotificationapi

import (
	"context"
	"errors"
	"testing"

	simplenotification "github.com/sacloud/sacloud-sdk-go/api/simple-notification"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestDestinationAdapterProtectsAllSDKErrorsAndPreservesClassification(t *testing.T) {
	opErr := errors.New("saknoticedestination op failed")
	responses := map[string]struct {
		response any
		err      error
	}{
		"List":       {response: &v1.ListCommonServiceItemsResponse{}, err: opErr},
		"Read":       {response: &v1.ReadCommonServiceItemOK{}, err: opErr},
		"Create":     {response: &v1.CreateCommonServiceItemCreated{}, err: opErr},
		"Update":     {response: &v1.UpdateCommonServiceItemOK{}, err: opErr},
		"ReadStatus": {response: &v1.CheckDestinationStatusResponse{}, err: opErr},
	}
	for method, result := range responses {
		t.Run(method, func(t *testing.T) {
			adapter := destinationAdapter{newDestinationOpStub(t, method, result.response, result.err)}
			require.Error(t, callDestinationAdapterMethod(adapter, method), "%s should return an error", method)
		})
	}
	t.Run("Delete", func(t *testing.T) {
		adapter := destinationAdapter{newDestinationOpStub(t, "Delete", nil, opErr)}
		require.Error(t, adapter.Delete(context.Background(), "id"))
	})
	t.Run("success responses keep classification", func(t *testing.T) {
		adapter := destinationAdapter{newDestinationOpStub(t, "Read", &v1.ReadCommonServiceItemOK{}, nil)}
		result, err := adapter.Read(context.Background(), "id")
		require.NoError(t, err)
		require.IsType(t, &v1.ReadCommonServiceItemOK{}, result)
	})
	t.Run("error messages do not include request values", func(t *testing.T) {
		adapter := destinationAdapter{newDestinationOpStub(t, "Create", nil, opErr)}
		_, err := adapter.Create(context.Background(), v1.PostCommonServiceItemRequest{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "通知先の作成に失敗しました")
	})
}

func callDestinationAdapterMethod(adapter destinationAdapter, method string) error {
	_, err := invokeDestinationAdapter(adapter, method)
	return err
}

func invokeDestinationAdapter(adapter destinationAdapter, method string) (any, error) {
	ctx := context.Background()
	switch method {
	case "List":
		result, err := adapter.List(ctx)
		return result, err
	case "Read":
		result, err := adapter.Read(ctx, "id")
		return result, err
	case "Create":
		result, err := adapter.Create(ctx, v1.PostCommonServiceItemRequest{})
		return result, err
	case "Update":
		result, err := adapter.Update(ctx, "id", v1.PutCommonServiceItemRequest{})
		return result, err
	case "ReadStatus":
		result, err := adapter.ReadStatus(ctx, "id")
		return result, err
	default:
		return nil, errors.New("unknown method: " + method)
	}
}

func newDestinationOpStub(t *testing.T, method string, response any, err error) simplenotification.DestinationAPI {
	t.Helper()
	op := &destinationOpStub{err: err}
	switch method {
	case "List":
		op.list = func() (any, error) { return response, err }
	case "Read":
		op.read = func() (any, error) { return response, err }
	case "Create":
		op.create = func() (any, error) { return response, err }
	case "Update":
		op.update = func() (any, error) { return response, err }
	case "ReadStatus":
		op.readStatus = func() (any, error) { return response, err }
	}
	return op
}

// destinationOpStub は DestinationAPI の単一メソッドだけを置き換えるスタブです。
type destinationOpStub struct {
	simplenotification.DestinationAPI
	list       func() (any, error)
	read       func() (any, error)
	create     func() (any, error)
	update     func() (any, error)
	readStatus func() (any, error)
	err        error
}

func (s *destinationOpStub) Delete(ctx context.Context, id string) error {
	return s.err
}

func (s *destinationOpStub) List(ctx context.Context) (*v1.ListCommonServiceItemsResponse, error) {
	if s.list == nil {
		return nil, s.err
	}
	result, err := s.list()
	if result == nil {
		return nil, err
	}
	return result.(*v1.ListCommonServiceItemsResponse), err
}

func (s *destinationOpStub) Read(ctx context.Context, id string) (*v1.ReadCommonServiceItemOK, error) {
	if s.read == nil {
		return nil, s.err
	}
	result, err := s.read()
	if result == nil {
		return nil, err
	}
	return result.(*v1.ReadCommonServiceItemOK), err
}

func (s *destinationOpStub) Create(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
	if s.create == nil {
		return nil, s.err
	}
	result, err := s.create()
	if result == nil {
		return nil, err
	}
	return result.(*v1.CreateCommonServiceItemCreated), err
}

func (s *destinationOpStub) Update(ctx context.Context, id string, request v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error) {
	if s.update == nil {
		return nil, s.err
	}
	result, err := s.update()
	if result == nil {
		return nil, err
	}
	return result.(*v1.UpdateCommonServiceItemOK), err
}

func (s *destinationOpStub) ReadStatus(ctx context.Context, id string) (*v1.CheckDestinationStatusResponse, error) {
	if s.readStatus == nil {
		return nil, s.err
	}
	result, err := s.readStatus()
	if result == nil {
		return nil, err
	}
	return result.(*v1.CheckDestinationStatusResponse), err
}
