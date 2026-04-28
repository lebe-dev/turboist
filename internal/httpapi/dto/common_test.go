package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func TestParsePageParams_Defaults(t *testing.T) {
	p := dto.ParsePageParams("", "")
	if p.Limit != dto.DefaultLimit {
		t.Errorf("limit = %d; want %d", p.Limit, dto.DefaultLimit)
	}
	if p.Offset != 0 {
		t.Errorf("offset = %d; want 0", p.Offset)
	}
}

func TestParsePageParams_Clamp(t *testing.T) {
	cases := []struct {
		limitStr  string
		wantLimit int
	}{
		{"0", dto.DefaultLimit},
		{"-5", dto.DefaultLimit},
		{"1", 1},
		{"100", 100},
		{"200", 200},
		{"201", 200},
		{"9999", 200},
	}
	for _, tc := range cases {
		p := dto.ParsePageParams(tc.limitStr, "0")
		if p.Limit != tc.wantLimit {
			t.Errorf("limit(%q) = %d; want %d", tc.limitStr, p.Limit, tc.wantLimit)
		}
	}
}

func TestParsePageParams_NegativeOffset(t *testing.T) {
	p := dto.ParsePageParams("50", "-10")
	if p.Offset != 0 {
		t.Errorf("offset = %d; want 0 for negative input", p.Offset)
	}
}

func TestNewPagedResponse_NilItems(t *testing.T) {
	pr := dto.NewPagedResponse[int](nil, 0, 50, 0)
	if pr.Items == nil {
		t.Error("items must not be nil")
	}
	if len(pr.Items) != 0 {
		t.Error("items must be empty slice")
	}
}

func TestOptional_Absent(t *testing.T) {
	type req struct {
		Name dto.Optional[string] `json:"name"`
	}
	var r req
	if err := json.Unmarshal([]byte(`{}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Name.IsAbsent() {
		t.Error("want absent")
	}
}

func TestOptional_Null(t *testing.T) {
	type req struct {
		Name dto.Optional[string] `json:"name"`
	}
	var r req
	if err := json.Unmarshal([]byte(`{"name":null}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Name.IsNull() {
		t.Error("want null")
	}
}

func TestOptional_Set(t *testing.T) {
	type req struct {
		Name dto.Optional[string] `json:"name"`
	}
	var r req
	if err := json.Unmarshal([]byte(`{"name":"hello"}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Name.IsSet() {
		t.Error("want set")
	}
	v, _ := r.Name.Value()
	if v != "hello" {
		t.Errorf("value = %q; want hello", v)
	}
}

func TestFormatTime(t *testing.T) {
	fixed := time.Date(2026, 4, 27, 14, 30, 0, 500_000_000, time.UTC)
	got := dto.FormatTime(fixed)
	want := "2026-04-27T14:30:00.500Z"
	if got != want {
		t.Errorf("FormatTime = %q; want %q", got, want)
	}
}

func TestFormatTimePtr_Nil(t *testing.T) {
	if dto.FormatTimePtr(nil) != nil {
		t.Error("nil input should return nil pointer")
	}
}

func TestFormatTimePtr_NonNil(t *testing.T) {
	fixed := time.Date(2026, 4, 27, 14, 30, 0, 0, time.UTC)
	got := dto.FormatTimePtr(&fixed)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if *got != "2026-04-27T14:30:00.000Z" {
		t.Errorf("FormatTimePtr = %q", *got)
	}
}
