# Kubernetes Operator Development

Patterns for developing Kubernetes operators in Go using controller-runtime and kubebuilder.

## Scaffolding with Kubebuilder

```bash
kubebuilder init --domain <DOMAIN> --repo github.com/<USER>/<PROJECT>
kubebuilder create api --group <GROUP> --version v1alpha1 --kind <KIND>
```

## Operator Structure

```
<OPERATOR>/
├── cmd/
│   └── main.go
├── api/
│   └── v1alpha1/
│       ├── <KIND>_types.go          # CRD type definitions
│       ├── <KIND>_types_test.go
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go  # Generated
├── internal/
│   └── controller/
│       ├── <KIND>_controller.go      # Reconcile logic
│       └── <KIND>_controller_test.go
├── config/
│   ├── crd/                          # Generated CRD manifests
│   ├── rbac/                         # RBAC roles
│   ├── manager/                      # Controller manager config
│   └── samples/                      # Example CRs
├── go.mod
├── go.sum
├── Makefile
└── Dockerfile
```

## CRD Type Definitions

Use kubebuilder markers for validation and documentation:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type MyResource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   MyResourceSpec   `json:"spec,omitempty"`
    Status MyResourceStatus `json:"status,omitempty"`
}

type MyResourceSpec struct {
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    Name string `json:"name"`

    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=10
    // +kubebuilder:default=1
    Replicas int32 `json:"replicas,omitempty"`

    // +kubebuilder:validation:Enum=small;medium;large
    Size string `json:"size"`
}

type MyResourceStatus struct {
    // +kubebuilder:validation:Enum=Pending;Running;Failed;Ready
    Phase string `json:"phase,omitempty"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

## Reconcile Pattern

```go
func (r *MyResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. Fetch the resource
    var obj myv1.MyResource
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Handle finalizers for cleanup
    if !obj.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &obj)
    }
    if !controllerutil.ContainsFinalizer(&obj, finalizerName) {
        controllerutil.AddFinalizer(&obj, finalizerName)
        if err := r.Update(ctx, &obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 3. Reconcile desired state (idempotent)
    result, err := r.reconcileNormal(ctx, &obj)
    if err != nil {
        // Update status with error condition
        meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
            Type:    "Ready",
            Status:  metav1.ConditionFalse,
            Reason:  "ReconcileFailed",
            Message: err.Error(),
        })
        if updateErr := r.Status().Update(ctx, &obj); updateErr != nil {
            log.Error(updateErr, "failed to update status")
        }
        return result, err
    }

    // 4. Update status with success condition
    meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
        Type:    "Ready",
        Status:  metav1.ConditionTrue,
        Reason:  "ReconcileSucceeded",
        Message: "Resource is ready",
    })
    obj.Status.Phase = "Ready"
    if err := r.Status().Update(ctx, &obj); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

## Owner References and Garbage Collection

```go
func (r *MyResourceReconciler) reconcileDeployment(ctx context.Context, owner *myv1.MyResource) error {
    deploy := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      owner.Name,
            Namespace: owner.Namespace,
        },
    }

    _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
        // Set owner reference for automatic garbage collection
        if err := controllerutil.SetControllerReference(owner, deploy, r.Scheme); err != nil {
            return err
        }
        // Set desired spec
        deploy.Spec = desiredDeploymentSpec(owner)
        return nil
    })
    return err
}
```

## Testing with envtest

```go
var (
    k8sClient client.Client
    testEnv   *envtest.Environment
)

func TestMain(m *testing.M) {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
    }

    cfg, err := testEnv.Start()
    if err != nil {
        panic(err)
    }

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    if err != nil {
        panic(err)
    }

    code := m.Run()
    testEnv.Stop()
    os.Exit(code)
}

func TestReconcile(t *testing.T) {
    ctx := context.Background()

    // Create CR
    obj := &myv1.MyResource{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-resource",
            Namespace: "default",
        },
        Spec: myv1.MyResourceSpec{
            Name:     "test",
            Replicas: 1,
            Size:     "small",
        },
    }
    if err := k8sClient.Create(ctx, obj); err != nil {
        t.Fatal(err)
    }

    // Trigger reconciliation and verify
    // ...
}
```

## Code Generation

```makefile
.PHONY: generate
generate:
	controller-gen object paths="./..."  # DeepCopy
	controller-gen crd paths="./..." output:crd:artifacts:config=config/crd/bases
	controller-gen rbac:roleName=manager-role paths="./..." output:rbac:artifacts:config=config/rbac
```

## Sources

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Operator SDK](https://sdk.operatorframework.io/)
