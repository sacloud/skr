package eventbus

import (
	"context"
	"fmt"
	"regexp"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

type TriggerCondition struct {
	Key    string
	Op     string
	Values []string
}
type TriggerSettings struct {
	Source                 string
	Types                  []string
	Conditions             []TriggerCondition
	ProcessConfigurationID string
}
type CreateTriggerRequest struct {
	Name, Description string
	Settings          TriggerSettings
}
type UpdateTriggerRequest struct {
	ID                                                string
	Name, Description, Source, ProcessConfigurationID *string
	Types                                             []string
	Conditions                                        []TriggerCondition
}

var conditionKey = regexp.MustCompile(`^[a-z0-9]{1,20}$`)

func validateSource(source string) error {
	if source != "//eventbus.sakura.ad.jp/eventlog" && source != "//eventbus.sakura.ad.jp/monitoringsuite/alert" {
		return fmt.Errorf("Source は //eventbus.sakura.ad.jp/eventlog または //eventbus.sakura.ad.jp/monitoringsuite/alert を指定してください")
	}
	return nil
}

func validateTypes(types []string) error {
	if types != nil && len(types) == 0 {
		return fmt.Errorf("Types の消去は低レベル API を使ってください")
	}
	for _, value := range types {
		if value == "" {
			return fmt.Errorf("Types に空文字列は指定できません")
		}
	}
	return nil
}

func convertConditions(conditions []TriggerCondition) (v1.OptNilTriggerSettingsConditionsItemArray, error) {
	result := v1.OptNilTriggerSettingsConditionsItemArray{}
	if conditions == nil {
		result.SetToNull()
		return result, nil
	}
	if len(conditions) == 0 {
		return result, fmt.Errorf("Conditions の消去は低レベル API を使ってください")
	}
	values := make([]v1.TriggerSettingsConditionsItem, 0, len(conditions))
	for _, condition := range conditions {
		if !conditionKey.MatchString(condition.Key) || condition.Key == "data" {
			return result, fmt.Errorf("Conditions.Key は data 以外の小文字英数字で指定してください")
		}
		switch condition.Op {
		case "eq":
			if len(condition.Values) != 1 {
				return result, fmt.Errorf("Conditions の eq は Values が1件必要です")
			}
			values = append(values, v1.NewTriggerConditionEqTriggerSettingsConditionsItem(v1.TriggerConditionEq{
				Key: condition.Key, Op: v1.TriggerConditionEqOpEq, Values: condition.Values,
			}))
		case "in":
			if len(condition.Values) == 0 {
				return result, fmt.Errorf("Conditions の in は Values が必要です")
			}
			values = append(values, v1.NewTriggerConditionInTriggerSettingsConditionsItem(v1.TriggerConditionIn{
				Key: condition.Key, Op: v1.TriggerConditionInOpIn, Values: condition.Values,
			}))
		default:
			return result, fmt.Errorf("Conditions.Op は eq または in を指定してください")
		}
	}
	result.SetTo(values)
	return result, nil
}

func (s *Service) ListTriggers(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.trigger.List(ctx)
}
func (s *Service) ReadTrigger(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.trigger, req.ID, v1.ProviderClassEventbustrigger)
}
func (s *Service) CreateTrigger(ctx context.Context, req *CreateTriggerRequest) (*v1.CommonServiceItem, error) {
	if err := validateSource(req.Settings.Source); err != nil {
		return nil, err
	}
	if req.Settings.ProcessConfigurationID == "" {
		return nil, fmt.Errorf("ProcessConfigurationID が必要です")
	}
	if err := validateTypes(req.Settings.Types); err != nil {
		return nil, err
	}
	conditions, err := convertConditions(req.Settings.Conditions)
	if err != nil {
		return nil, err
	}
	types := v1.NewOptNilStringArray(req.Settings.Types)
	if req.Settings.Types == nil {
		types.SetToNull()
	}
	settings := v1.NewTriggerSettingsSettings(v1.TriggerSettings{
		Source: req.Settings.Source, Types: types, Conditions: conditions, ProcessConfigurationID: req.Settings.ProcessConfigurationID,
	})
	request, err := createRequest(req.Name, req.Description, settings)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.trigger.Create(ctx, request)
}
func (s *Service) UpdateTrigger(ctx context.Context, req *UpdateTriggerRequest) (*v1.CommonServiceItem, error) {
	if req.Source != nil {
		if err := validateSource(*req.Source); err != nil {
			return nil, err
		}
	}
	if req.ProcessConfigurationID != nil && *req.ProcessConfigurationID == "" {
		return nil, fmt.Errorf("ProcessConfigurationID が必要です")
	}
	if err := validateTypes(req.Types); err != nil {
		return nil, err
	}
	conditions, err := convertConditions(req.Conditions)
	if err != nil {
		return nil, err
	}
	current, err := s.ReadTrigger(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.Source != nil || req.ProcessConfigurationID != nil || req.Types != nil || req.Conditions != nil {
		settings := current.Settings.TriggerSettings
		if req.Source != nil {
			settings.Source = *req.Source
		}
		if req.ProcessConfigurationID != nil {
			settings.ProcessConfigurationID = *req.ProcessConfigurationID
		}
		if req.Types != nil {
			settings.Types.SetTo(req.Types)
		}
		if req.Conditions != nil {
			settings.Conditions = conditions
		}
		request.CommonServiceItem.Settings.SetTo(v1.NewTriggerSettingsSettings(settings))
	}
	return update(ctx, s.trigger, req.ID, request)
}
func (s *Service) DeleteTrigger(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.trigger, req.ID, v1.ProviderClassEventbustrigger)
}
