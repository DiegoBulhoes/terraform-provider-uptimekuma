package maintenance

import (
	"fmt"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Times of day and the strategies that use them.
//
// Uptime Kuma stores these as strings and validates loosely, so a typo would be
// accepted and produce a window at the wrong hour.

// IsRecurring reports whether a strategy uses the daily time range. The server
// only reads timeRange, weekdays and daysOfMonth for these.
func IsRecurring(strategy string) bool {
	switch strategy {
	case StrategyRecurringInterval, StrategyRecurringWeekday, StrategyRecurringDayOfMonth:
		return true
	default:
		return false
	}
}

// ParseClockTime converts "HH:MM" into the hours/minutes object the API uses.
func ParseClockTime(value string) (kuma.TimePart, error) {
	var hours, minutes int
	// Sscanf alone is too permissive — it accepts "2:00 and then some" — so the
	// parsed values are rendered back and compared.
	if _, err := fmt.Sscanf(value, "%d:%d", &hours, &minutes); err != nil {
		return kuma.TimePart{}, fmt.Errorf("must be in HH:MM format, got %q", value)
	}
	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return kuma.TimePart{}, fmt.Errorf("must be a valid time of day, got %q", value)
	}
	part := kuma.TimePart{Hours: hours, Minutes: minutes}
	if FormatClockTime(part) != normalizeClockText(value) {
		return kuma.TimePart{}, fmt.Errorf("must be in HH:MM format, got %q", value)
	}
	return part, nil
}

// FormatClockTime renders a time part back as "HH:MM".
func FormatClockTime(part kuma.TimePart) string {
	return fmt.Sprintf("%02d:%02d", part.Hours, part.Minutes)
}

// normalizeClockText zero-pads a "H:M" input so it can be compared with the
// canonical rendering.
func normalizeClockText(value string) string {
	hours, minutes, found := strings.Cut(strings.TrimSpace(value), ":")
	if !found {
		return value
	}
	if len(hours) == 1 {
		hours = "0" + hours
	}
	if len(minutes) == 1 {
		minutes = "0" + minutes
	}
	return hours + ":" + minutes
}

// parseClockTime adapts ParseClockTime to Terraform diagnostics.
func parseClockTime(value types.String, attribute string, diags *diag.Diagnostics) (kuma.TimePart, bool) {
	if !common.IsSet(value) {
		return kuma.TimePart{}, true
	}
	part, err := ParseClockTime(value.ValueString())
	if err != nil {
		diags.AddError("Invalid time", fmt.Sprintf("`%s` %s.", attribute, err))
		return kuma.TimePart{}, false
	}
	return part, true
}
