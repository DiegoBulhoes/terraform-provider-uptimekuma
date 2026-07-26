package common

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// DefaultTimeout is the default per-operation timeout used by resources.
const DefaultTimeout = 5 * time.Minute

// IsSet returns true if a Terraform attribute value is neither null nor unknown.
func IsSet(val interface {
	IsNull() bool
	IsUnknown() bool
}) bool {
	return !val.IsNull() && !val.IsUnknown()
}

// StringPtr converts a Terraform string into *string, mapping null and unknown
// onto nil so the wire payload keeps them absent.
func StringPtr(val types.String) *string {
	if !IsSet(val) {
		return nil
	}
	v := val.ValueString()
	return &v
}

// BoolPtr converts a Terraform bool into the wire boolean type.
func BoolPtr(val types.Bool) *kuma.Bool {
	if !IsSet(val) {
		return nil
	}
	return kuma.BoolPtr(val.ValueBool())
}

// IntPtr converts a Terraform int64 into *int.
func IntPtr(val types.Int64) *int {
	if !IsSet(val) {
		return nil
	}
	v := int(val.ValueInt64())
	return &v
}

// Float64Ptr converts a Terraform float64 into *float64.
func Float64Ptr(val types.Float64) *float64 {
	if !IsSet(val) {
		return nil
	}
	v := val.ValueFloat64()
	return &v
}

// StringValue converts *string back into a Terraform string, mapping nil onto
// null.
func StringValue(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// BoolValue converts the wire boolean back into a Terraform bool.
func BoolValue(v *kuma.Bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(bool(*v))
}

// IntValue converts *int back into a Terraform int64.
func IntValue(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// Float64Value converts *float64 back into a Terraform float64.
func Float64Value(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

// ParseID turns the string ID Terraform stores into the numeric one the API
// uses. Every resource and data source keys off a numeric Uptime Kuma ID while
// Terraform models IDs as strings.
func ParseID(id types.String, diags *diag.Diagnostics) (int, bool) {
	parsed, err := strconv.Atoi(id.ValueString())
	if err != nil {
		diags.AddError(
			"Invalid resource ID",
			fmt.Sprintf("Expected a numeric ID, got %q.", id.ValueString()),
		)
		return 0, false
	}
	return parsed, true
}

// OptionalString maps both nil and the empty string onto null.
//
// Uptime Kuma stores an unset text field as an empty string, while Terraform
// models it as null; without this the two disagree on every plan.
func OptionalString(v *string) types.String {
	if v == nil || *v == "" {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// BoolOrTrue reads a wire boolean, treating absent as true. Used for fields the
// server defaults to enabled, such as a monitor's active state.
func BoolOrTrue(v *kuma.Bool) types.Bool {
	if v == nil {
		return types.BoolValue(true)
	}
	return types.BoolValue(bool(*v))
}

// BoolOrFalse reads a wire boolean, treating absent as false. Computed
// attributes must never be left null, so an omitted flag becomes an explicit
// false rather than an unknown value.
func BoolOrFalse(v *kuma.Bool) types.Bool {
	if v == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(bool(*v))
}

// Int64Set converts a slice of ints into a Terraform set.
func Int64Set(values []int, diags *diag.Diagnostics) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.Int64Value(int64(value)))
	}
	set, setDiags := types.SetValue(types.Int64Type, elements)
	diags.Append(setDiags...)
	return set
}

// StringSetToSlice converts a types.Set of strings into a []string.
func StringSetToSlice(ctx context.Context, set types.Set) []string {
	if !IsSet(set) {
		return nil
	}
	var elems []types.String
	set.ElementsAs(ctx, &elems, false)
	result := make([]string, len(elems))
	for i, e := range elems {
		result[i] = e.ValueString()
	}
	return result
}

// StringListToSlice converts a types.List of strings into a []string.
func StringListToSlice(ctx context.Context, list types.List) []string {
	if !IsSet(list) {
		return nil
	}
	var elems []types.String
	list.ElementsAs(ctx, &elems, false)
	result := make([]string, len(elems))
	for i, e := range elems {
		result[i] = e.ValueString()
	}
	return result
}

// Int64SetToSlice converts a types.Set of numbers into an []int.
func Int64SetToSlice(ctx context.Context, set types.Set) []int {
	if !IsSet(set) {
		return nil
	}
	var elems []types.Int64
	set.ElementsAs(ctx, &elems, false)
	result := make([]int, len(elems))
	for i, e := range elems {
		result[i] = int(e.ValueInt64())
	}
	return result
}

// RetryRPC runs an operation, repeating it on transient failures with
// exponential backoff (1s, 2s, 4s).
//
// Retries matter more here than with a request/response API: the provider holds
// a long-lived Socket.IO session, and the first call after an idle period is the
// one that discovers the connection died.
func RetryRPC(ctx context.Context, attempts int, op func() error) error {
	if attempts < 1 {
		attempts = 1
	}

	var err error
	for attempt := range attempts {
		err = op()
		if err == nil || !kuma.IsRetryable(err) {
			return err
		}

		if attempt == attempts-1 {
			break
		}

		wait := time.Duration(1<<uint(attempt)) * time.Second
		tflog.Warn(ctx, "Transient Uptime Kuma error, retrying", map[string]any{
			"attempt": attempt + 1,
			"error":   err.Error(),
			"wait":    wait.String(),
		})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return err
}

// EnvOrDefault returns the Terraform attribute value, or falls back to the given
// environment variable, or finally the default value.
func EnvOrDefault(val types.String, envVar, defaultVal string) string {
	if IsSet(val) {
		return val.ValueString()
	}
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
	}
	return defaultVal
}

// EnvOrDefaultInt is EnvOrDefault for integers.
func EnvOrDefaultInt(val types.Int64, envVar string, defaultVal int) int {
	if IsSet(val) {
		return int(val.ValueInt64())
	}
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return defaultVal
}

// EnvOrDefaultBool is EnvOrDefault for booleans.
func EnvOrDefaultBool(val types.Bool, envVar string, defaultVal bool) bool {
	if IsSet(val) {
		return val.ValueBool()
	}
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
	}
	return defaultVal
}
