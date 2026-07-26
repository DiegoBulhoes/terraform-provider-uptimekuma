package monitor

// Test-only hooks. They let an external test package drive the per-type model
// hooks directly, which is the only way to round-trip every monitor type
// without standing up 33 acceptance tests.

// DefForTest exposes the type descriptor the resource was built with.
func (r *Resource) DefForTest() TypeDef { return r.def }

// NewModelForTest returns an empty model of this resource's concrete type.
func (r *Resource) NewModelForTest() Model { return r.def.NewModel() }
