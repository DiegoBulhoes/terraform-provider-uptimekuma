package common_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestIsSet covers the null/unknown check every conversion depends on.
//
// Unknown matters as much as null: during a plan, an Optional+Computed attribute
// the user did not set is unknown, and treating that as a value would send a
// placeholder to the server.
func TestIsSet(t *testing.T) {
	t.Parallel()

	if common.IsSet(types.StringNull()) {
		t.Error("null must not count as set")
	}
	if common.IsSet(types.StringUnknown()) {
		t.Error("unknown must not count as set")
	}
	if !common.IsSet(types.StringValue("")) {
		t.Error("an empty string is still a value the user chose")
	}
	if !common.IsSet(types.StringValue("x")) {
		t.Error("a plain string must count as set")
	}
	if common.IsSet(types.Int64Null()) || common.IsSet(types.BoolUnknown()) {
		t.Error("null and unknown must not count as set for other types")
	}
}

// TestPointerConversions checks that null and unknown both become nil, which is
// what keeps an unset attribute out of the wire payload.
func TestPointerConversions(t *testing.T) {
	t.Parallel()

	if got := common.StringPtr(types.StringNull()); got != nil {
		t.Errorf("StringPtr(null) = %v, want nil", *got)
	}
	if got := common.StringPtr(types.StringUnknown()); got != nil {
		t.Errorf("StringPtr(unknown) = %v, want nil", *got)
	}
	if got := common.StringPtr(types.StringValue("value")); got == nil || *got != "value" {
		t.Errorf("StringPtr(value) = %v", got)
	}
	// An empty string is deliberately preserved: it is how a field gets cleared.
	if got := common.StringPtr(types.StringValue("")); got == nil || *got != "" {
		t.Errorf("StringPtr(\"\") = %v, want a pointer to the empty string", got)
	}

	if got := common.IntPtr(types.Int64Null()); got != nil {
		t.Errorf("IntPtr(null) = %v, want nil", *got)
	}
	if got := common.IntPtr(types.Int64Value(3128)); got == nil || *got != 3128 {
		t.Errorf("IntPtr(3128) = %v", got)
	}

	if got := common.BoolPtr(types.BoolNull()); got != nil {
		t.Errorf("BoolPtr(null) = %v, want nil", *got)
	}
	if got := common.BoolPtr(types.BoolValue(true)); got == nil || !got.Value() {
		t.Errorf("BoolPtr(true) = %v", got)
	}
	// False must survive as a pointer to false, not collapse to nil.
	if got := common.BoolPtr(types.BoolValue(false)); got == nil || got.Value() {
		t.Errorf("BoolPtr(false) = %v, want a pointer to false", got)
	}

	if got := common.Float64Ptr(types.Float64Value(1.5)); got == nil || *got != 1.5 {
		t.Errorf("Float64Ptr(1.5) = %v", got)
	}
}

// TestValueConversions covers the reverse direction, where nil becomes null.
func TestValueConversions(t *testing.T) {
	t.Parallel()

	if got := common.StringValue(nil); !got.IsNull() {
		t.Error("StringValue(nil) must be null")
	}
	value := "x"
	if got := common.StringValue(&value); got.ValueString() != "x" {
		t.Errorf("StringValue = %q", got.ValueString())
	}

	if got := common.IntValue(nil); !got.IsNull() {
		t.Error("IntValue(nil) must be null")
	}
	number := 7
	if got := common.IntValue(&number); got.ValueInt64() != 7 {
		t.Errorf("IntValue = %d", got.ValueInt64())
	}

	if got := common.BoolValue(nil); !got.IsNull() {
		t.Error("BoolValue(nil) must be null")
	}
	if got := common.BoolValue(kuma.BoolPtr(true)); !got.ValueBool() {
		t.Error("BoolValue(true) should be true")
	}

	if got := common.Float64Value(nil); !got.IsNull() {
		t.Error("Float64Value(nil) must be null")
	}
}

// TestCollectionConversions covers the set helpers used for notification IDs,
// status codes and maintenance weekdays.
func TestCollectionConversions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	strings, diags := types.SetValue(types.StringType, []attr.Value{
		types.StringValue("200-299"),
		types.StringValue("404"),
	})
	if diags.HasError() {
		t.Fatalf("building the set: %s", diags)
	}
	if got := common.StringSetToSlice(ctx, strings); len(got) != 2 {
		t.Errorf("StringSetToSlice = %v, want 2 entries", got)
	}

	// A null set must produce nil, not an empty slice: the two mean different
	// things to the server for status codes.
	if got := common.StringSetToSlice(ctx, types.SetNull(types.StringType)); got != nil {
		t.Errorf("StringSetToSlice(null) = %v, want nil", got)
	}

	numbers, diags := types.SetValue(types.Int64Type, []attr.Value{
		types.Int64Value(1),
		types.Int64Value(3),
		types.Int64Value(5),
	})
	if diags.HasError() {
		t.Fatalf("building the set: %s", diags)
	}
	got := common.Int64SetToSlice(ctx, numbers)
	if len(got) != 3 {
		t.Fatalf("Int64SetToSlice = %v, want 3 entries", got)
	}
	total := 0
	for _, value := range got {
		total += value
	}
	if total != 9 {
		t.Errorf("values sum to %d, want 9", total)
	}

	if got := common.Int64SetToSlice(ctx, types.SetNull(types.Int64Type)); got != nil {
		t.Errorf("Int64SetToSlice(null) = %v, want nil", got)
	}
}

// TestEnvOrDefault covers the precedence of attribute, environment and default.
func TestEnvOrDefault(t *testing.T) {
	// No t.Parallel here: these tests set environment variables, which the
	// testing package forbids in parallel tests.

	t.Setenv("UPTIME_KUMA_TEST_VALUE", "from-env")

	// The attribute wins over the environment.
	if got := common.EnvOrDefault(types.StringValue("from-config"), "UPTIME_KUMA_TEST_VALUE", "fallback"); got != "from-config" {
		t.Errorf("got %q, want from-config", got)
	}
	// The environment wins over the default.
	if got := common.EnvOrDefault(types.StringNull(), "UPTIME_KUMA_TEST_VALUE", "fallback"); got != "from-env" {
		t.Errorf("got %q, want from-env", got)
	}
	// The default is the last resort.
	if got := common.EnvOrDefault(types.StringNull(), "UPTIME_KUMA_UNSET_VALUE", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

func TestEnvOrDefaultInt(t *testing.T) {
	// No t.Parallel here: these tests set environment variables, which the
	// testing package forbids in parallel tests.

	t.Setenv("UPTIME_KUMA_TEST_TIMEOUT", "45")

	if got := common.EnvOrDefaultInt(types.Int64Value(10), "UPTIME_KUMA_TEST_TIMEOUT", 30); got != 10 {
		t.Errorf("got %d, want 10", got)
	}
	if got := common.EnvOrDefaultInt(types.Int64Null(), "UPTIME_KUMA_TEST_TIMEOUT", 30); got != 45 {
		t.Errorf("got %d, want 45", got)
	}
	if got := common.EnvOrDefaultInt(types.Int64Null(), "UPTIME_KUMA_UNSET", 30); got != 30 {
		t.Errorf("got %d, want 30", got)
	}

	// A non-numeric environment value falls back rather than failing the run.
	t.Setenv("UPTIME_KUMA_TEST_BAD", "not-a-number")
	if got := common.EnvOrDefaultInt(types.Int64Null(), "UPTIME_KUMA_TEST_BAD", 30); got != 30 {
		t.Errorf("got %d, want the default when the env var is unparseable", got)
	}
}

func TestEnvOrDefaultBool(t *testing.T) {
	// No t.Parallel here: these tests set environment variables, which the
	// testing package forbids in parallel tests.

	t.Setenv("UPTIME_KUMA_TEST_FLAG", "true")

	if got := common.EnvOrDefaultBool(types.BoolValue(false), "UPTIME_KUMA_TEST_FLAG", false); got {
		t.Error("the attribute must win, even when it is false")
	}
	if got := common.EnvOrDefaultBool(types.BoolNull(), "UPTIME_KUMA_TEST_FLAG", false); !got {
		t.Error("the environment should be used when the attribute is null")
	}
	if got := common.EnvOrDefaultBool(types.BoolNull(), "UPTIME_KUMA_UNSET", true); !got {
		t.Error("the default should be used when neither is set")
	}
}

// TestRetryRPC covers the retry loop the resources wrap every call in.
func TestRetryRPC(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("succeeds without retrying", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := common.RetryRPC(ctx, 3, func() error {
			calls++
			return nil
		})
		if err != nil {
			t.Fatalf("RetryRPC: %v", err)
		}
		if calls != 1 {
			t.Errorf("called %d times, want 1", calls)
		}
	})

	t.Run("does not retry a server rejection", func(t *testing.T) {
		t.Parallel()
		calls := 0
		rejection := &kuma.APIError{Event: "add", Msg: "Interval cannot be less than 20 seconds"}
		err := common.RetryRPC(ctx, 3, func() error {
			calls++
			return rejection
		})
		if !errors.Is(err, error(rejection)) {
			t.Errorf("error = %v, want the rejection", err)
		}
		// Replaying a rejected payload would fail identically, so one attempt is
		// all it gets.
		if calls != 1 {
			t.Errorf("called %d times, want 1", calls)
		}
	})

	t.Run("retries a transient failure then succeeds", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := common.RetryRPC(ctx, 3, func() error {
			calls++
			if calls < 2 {
				return kuma.ErrTimeout
			}
			return nil
		})
		if err != nil {
			t.Fatalf("RetryRPC: %v", err)
		}
		if calls != 2 {
			t.Errorf("called %d times, want 2", calls)
		}
	})

	t.Run("gives up after the last attempt", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := common.RetryRPC(ctx, 2, func() error {
			calls++
			return kuma.ErrTimeout
		})
		if !errors.Is(err, kuma.ErrTimeout) {
			t.Errorf("error = %v, want a timeout", err)
		}
		if calls != 2 {
			t.Errorf("called %d times, want 2", calls)
		}
	})

	t.Run("treats a non-positive attempt count as one", func(t *testing.T) {
		t.Parallel()
		calls := 0
		_ = common.RetryRPC(ctx, 0, func() error {
			calls++
			return kuma.ErrTimeout
		})
		if calls != 1 {
			t.Errorf("called %d times, want 1", calls)
		}
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		t.Parallel()
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		err := common.RetryRPC(cancelled, 3, func() error {
			calls++
			return kuma.ErrTimeout
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Errorf("called %d times, want 1", calls)
		}
	})
}

// TestConfigureClient covers the provider-data plumbing every resource uses.
func TestConfigureClient(t *testing.T) {
	t.Parallel()

	if _, err := common.ConfigureClient("not a client"); err == nil {
		t.Error("a wrong type must be rejected")
	}

	client := &kuma.Client{}
	got, err := common.ConfigureClient(client)
	if err != nil {
		t.Fatalf("ConfigureClient: %v", err)
	}
	if got == nil {
		t.Error("a valid client must be returned")
	}
}
