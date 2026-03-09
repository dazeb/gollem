package codetool

import (
	"encoding/json"
	"testing"
)

func TestBashParamsRiskFieldsDeserialize(t *testing.T) {
	var params BashParams
	if err := json.Unmarshal([]byte(`{"command":"git status","risk_level":"low","risk_reason":"read-only"}`), &params); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if params.RiskLevel != "low" || params.RiskReason != "read-only" {
		t.Fatalf("params=%+v", params)
	}
}

func TestBashParamsWithoutRiskFieldsDeserialize(t *testing.T) {
	var params BashParams
	if err := json.Unmarshal([]byte(`{"command":"git status"}`), &params); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if params.RiskLevel != "" || params.RiskReason != "" {
		t.Fatalf("params=%+v", params)
	}
}
