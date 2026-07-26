package resource_test

import (
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// Each strategy builds a different payload from the same schema. A field sent
// under the wrong strategy is either ignored — a permanent diff — or rejected.

func intSet(values ...int64) tftypes.Value {
	elements := make([]tftypes.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, tftypes.NewValue(tftypes.Number, value))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.Number}, elements)
}

// One create per strategy, checking the payload the client receives.
func TestCreateSendsTheFieldsEachStrategyUses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		plan map[string]tftypes.Value
		want func(*testing.T, kuma.Maintenance)
	}{
		{
			name: "recurring by day of month",
			plan: map[string]tftypes.Value{
				"title":            str("monthly"),
				"strategy":         str("recurring-day-of-month"),
				"days_of_month":    intSet(1, 15, 28),
				"start_time":       str("02:00"),
				"end_time":         str("04:00"),
				"duration_minutes": num(120),
				"timezone":         str("UTC"),
			},
			want: func(t *testing.T, sent kuma.Maintenance) {
				if len(sent.DaysOfMonth) != 3 {
					t.Errorf("days of month = %v, want three entries", sent.DaysOfMonth)
				}
				if sent.DurationMinutes == nil {
					t.Fatal("duration was not sent")
				}
				if *sent.DurationMinutes != 120 {
					t.Errorf("duration = %d, want 120", *sent.DurationMinutes)
				}
			},
		},
		{
			name: "recurring by weekday",
			plan: map[string]tftypes.Value{
				"title":      str("weekly"),
				"strategy":   str("recurring-weekday"),
				"weekdays":   intSet(1, 3, 5),
				"start_time": str("01:00"),
				"end_time":   str("02:00"),
				"timezone":   str("UTC"),
			},
			want: func(t *testing.T, sent kuma.Maintenance) {
				if len(sent.Weekdays) != 3 {
					t.Errorf("weekdays = %v, want three entries", sent.Weekdays)
				}
				if len(sent.DaysOfMonth) != 0 {
					t.Errorf("days of month should be absent for a weekly window, got %v",
						sent.DaysOfMonth)
				}
			},
		},
		{
			name: "a single window",
			plan: map[string]tftypes.Value{
				"title":      str("one off"),
				"strategy":   str("single"),
				"start_date": str("2026-01-01 00:00"),
				"end_date":   str("2026-01-01 06:00"),
				"timezone":   str("UTC"),
			},
			want: func(t *testing.T, sent kuma.Maintenance) {
				if len(sent.DateRange) == 0 {
					t.Error("dateRange must always be sent: the column is NOT NULL with no default")
				}
			},
		},
		{
			name: "manual",
			plan: map[string]tftypes.Value{
				"title":    str("manual"),
				"strategy": str("manual"),
			},
			want: func(t *testing.T, sent kuma.Maintenance) {
				// No schedule at all; NormalizeMaintenance fills the rest.
				if sent.Strategy != "manual" {
					t.Errorf("strategy = %q, want manual", sent.Strategy)
				}
				if len(sent.Weekdays) != 0 || len(sent.DaysOfMonth) != 0 {
					t.Error("a manual window should carry no schedule fields")
				}
			},
		},
		{
			name: "recurring by interval",
			plan: map[string]tftypes.Value{
				"title":        str("every three days"),
				"strategy":     str("recurring-interval"),
				"interval_day": num(3),
				"start_time":   str("00:30"),
				"end_time":     str("01:30"),
				"timezone":     str("UTC"),
			},
			want: func(t *testing.T, sent kuma.Maintenance) {
				if sent.IntervalDay == nil || *sent.IntervalDay != 3 {
					t.Errorf("interval day = %v, want 3", sent.IntervalDay)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sent kuma.Maintenance
			client := mocks.NewMockKumaClient(gomock.NewController(t))
			client.EXPECT().CreateMaintenance(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, payload kuma.Maintenance) (int, error) {
					sent = payload
					return 6, nil
				})
			client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
			client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
			client.EXPECT().GetMaintenance(gomock.Any(), 6).
				DoAndReturn(func(_ any, _ int) (*kuma.Maintenance, error) {
					created := sent
					created.ID = 6
					return &created, nil
				}).AnyTimes()
			client.EXPECT().GetMaintenanceMonitors(gomock.Any(), 6).Return(nil, nil).AnyTimes()
			client.EXPECT().GetMaintenanceStatusPages(gomock.Any(), 6).Return(nil, nil).AnyTimes()

			r := configure(t, maintenance.New, client)
			if errs := r.create(t, r.state(t, tt.plan)); errs != "" {
				t.Fatalf("creating: %s", errs)
			}
			tt.want(t, sent)
		})
	}
}

// The server stores these as strings and validates loosely, so a typo would
// silently produce a window at the wrong hour.
func TestInvalidClockTimesAreRejectedBeforeTheServer(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"25:00",           // hour out of range
		"12:60",           // minute out of range
		"-1:00",           // negative
		"noon",            // not a time
		"2:00 and change", // Sscanf alone would accept this
		"2",               // no minutes
		"::",              // punctuation only
		"12:5x",           // trailing junk in the minutes
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			r := configure(t, maintenance.New, client)

			errs := r.create(t, r.state(t, map[string]tftypes.Value{
				"title":      str("bad time"),
				"strategy":   str("recurring-weekday"),
				"weekdays":   intSet(1),
				"start_time": str(value),
				"end_time":   str("02:00"),
			}))
			if errs == "" {
				t.Fatalf("%q was accepted as a time of day", value)
			}
			if !strings.Contains(errs, "start_time") {
				t.Errorf("the diagnostic should name the attribute: %s", errs)
			}
		})
	}
}

// A time written without zero padding has to survive: the server returns the
// padded form, and a mismatch is a permanent diff.
func TestValidClockTimesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"00:00", "23:59", "2:00", "9:05", "12:30"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			part, err := maintenance.ParseClockTime(value)
			if err != nil {
				t.Fatalf("%q should be a valid time: %s", value, err)
			}

			again, err := maintenance.ParseClockTime(maintenance.FormatClockTime(part))
			if err != nil {
				t.Fatalf("the canonical rendering of %q does not parse: %s", value, err)
			}
			if again != part {
				t.Errorf("%q does not round-trip: %v then %v", value, part, again)
			}
		})
	}
}

// Not-found handling, including the two association reads that follow.
func TestReadDropsAMaintenanceDeletedOutsideTerraform(t *testing.T) {
	t.Parallel()

	t.Run("the maintenance itself is gone", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(nil, gone)

		r := configure(t, maintenance.New, client)
		removed, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("6")}))
		if errs != "" {
			t.Fatalf("a deleted maintenance should not fail the run: %s", errs)
		}
		if !removed {
			t.Error("it should have been dropped from state")
		}
	})

	t.Run("the monitor list fails", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(&kuma.Maintenance{
			ID: 6, Title: "t", Strategy: "manual", Active: kuma.BoolPtr(true),
		}, nil)
		client.EXPECT().GetMaintenanceMonitors(gomock.Any(), 6).Return(nil, denied)

		r := configure(t, maintenance.New, client)
		if _, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("6")})); errs == "" {
			t.Error("a failing association read should surface")
		}
	})

	t.Run("the status page list fails", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(&kuma.Maintenance{
			ID: 6, Title: "t", Strategy: "manual", Active: kuma.BoolPtr(true),
		}, nil)
		client.EXPECT().GetMaintenanceMonitors(gomock.Any(), 6).Return([]int{1}, nil)
		client.EXPECT().GetMaintenanceStatusPages(gomock.Any(), 6).Return(nil, denied)

		r := configure(t, maintenance.New, client)
		if _, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("6")})); errs == "" {
			t.Error("a failing association read should surface")
		}
	})
}

// The association events replace the whole list; add-one-at-a-time would leave
// removed monitors attached.
func TestUpdateReplacesAssociationsWholesale(t *testing.T) {
	t.Parallel()

	var sentMonitors []int
	client := mocks.NewMockKumaClient(gomock.NewController(t))
	client.EXPECT().UpdateMaintenance(gomock.Any(), gomock.Any()).Return(nil)
	client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, gomock.Any()).
		DoAndReturn(func(_ any, _ int, ids []int) error {
			sentMonitors = ids
			return nil
		})
	client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
	client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(&kuma.Maintenance{
		ID: 6, Title: "t", Strategy: "manual", Active: kuma.BoolPtr(true),
	}, nil).AnyTimes()
	client.EXPECT().GetMaintenanceMonitors(gomock.Any(), 6).Return([]int{2}, nil).AnyTimes()
	client.EXPECT().GetMaintenanceStatusPages(gomock.Any(), 6).Return(nil, nil).AnyTimes()

	r := configure(t, maintenance.New, client)
	state := r.state(t, map[string]tftypes.Value{
		"id": str("6"), "title": str("t"), "strategy": str("manual"),
		"monitor_ids": intSet(1, 2, 3),
	})
	plan := r.state(t, map[string]tftypes.Value{
		"id": str("6"), "title": str("t"), "strategy": str("manual"),
		"monitor_ids": intSet(2),
	})

	if errs := r.update(t, plan, state); errs != "" {
		t.Fatalf("updating: %s", errs)
	}
	if len(sentMonitors) != 1 || sentMonitors[0] != 2 {
		t.Errorf("sent %v, want exactly [2] — the event replaces the whole list, so "+
			"anything else leaves removed monitors attached", sentMonitors)
	}
}
