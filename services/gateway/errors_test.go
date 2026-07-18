package gateway

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/srav-afk/forge-labs/services/scheduler"
)

func TestSelectErrorStatus(t *testing.T) {
	cases := []struct {
		err            error
		wantStatus     int
		wantType, code string
	}{
		{ErrNoSnapshot, http.StatusServiceUnavailable, "server_error", "no_snapshot"},
		{fmt.Errorf("%w: %q", ErrModelNotFound, "x"), http.StatusNotFound, "invalid_request_error", "model_not_found"},
		{fmt.Errorf("%w: %q", ErrNoLiveAssignee, "x"), http.StatusServiceUnavailable, "server_error", "no_capacity"},
		{scheduler.ErrAdmissionRejected, http.StatusTooManyRequests, "capacity_exceeded", "fleet_saturated"},
		{scheduler.ErrNoCapacity, http.StatusServiceUnavailable, "server_error", "no_capacity"},
	}
	for _, tc := range cases {
		st, typ, code := selectErrorStatus(tc.err)
		if st != tc.wantStatus || typ != tc.wantType || code != tc.code {
			t.Fatalf("%v -> %d %s %s", tc.err, st, typ, code)
		}
	}
}
