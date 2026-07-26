package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/apikey"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/dockerhost"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/monitor"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/proxy"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/remotebrowser"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspageincident"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/tag"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// Parsing "abc" yields zero, and an operation against id 0 either fails
// confusingly or hits whatever row sits at that index. No mock call is expected,
// which is the assertion.

func TestEveryOperationRejectsAnUnparseableStateID(t *testing.T) {
	t.Parallel()

	numeric := map[string]func() fwresource.Resource{
		"monitor":        monitor.NewKeywordResource,
		"tag":            tag.New,
		"notification":   notification.New,
		"proxy":          proxy.New,
		"docker host":    dockerhost.New,
		"remote browser": remotebrowser.New,
		"api key":        apikey.New,
		"maintenance":    maintenance.New,
	}

	for name, factory := range numeric {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, id := range []string{"not-a-number", "", "1.5", "0x7", " 7 ", "9999999999999999999999"} {
				t.Run("id="+id, func(t *testing.T) {
					t.Parallel()

					readClient := mocks.NewMockKumaClient(gomock.NewController(t))
					readResource := configure(t, factory, readClient)
					state := readResource.state(t, map[string]tftypes.Value{"id": str(id)})
					if _, errs := readResource.read(t, state); errs == "" {
						t.Errorf("Read accepted %q as an ID", id)
					}

					deleteClient := mocks.NewMockKumaClient(gomock.NewController(t))
					deleteResource := configure(t, factory, deleteClient)
					deleteState := deleteResource.state(t, map[string]tftypes.Value{"id": str(id)})
					if errs := deleteResource.delete(t, deleteState); errs == "" {
						t.Errorf("Delete accepted %q as an ID", id)
					}
				})
			}
		})
	}
}

// The one composite ID: "<slug>/<id>" needs both halves.
func TestIncidentIDsNeedBothParts(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"9",           // no slug
		"public",      // no number
		"public/abc",  // non-numeric number
		"public/",     // empty number
		"/9",          // empty slug
		"a/b/9",       // too many parts
		"",            // nothing at all
		"public/9/xx", // trailing junk
	} {
		t.Run("id="+id, func(t *testing.T) {
			t.Parallel()

			readClient := mocks.NewMockKumaClient(gomock.NewController(t))
			readResource := configure(t, statuspageincident.New, readClient)
			state := readResource.state(t, map[string]tftypes.Value{"id": str(id)})
			if _, errs := readResource.read(t, state); errs == "" {
				t.Errorf("Read accepted %q as an incident ID", id)
			}

			deleteClient := mocks.NewMockKumaClient(gomock.NewController(t))
			deleteResource := configure(t, statuspageincident.New, deleteClient)
			deleteState := deleteResource.state(t, map[string]tftypes.Value{"id": str(id)})
			if errs := deleteResource.delete(t, deleteState); errs == "" {
				t.Errorf("Delete accepted %q as an incident ID", id)
			}
		})
	}
}
