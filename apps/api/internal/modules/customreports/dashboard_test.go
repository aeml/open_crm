package customreports

import (
	"errors"
	"testing"
)

func TestNormalizeDashboardInputDefaultsWidthsAndRejectsUnsafeShapes(t *testing.T) {
	normalized, err := normalizeDashboardInput(DashboardInput{
		Revision: 2,
		Widgets: []DashboardWidgetInput{
			{ReportDefinitionID: 11},
			{ReportDefinitionID: 12, Width: "full"},
		},
	})
	if err != nil {
		t.Fatalf("normalize valid dashboard input: %v", err)
	}
	if normalized.Revision != 2 || len(normalized.Widgets) != 2 || normalized.Widgets[0].Width != "half" || normalized.Widgets[1].Width != "full" {
		t.Fatalf("unexpected normalized dashboard input: %#v", normalized)
	}

	tests := []struct {
		name  string
		input DashboardInput
	}{
		{name: "negative revision", input: DashboardInput{Revision: -1}},
		{name: "missing widget array", input: DashboardInput{}},
		{name: "missing report", input: DashboardInput{Widgets: []DashboardWidgetInput{{Width: "half"}}}},
		{name: "duplicate report", input: DashboardInput{Widgets: []DashboardWidgetInput{{ReportDefinitionID: 1}, {ReportDefinitionID: 1}}}},
		{name: "unsupported width", input: DashboardInput{Widgets: []DashboardWidgetInput{{ReportDefinitionID: 1, Width: "third"}}}},
		{name: "over widget ceiling", input: DashboardInput{Widgets: []DashboardWidgetInput{
			{ReportDefinitionID: 1}, {ReportDefinitionID: 2}, {ReportDefinitionID: 3}, {ReportDefinitionID: 4},
			{ReportDefinitionID: 5}, {ReportDefinitionID: 6}, {ReportDefinitionID: 7},
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeDashboardInput(test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("normalize invalid dashboard input returned %v", err)
			}
		})
	}
}

func TestDashboardMatchesInputRequiresExactOrderAndWidth(t *testing.T) {
	dashboard := Dashboard{Revision: 4, Widgets: []DashboardWidget{
		{ReportDefinitionID: 11, Position: 0, Width: "half"},
		{ReportDefinitionID: 12, Position: 1, Width: "full"},
	}}
	if !dashboardMatchesInput(dashboard, DashboardInput{Revision: 4, Widgets: []DashboardWidgetInput{
		{ReportDefinitionID: 11, Width: "half"},
		{ReportDefinitionID: 12, Width: "full"},
	}}) {
		t.Fatal("exact dashboard input did not match")
	}
	for _, input := range []DashboardInput{
		{Revision: 4, Widgets: []DashboardWidgetInput{{ReportDefinitionID: 12, Width: "full"}, {ReportDefinitionID: 11, Width: "half"}}},
		{Revision: 4, Widgets: []DashboardWidgetInput{{ReportDefinitionID: 11, Width: "full"}, {ReportDefinitionID: 12, Width: "full"}}},
		{Revision: 4, Widgets: []DashboardWidgetInput{{ReportDefinitionID: 11, Width: "half"}}},
	} {
		if dashboardMatchesInput(dashboard, input) {
			t.Fatalf("non-identical dashboard input matched: %#v", input)
		}
	}
}
