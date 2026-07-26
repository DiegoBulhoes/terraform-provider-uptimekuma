# Go IaC Tooling -- Shared Patterns

Patterns shared between Terraform providers, Kubernetes operators, and other infrastructure tools in Go.

For domain-specific patterns, see:
- `terraform-provider.md` -- Terraform provider development (CRUD lifecycle, schemas, testing, service-per-resource layout)
- `kubernetes-operator.md` -- Kubernetes operator development (reconcile loops, CRDs, envtest, finalizers)

## API Client Pattern

Both providers and operators need an API client:

```go
type Client struct {
    httpClient *http.Client
    baseURL    string
    token      string
}

func NewClient(baseURL, token string) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        baseURL:    strings.TrimRight(baseURL, "/"),
        token:      token,
    }
}

func (c *Client) do(ctx context.Context, method, path string, body, result any) error {
    var reqBody io.Reader
    if body != nil {
        data, err := json.Marshal(body)
        if err != nil {
            return fmt.Errorf("marshaling request: %w", err)
        }
        reqBody = bytes.NewReader(data)
    }

    req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
    if err != nil {
        return fmt.Errorf("creating request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("executing request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return parseAPIError(resp)
    }

    if result != nil {
        if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
            return fmt.Errorf("decoding response: %w", err)
        }
    }
    return nil
}
```

## Retry and Backoff

```go
import "k8s.io/apimachinery/pkg/util/wait"

func (c *Client) GetWithRetry(ctx context.Context, id string) (*Resource, error) {
    var result *Resource
    err := wait.ExponentialBackoffWithContext(ctx, wait.Backoff{
        Duration: 1 * time.Second,
        Factor:   2.0,
        Jitter:   0.1,
        Steps:    5,
    }, func(ctx context.Context) (bool, error) {
        var err error
        result, err = c.Get(ctx, id)
        if err != nil {
            if isRetryable(err) {
                return false, nil // retry
            }
            return false, err // don't retry
        }
        return true, nil // done
    })
    return result, err
}
```

## Structured Logging

Both ecosystems converge on structured logging:

- **Terraform providers**: use `tflog` from `terraform-plugin-log`
- **Kubernetes operators**: use `logr` from `controller-runtime` (backed by `zap` or `slog`)

```go
// Terraform provider logging
tflog.Debug(ctx, "fetching resource", map[string]interface{}{
    "resource_id": id,
})

// Kubernetes operator logging
log := log.FromContext(ctx)
log.Info("reconciling resource", "name", req.Name, "namespace", req.Namespace)
```

Both redact sensitive fields -- use `tflog.MaskFieldValuesWithFieldKeys` for providers and structured `slog` attributes for operators.

## Context Propagation

ALWAYS propagate `context.Context` through the entire call chain:

- Terraform framework passes `ctx` to every CRUD method
- controller-runtime passes `ctx` to every `Reconcile` call
- API clients MUST accept `ctx` and use `http.NewRequestWithContext`

This ensures cancellation, timeouts, and tracing work end-to-end.

## Interface Compliance at Compile Time

Verify interfaces with nil pointer assignment (applicable to both ecosystems):

```go
// Terraform provider
var _ resource.ResourceWithConfigure = (*MyResource)(nil)
var _ resource.ResourceWithImportState = (*MyResource)(nil)

// Kubernetes operator
var _ reconcile.Reconciler = (*MyReconciler)(nil)
```

## Sources

- [cloudflare/terraform-provider-cloudflare](https://github.com/cloudflare/terraform-provider-cloudflare) -- Reference large-scale Terraform provider
- [terraform-plugin-framework Documentation](https://developer.hashicorp.com/terraform/plugin/framework)
- [terraform-plugin-testing](https://developer.hashicorp.com/terraform/plugin/testing)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Operator SDK](https://sdk.operatorframework.io/)
