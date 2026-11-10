package eventbus

import (
	"fmt"
	"strings"
)

func ParseConditions(inputs []string) ([]TriggerCondition, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]TriggerCondition, 0, len(inputs))
	for _, input := range inputs {
		var condition TriggerCondition
		for _, element := range strings.Split(input, ",") {
			key, value, ok := strings.Cut(element, "=")
			if !ok || value == "" {
				return nil, fmt.Errorf("--condition は key=...,op=eq|in,value=... の形式です")
			}
			switch key {
			case "key":
				if condition.Key != "" {
					return nil, fmt.Errorf("--condition の key は重複できません")
				}
				condition.Key = value
			case "op":
				if condition.Op != "" {
					return nil, fmt.Errorf("--condition の op は重複できません")
				}
				condition.Op = value
			case "value":
				condition.Values = append(condition.Values, value)
			default:
				return nil, fmt.Errorf("--condition に未知のキーがあります")
			}
		}
		if condition.Key == "" || condition.Op == "" || len(condition.Values) == 0 {
			return nil, fmt.Errorf("--condition は key・op・value が必要です")
		}
		result = append(result, condition)
	}
	return result, nil
}
