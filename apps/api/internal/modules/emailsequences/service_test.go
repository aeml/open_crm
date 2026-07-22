package emailsequences

import (
	"errors"
	"strings"
	"testing"
)

func TestSequenceDefinitionInputBounds(t *testing.T) {
	valid := Input{
		Name:        strings.Repeat("n", MaxSequenceNameLength),
		Description: strings.Repeat("d", MaxSequenceDescription),
		Status:      "draft",
		Steps: []StepInput{{
			DelayDays: MaxSequenceStepDelayDays,
			Subject:   strings.Repeat("s", MaxSequenceSubjectLength),
			Body:      strings.Repeat("b", MaxSequenceBodyLength),
		}},
	}
	if err := validateInput(normalizeInput(valid)); err != nil {
		t.Fatalf("maximum valid email sequence definition was rejected: %v", err)
	}

	tests := []struct {
		name  string
		input Input
	}{
		{name: "name", input: withDefinitionName(valid, strings.Repeat("n", MaxSequenceNameLength+1))},
		{name: "description", input: withDefinitionDescription(valid, strings.Repeat("d", MaxSequenceDescription+1))},
		{name: "steps", input: withDefinitionSteps(valid, make([]StepInput, MaxSequenceSteps+1))},
		{name: "delay", input: withDefinitionSteps(valid, []StepInput{{DelayDays: MaxSequenceStepDelayDays + 1, Subject: "Subject", Body: "Body"}})},
		{name: "subject", input: withDefinitionSteps(valid, []StepInput{{Subject: strings.Repeat("s", MaxSequenceSubjectLength+1), Body: "Body"}})},
		{name: "body", input: withDefinitionSteps(valid, []StepInput{{Subject: "Subject", Body: strings.Repeat("b", MaxSequenceBodyLength+1)}})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateInput(normalizeInput(test.input)); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("email sequence %s bound returned %v", test.name, err)
			}
		})
	}
}

func withDefinitionName(input Input, name string) Input {
	input.Name = name
	return input
}

func withDefinitionDescription(input Input, description string) Input {
	input.Description = description
	return input
}

func withDefinitionSteps(input Input, steps []StepInput) Input {
	input.Steps = steps
	return input
}
