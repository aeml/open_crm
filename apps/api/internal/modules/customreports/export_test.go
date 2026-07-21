package customreports

import "testing"

func TestSpreadsheetSafePreventsFormulaExecutionWithoutChangingOrdinaryValues(t *testing.T) {
	for _, value := range []string{"=SUM(1,1)", "+cmd", "-1+2", "@import", "  =hidden", "\t+hidden"} {
		if got := spreadsheetSafe(value); got != "'"+value {
			t.Fatalf("spreadsheet value %q was not protected: %q", value, got)
		}
	}
	for _, value := range []string{"ordinary value", "", "  ordinary"} {
		if got := spreadsheetSafe(value); got != value {
			t.Fatalf("ordinary spreadsheet value %q changed: %q", value, got)
		}
	}
}
