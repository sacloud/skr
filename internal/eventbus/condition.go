package eventbus

import (
	"fmt"
	"strings"

	eventbusapi "github.com/sacloud/skr/service/eventbus-service-go"
)

// ParseConditions は反復可能な --condition の値をライブラリの TriggerCondition へ変換します。
// 各値は 1 条件を表し、"key=名前,op=eq|in,value=値" の形式でカンマ区切りの要素に分解
// して解釈します。value= は反復でき、op=in の複数値を表します。要素の値に "=" を含む
// 場合は最初の "=" の後ろすべてを値として保持します。値の前後の空白は補正せず、
// key・op の重複、未知の要素、key・op・value の欠落はエラーになります。
func ParseConditions(inputs []string) ([]eventbusapi.TriggerCondition, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]eventbusapi.TriggerCondition, 0, len(inputs))
	for _, input := range inputs {
		var condition eventbusapi.TriggerCondition
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
