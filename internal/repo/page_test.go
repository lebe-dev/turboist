package repo

import "testing"

func TestPage_Normalize(t *testing.T) {
	cases := []struct {
		name string
		in   Page
		want Page
	}{
		{"zero_limit_defaults_to_50", Page{Limit: 0, Offset: 0}, Page{Limit: 50, Offset: 0}},
		{"negative_limit_defaults_to_50", Page{Limit: -5, Offset: 0}, Page{Limit: 50, Offset: 0}},
		{"in_range_unchanged", Page{Limit: 10, Offset: 5}, Page{Limit: 10, Offset: 5}},
		{"limit_clamped_to_200", Page{Limit: 500, Offset: 0}, Page{Limit: 200, Offset: 0}},
		{"negative_offset_clamped_to_zero", Page{Limit: 50, Offset: -1}, Page{Limit: 50, Offset: 0}},
		{"max_inclusive_200_unchanged", Page{Limit: 200, Offset: 0}, Page{Limit: 200, Offset: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Normalize()
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
