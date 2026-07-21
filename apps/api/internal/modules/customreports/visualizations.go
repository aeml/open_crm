package customreports

const groupedBarContract = "grouped_bar_v1"

func isExecutableVisualization(definition Definition) bool {
	return definition.VisualizationType == "table" || (definition.VisualizationType == "bar" && definition.VisualizationContract == groupedBarContract)
}

func validateVisualizationInput(input Input) error {
	switch input.VisualizationType {
	case "bar":
		if input.VisualizationContract != groupedBarContract || len(input.Columns) != 0 || input.GroupBy == "" {
			return ErrInvalidInput
		}
		switch input.Aggregation.Function {
		case "count", "sum", "avg":
			return nil
		default:
			return ErrInvalidInput
		}
	default:
		if input.VisualizationContract != "" || len(input.Columns) == 0 {
			return ErrInvalidInput
		}
		return nil
	}
}
