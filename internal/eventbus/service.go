package eventbus

import (
	"context"
	"fmt"

	sdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

type Service struct {
	process  sdk.ProcessConfigurationAPI
	schedule sdk.ScheduleAPI
	trigger  sdk.TriggerAPI
}

func New(caller *v1.Client) *Service {
	return &Service{
		process:  sdk.NewProcessConfigurationOp(caller),
		schedule: sdk.NewScheduleOp(caller),
		trigger:  sdk.NewTriggerOp(caller),
	}
}

type ReadRequest struct{ ID string }
type DeleteRequest struct{ ID string }

type resourceAPI interface {
	List(context.Context) ([]v1.CommonServiceItem, error)
	Read(context.Context, string) (*v1.CommonServiceItem, error)
	Create(context.Context, v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Update(context.Context, string, v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Delete(context.Context, string) error
}

func checkItem(item *v1.CommonServiceItem, id string, class v1.ProviderClass) error {
	if item == nil || item.ID == "" || (id != "" && item.ID != id) || item.Provider.Class != class {
		return fmt.Errorf("応答の ID またはリソース種別が一致しません")
	}
	valid := class == v1.ProviderClassEventbusprocessconfiguration && item.Settings.IsProcessConfigurationSettings() ||
		class == v1.ProviderClassEventbusschedule && item.Settings.IsScheduleSettings() ||
		class == v1.ProviderClassEventbustrigger && item.Settings.IsTriggerSettings()
	if !valid {
		return fmt.Errorf("応答の Settings の種別が一致しません")
	}
	return nil
}

func read(ctx context.Context, api resourceAPI, id string, class v1.ProviderClass) (*v1.CommonServiceItem, error) {
	if id == "" {
		return nil, fmt.Errorf("ID が必要です")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, err := api.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := checkItem(item, id, class); err != nil {
		return nil, err
	}
	return item, nil
}

func remove(ctx context.Context, api resourceAPI, id string, class v1.ProviderClass) error {
	if _, err := read(ctx, api, id, class); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return api.Delete(ctx, id)
}

func createRequest(name, description string, settings v1.Settings) (v1.CreateCommonServiceItemRequest, error) {
	if name == "" {
		return v1.CreateCommonServiceItemRequest{}, fmt.Errorf("Name が必要です")
	}
	desc := v1.NewOptNilString(description)
	if description == "" {
		desc.SetToNull()
	}
	return v1.CreateCommonServiceItemRequest{CommonServiceItem: v1.CreateCommonServiceItemRequestCommonServiceItem{
		Name: name, Description: desc, Settings: settings,
	}}, nil
}

func updateRequest(name, description *string) v1.UpdateCommonServiceItemRequest {
	req := v1.UpdateCommonServiceItemRequest{}
	if name != nil {
		req.CommonServiceItem.Name.SetTo(*name)
	}
	if description != nil {
		req.CommonServiceItem.Description.SetTo(*description)
	}
	return req
}

func update(ctx context.Context, api resourceAPI, id string, req v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return api.Update(ctx, id, req)
}
