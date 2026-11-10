package eventbus

import (
	"context"
	"fmt"
	"strconv"
	"time"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

type ScheduleSettings struct {
	ProcessConfigurationID string
	StartsAt               time.Time
	RecurringStep          *int
	RecurringUnit, Crontab *string
}
type CreateScheduleRequest struct {
	Name, Description string
	Settings          ScheduleSettings
}
type UpdateScheduleRequest struct {
	ID                                        string
	Name, Description, ProcessConfigurationID *string
	StartsAt                                  *time.Time
	RecurringStep                             *int
	RecurringUnit, Crontab                    *string
}

func startsAtMillis(value v1.ScheduleSettingsStartsAt) (int64, error) {
	if ms, ok := value.GetInt64(); ok {
		return ms, nil
	}
	if text, ok := value.GetString(); ok {
		ms, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return ms, nil
		}
	}
	return 0, fmt.Errorf("StartsAt をエポックミリ秒として解釈できません")
}

func scheduleSettings(input ScheduleSettings) (v1.Settings, error) {
	if input.ProcessConfigurationID == "" || input.StartsAt.IsZero() {
		return v1.Settings{}, fmt.Errorf("ProcessConfigurationID と StartsAt が必要です")
	}
	periodic := input.RecurringStep != nil || input.RecurringUnit != nil
	if (input.Crontab != nil) == periodic || periodic && (input.RecurringStep == nil || input.RecurringUnit == nil) {
		return v1.Settings{}, fmt.Errorf("RecurringStep・RecurringUnit の組と Crontab のどちらか一方が必要です")
	}
	settings := v1.ScheduleSettings{ProcessConfigurationID: input.ProcessConfigurationID,
		StartsAt: v1.NewInt64ScheduleSettingsStartsAt(input.StartsAt.UnixMilli())}
	if input.Crontab != nil {
		if *input.Crontab == "" {
			return v1.Settings{}, fmt.Errorf("Crontab は空にできません")
		}
		settings.Crontab.SetTo(*input.Crontab)
	} else {
		switch *input.RecurringUnit {
		case "min", "hour", "day":
		default:
			return v1.Settings{}, fmt.Errorf("RecurringUnit は min、hour、day のいずれかです")
		}
		settings.RecurringStep.SetTo(*input.RecurringStep)
		settings.RecurringUnit.SetTo(v1.ScheduleSettingsRecurringUnit(*input.RecurringUnit))
	}
	return v1.NewScheduleSettingsSettings(settings), nil
}

func (s *Service) ListSchedules(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.schedule.List(ctx)
}
func (s *Service) ReadSchedule(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.schedule, req.ID, v1.ProviderClassEventbusschedule)
}
func (s *Service) CreateSchedule(ctx context.Context, req *CreateScheduleRequest) (*v1.CommonServiceItem, error) {
	settings, err := scheduleSettings(req.Settings)
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
	return s.schedule.Create(ctx, request)
}
func (s *Service) UpdateSchedule(ctx context.Context, req *UpdateScheduleRequest) (*v1.CommonServiceItem, error) {
	if req.Crontab != nil && (req.RecurringStep != nil || req.RecurringUnit != nil) {
		return nil, fmt.Errorf("Crontab と周期指定は併用できません")
	}
	current, err := s.ReadSchedule(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.ProcessConfigurationID != nil || req.StartsAt != nil || req.RecurringStep != nil || req.RecurringUnit != nil || req.Crontab != nil {
		old := current.Settings.ScheduleSettings
		ms, err := startsAtMillis(old.StartsAt)
		if err != nil {
			return nil, err
		}
		input := ScheduleSettings{ProcessConfigurationID: old.ProcessConfigurationID, StartsAt: time.UnixMilli(ms)}
		if old.Crontab.IsSet() {
			value := old.Crontab.Value
			input.Crontab = &value
		}
		if old.RecurringStep.IsSet() {
			value := old.RecurringStep.Value
			input.RecurringStep = &value
		}
		if old.RecurringUnit.IsSet() {
			value := string(old.RecurringUnit.Value)
			input.RecurringUnit = &value
		}
		if req.ProcessConfigurationID != nil {
			input.ProcessConfigurationID = *req.ProcessConfigurationID
		}
		if req.StartsAt != nil {
			input.StartsAt = *req.StartsAt
		}
		if req.Crontab != nil {
			input.Crontab, input.RecurringStep, input.RecurringUnit = req.Crontab, nil, nil
		} else if req.RecurringStep != nil || req.RecurringUnit != nil {
			if input.Crontab != nil {
				input.RecurringStep, input.RecurringUnit = nil, nil
			}
			input.Crontab = nil
			if req.RecurringStep != nil {
				input.RecurringStep = req.RecurringStep
			}
			if req.RecurringUnit != nil {
				input.RecurringUnit = req.RecurringUnit
			}
		}
		settings, err := scheduleSettings(input)
		if err != nil {
			return nil, err
		}
		request.CommonServiceItem.Settings.SetTo(settings)
	}
	return update(ctx, s.schedule, req.ID, request)
}
func (s *Service) DeleteSchedule(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.schedule, req.ID, v1.ProviderClassEventbusschedule)
}
