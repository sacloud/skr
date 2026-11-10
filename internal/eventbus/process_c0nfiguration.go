package eventbus

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

type ProcessConfigurationSettings struct {
	Destination                                              string
	GroupID, Message, QueueName, Content, Action, ResourceID *string
}

type CreateProcessConfigurationRequest struct {
	Name, Description string
	Settings          ProcessConfigurationSettings
}

type UpdateProcessConfigurationRequest struct {
	ID                                                       string
	Name, Description, Destination                           *string
	GroupID, Message, QueueName, Content, Action, ResourceID *string
}

type SetProcessConfigurationSecretRequest struct {
	ID     string
	Secret v1.SetSecretRequestSecret
}

func parameterKeys(destination string) ([]string, error) {
	switch destination {
	case "simplenotification":
		return []string{"group_id", "message"}, nil
	case "simplemq":
		return []string{"queue_name", "content"}, nil
	case "autoscale":
		return []string{"action", "resource_id"}, nil
	default:
		return nil, fmt.Errorf("Destination は simplenotification、simplemq、autoscale のいずれかです")
	}
}

func parameters(input ProcessConfigurationSettings, existing string) (v1.Settings, error) {
	keys, err := parameterKeys(input.Destination)
	if err != nil {
		return v1.Settings{}, err
	}
	fields := map[string]*string{
		"group_id": input.GroupID, "message": input.Message,
		"queue_name": input.QueueName, "content": input.Content,
		"action": input.Action, "resource_id": input.ResourceID,
	}
	for key, value := range fields {
		if value != nil && key != keys[0] && key != keys[1] {
			return v1.Settings{}, fmt.Errorf("Destination と %s の組み合わせが不正です", key)
		}
	}
	values := map[string]json.RawMessage{}
	if existing != "" {
		if err := json.Unmarshal([]byte(existing), &values); err != nil || values == nil {
			return v1.Settings{}, fmt.Errorf("既存 Parameters が JSON オブジェクトではありません")
		}
	}
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || string(raw) == "null" {
				return v1.Settings{}, fmt.Errorf("Parameters の %s は文字列である必要があります", key)
			}
		}
		if fields[key] != nil {
			values[key], err = json.Marshal(*fields[key])
		} else if _, ok := values[key]; !ok {
			values[key] = json.RawMessage(`""`)
		}
		if err != nil {
			return v1.Settings{}, err
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return v1.Settings{}, fmt.Errorf("Parameters を変換できません: %w", err)
	}
	return v1.NewProcessConfigurationSettingsSettings(v1.ProcessConfigurationSettings{
		Destination: v1.ProcessConfigurationSettingsDestination(input.Destination), Parameters: string(data),
	}), nil
}

func (s *Service) ListProcessConfigurations(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.process.List(ctx)
}
func (s *Service) ReadProcessConfiguration(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.process, req.ID, v1.ProviderClassEventbusprocessconfiguration)
}
func (s *Service) CreateProcessConfiguration(ctx context.Context, req *CreateProcessConfigurationRequest) (*v1.CommonServiceItem, error) {
	settings, err := parameters(req.Settings, "")
	if err != nil {
		return nil, err
	}
	request, err := createRequest(req.Name, req.Description, settings)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.process.Create(ctx, request)
}
func (s *Service) UpdateProcessConfiguration(ctx context.Context, req *UpdateProcessConfigurationRequest) (*v1.CommonServiceItem, error) {
	current, err := s.ReadProcessConfiguration(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.Destination != nil || req.GroupID != nil || req.Message != nil || req.QueueName != nil || req.Content != nil || req.Action != nil || req.ResourceID != nil {
		old := current.Settings.ProcessConfigurationSettings
		destination := string(old.Destination)
		existing := old.Parameters
		if req.Destination != nil && *req.Destination != destination {
			destination, existing = *req.Destination, ""
		}
		// 既存の空文字列を、新規設定の空オブジェクトとして扱いません。
		if existing == "" && (req.Destination == nil || *req.Destination == string(old.Destination)) {
			return nil, fmt.Errorf("既存 Parameters が JSON オブジェクトではありません")
		}
		settings, err := parameters(ProcessConfigurationSettings{Destination: destination,
			GroupID: req.GroupID, Message: req.Message, QueueName: req.QueueName,
			Content: req.Content, Action: req.Action, ResourceID: req.ResourceID}, existing)
		if err != nil {
			return nil, err
		}
		request.CommonServiceItem.Settings.SetTo(settings)
	}
	return update(ctx, s.process, req.ID, request)
}
func (s *Service) DeleteProcessConfiguration(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.process, req.ID, v1.ProviderClassEventbusprocessconfiguration)
}
func (s *Service) SetProcessConfigurationSecret(ctx context.Context, req *SetProcessConfigurationSecretRequest) error {
	if _, err := s.ReadProcessConfiguration(ctx, &ReadRequest{ID: req.ID}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.process.UpdateSecret(ctx, req.ID, v1.SetSecretRequest{Secret: req.Secret})
}
