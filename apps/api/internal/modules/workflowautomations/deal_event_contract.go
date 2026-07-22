package workflowautomations

import "fmt"

func dealTriggerType(eventType string) (string, error) {
	switch eventType {
	case DealEventCreated:
		return "record_created", nil
	case DealEventStageChanged:
		return "stage_changed", nil
	case DealEventArchived, DealEventOwnerChanged:
		return "record_updated", nil
	default:
		return "", fmt.Errorf("unsupported deal automation event %q", eventType)
	}
}

func dealRuleMatchesEvent(config map[string]any, event DealTaskEvent) bool {
	if event.EventType == DealEventArchived || event.EventType == DealEventOwnerChanged {
		configured, _ := stringConfig(config, "event")
		return configured == event.EventType
	}
	if event.EventType != DealEventStageChanged {
		return true
	}
	value, exists := config["stageId"]
	return !exists || integerConfig(value) == event.StageID
}
