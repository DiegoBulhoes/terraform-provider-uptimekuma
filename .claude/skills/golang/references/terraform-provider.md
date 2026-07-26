# Terraform Provider Development

Patterns for developing Terraform providers in Go using `terraform-plugin-framework`.

## Framework Choice

Use `terraform-plugin-framework` for all new providers. The older SDKv2 is in maintenance mode.

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/provider"
    "github.com/hashicorp/terraform-plugin-framework/resource"
    "github.com/hashicorp/terraform-plugin-framework/datasource"
)
```

## Provider Structure -- Small Provider

For providers with fewer than ~20 resources, a flat structure under `internal/provider/` works well:

```
terraform-provider-<NAME>/
├── cmd/
│   └── terraform-provider-<NAME>/
│       └── main.go
├── internal/
│   └── provider/
│       ├── provider.go            # Provider definition and configuration
│       ├── provider_test.go
│       ├── <RESOURCE>_resource.go
│       ├── <RESOURCE>_resource_test.go
│       ├── <RESOURCE>_data_source.go
│       └── <RESOURCE>_data_source_test.go
├── examples/                       # HCL examples for documentation
│   ├── provider/
│   ├── resources/
│   └── data-sources/
├── templates/                      # tfplugindocs templates
├── go.mod
├── go.sum
├── GNUmakefile
└── .goreleaser.yml
```

## Provider Structure -- Large Provider (Service-per-Resource)

For providers with many resources (50+), use the **service-per-resource** pattern (as used by [cloudflare/terraform-provider-cloudflare](https://github.com/cloudflare/terraform-provider-cloudflare)):

```
terraform-provider-<NAME>/
├── main.go
├── internal/
│   ├── provider.go                # Provider definition, registers all resources
│   ├── consts/                    # Shared constants (schema keys, env vars)
│   ├── acctest/                   # Test helpers, shared factories, config loaders
│   ├── utils/                     # Random name generators, sweeper helpers
│   ├── services/                  # One directory per resource family
│   │   ├── <RESOURCE_A>/
│   │   │   ├── resource.go        # CRUD + ImportState + ModifyPlan
│   │   │   ├── schema.go          # ResourceSchema() definition
│   │   │   ├── model.go           # Terraform model structs
│   │   │   ├── data_source.go     # Single-item data source Read
│   │   │   ├── data_source_schema.go
│   │   │   ├── data_source_model.go
│   │   │   ├── list_data_source.go  # List data source (when applicable)
│   │   │   ├── resource_test.go
│   │   │   └── testdata/          # .tf template files for tests
│   │   │       ├── basic.tf
│   │   │       └── update.tf
│   │   └── <RESOURCE_B>/
│   │       └── ...
├── examples/
├── templates/
├── go.mod
└── GNUmakefile
```

This pattern provides:
- **Isolation** -- each resource is a self-contained package
- **Discoverability** -- `services/dns_record/` maps directly to `<provider>_dns_record`
- **Consistent file set** -- every service directory follows the same file layout
- **Scalability** -- adding a resource means adding a directory, not editing shared files

## Resource Lifecycle

Every resource MUST implement the full CRUD lifecycle:

```go
type ExampleResource struct {
    client *api.Client
}

func (r *ExampleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_example"
}

func (r *ExampleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Description: "Manages an example resource.",
        Attributes: map[string]schema.Attribute{
            "id": schema.StringAttribute{
                Computed:    true,
                Description: "Resource identifier.",
                PlanModifiers: []planmodifier.String{
                    stringplanmodifier.UseStateForUnknown(),
                },
            },
            "name": schema.StringAttribute{
                Required:    true,
                Description: "Resource name.",
                Validators: []validator.String{
                    stringvalidator.LengthBetween(1, 255),
                },
            },
        },
    }
}

func (r *ExampleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan ExampleModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    result, err := r.client.Create(ctx, plan.toAPI())
    if err != nil {
        resp.Diagnostics.AddError("Create failed", err.Error())
        return
    }

    plan.ID = types.StringValue(result.ID)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ExampleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state ExampleModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    result, err := r.client.Get(ctx, state.ID.ValueString())
    if err != nil {
        // Resource was deleted outside Terraform
        resp.State.RemoveResource(ctx)
        return
    }

    state.fromAPI(result)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ExampleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan ExampleModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() {
        return
    }

    result, err := r.client.Update(ctx, plan.ID.ValueString(), plan.toAPI())
    if err != nil {
        resp.Diagnostics.AddError("Update failed", err.Error())
        return
    }

    plan.fromAPI(result)
    resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ExampleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state ExampleModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    if err := r.client.Delete(ctx, state.ID.ValueString()); err != nil {
        resp.Diagnostics.AddError("Delete failed", err.Error())
        return
    }
}

func (r *ExampleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

## Interface Compliance at Compile Time

Verify all resource interfaces at compile time using the nil pointer pattern:

```go
var _ resource.ResourceWithConfigure = (*ExampleResource)(nil)
var _ resource.ResourceWithModifyPlan = (*ExampleResource)(nil)
var _ resource.ResourceWithImportState = (*ExampleResource)(nil)
```

This fails compilation if the type no longer satisfies an interface, catching regressions early.

## Model Mapping Pattern

Separate Terraform model from API model. Use `tfsdk` tags for Terraform and `json` tags for API serialization:

```go
type ExampleModel struct {
    ID   types.String `tfsdk:"id" json:"id,computed"`
    Name types.String `tfsdk:"name" json:"name,required"`
    Tags types.Map    `tfsdk:"tags" json:"tags,optional"`
}

func (m *ExampleModel) toAPI() *api.Example {
    return &api.Example{
        Name: m.Name.ValueString(),
    }
}

func (m *ExampleModel) fromAPI(a *api.Example) {
    m.ID = types.StringValue(a.ID)
    m.Name = types.StringValue(a.Name)
}
```

For large providers, consider a reflection-based JSON bridge (as the Cloudflare provider does with its `apijson` package) that reads struct tags to determine serialization behavior -- `computed` fields are only populated on Create/Update responses, while `Read` populates all fields.

## Read on 404 -- Graceful Removal

When a resource is deleted outside Terraform, `Read` MUST remove it from state instead of erroring:

```go
func (r *ExampleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state ExampleModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() {
        return
    }

    result, err := r.client.Get(ctx, state.ID.ValueString())
    if err != nil {
        if isNotFound(err) {
            resp.State.RemoveResource(ctx)
            return
        }
        resp.Diagnostics.AddError("Read failed", err.Error())
        return
    }

    state.fromAPI(result)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

## Schema Patterns

Common schema attribute patterns for the `terraform-plugin-framework`:

```go
// Computed ID with stable plan (server-assigned, doesn't change after create)
"id": schema.StringAttribute{
    Computed:      true,
    PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
},

// Immutable field (changing forces recreation)
"zone_id": schema.StringAttribute{
    Required:      true,
    PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
},

// Enum with case-insensitive matching
"type": schema.StringAttribute{
    Required: true,
    Validators: []validator.String{
        stringvalidator.OneOfCaseInsensitive("A", "AAAA", "CNAME", "MX", "TXT"),
    },
},

// Numeric range
"ttl": schema.Int64Attribute{
    Optional: true,
    Validators: []validator.Int64{
        int64validator.Between(60, 86400),
    },
},

// Optional boolean with default
"proxied": schema.BoolAttribute{
    Optional: true,
    Computed: true,
    Default:  booldefault.StaticBool(false),
},

// Nested object
"settings": schema.SingleNestedAttribute{
    Optional: true,
    Attributes: map[string]schema.Attribute{
        "ssl": schema.StringAttribute{Optional: true},
    },
},
```

## Import with Composite IDs

For resources that require more than just an ID to import (e.g., `<zone_id>/<record_id>`):

```go
func (r *ExampleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    parts := strings.SplitN(req.ID, "/", 2)
    if len(parts) != 2 {
        resp.Diagnostics.AddError(
            "Invalid import ID",
            fmt.Sprintf("Expected format: <zone_id>/<resource_id>, got: %s", req.ID),
        )
        return
    }

    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone_id"), parts[0])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
```

## Acceptance Tests

Use `terraform-plugin-testing` with real infrastructure:

```go
//go:build acceptance

func TestAccExampleResource(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProviderFactories,
        Steps: []resource.TestStep{
            // Create and verify
            {
                Config: testAccExampleConfig("test-name"),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("example_resource.test", "name", "test-name"),
                    resource.TestCheckResourceAttrSet("example_resource.test", "id"),
                ),
            },
            // Import
            {
                ResourceName:      "example_resource.test",
                ImportState:       true,
                ImportStateVerify: true,
            },
            // Update
            {
                Config: testAccExampleConfig("updated-name"),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("example_resource.test", "name", "updated-name"),
                ),
            },
        },
    })
}

func testAccExampleConfig(name string) string {
    return fmt.Sprintf(`
resource "example_resource" "test" {
  name = %q
}
`, name)
}
```

## Testing Patterns

- Use `ProtoV6ProviderFactories` (not the deprecated `ProviderFactories`)
- Always test Import state
- Test Update in the same test case (verify no recreation)
- Use `resource.ComposeAggregateTestCheckFunc` to combine checks
- Use `TestCheckResourceAttrSet` for computed values
- Use `ConfigPlanChecks` with `plancheck.ExpectNonEmptyPlan()` / `plancheck.ExpectEmptyPlan()`

## External Test Config Templates

For large providers, use external `.tf` files instead of inline HCL strings (as the Cloudflare provider does). Store templates in `testdata/` per service:

```go
// acctest/helpers.go
func LoadTestCase(filename string, args ...interface{}) string {
    _, file, _, _ := runtime.Caller(1)
    dir := filepath.Dir(file)
    data, err := os.ReadFile(filepath.Join(dir, "testdata", filename))
    if err != nil {
        panic(err)
    }
    return fmt.Sprintf(string(data), args...)
}

// resource_test.go
func TestAccDNSRecord_Basic(t *testing.T) {
    rnd := generateRandomName()
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: acctest.TestAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: acctest.LoadTestCase("basic.tf", zoneID, rnd, domain),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("cloudflare_dns_record."+rnd, "type", "A"),
                ),
            },
        },
    })
}
```

```hcl
# testdata/basic.tf
resource "cloudflare_dns_record" "%[2]s" {
  zone_id = "%[1]s"
  name    = "%[2]s.%[3]s"
  content = "192.168.0.10"
  type    = "A"
  ttl     = 3600
}
```

## Test Sweepers

Register sweepers to clean up resources after failed acceptance tests:

```go
func init() {
    resource.AddTestSweepers("cloudflare_dns_record", &resource.Sweeper{
        Name: "cloudflare_dns_record",
        F: func(region string) error {
            client := acctest.SharedClient()
            records, _ := client.ListDNSRecords(ctx, zoneID)
            for _, r := range records {
                if strings.HasPrefix(r.Name, "tf-acc-") {
                    client.DeleteDNSRecord(ctx, zoneID, r.ID)
                }
            }
            return nil
        },
    })
}
```

Run sweepers: `go test ./... -sweep=all -v`

## Code Generation

```makefile
.PHONY: generate
generate:
	go generate ./...
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
```

## Sources

- [cloudflare/terraform-provider-cloudflare](https://github.com/cloudflare/terraform-provider-cloudflare) -- Reference implementation: service-per-resource layout, dual-tag models, external test templates, schema patterns, sweepers
- [terraform-plugin-framework Documentation](https://developer.hashicorp.com/terraform/plugin/framework)
- [terraform-plugin-testing](https://developer.hashicorp.com/terraform/plugin/testing)
