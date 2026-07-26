package kuma_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// A status page's group tree is the one thing the provider reads over HTTP,
// because no Socket.IO event returns it. These tests cover what a real Uptime
// Kuma will not do on request: answer 404, fail with a 500, or send a body that
// does not parse.

func TestStatusPageGroupsOverHTTP(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed response yields the groups in order", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/status-page/") {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"config": {"slug":"public","title":"T"},
				"publicGroupList": [
					{"id":1,"name":"First","weight":1,"monitorList":[{"id":10,"sendUrl":1}]},
					{"id":2,"name":"Second","weight":2,"monitorList":[]}
				]
			}`))
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		groups, err := client.GetStatusPageGroups(context.Background(), "public")
		if err != nil {
			t.Fatalf("GetStatusPageGroups: %v", err)
		}
		if len(groups) != 2 {
			t.Fatalf("got %d groups", len(groups))
		}
		if groups[0].Name != "First" || groups[1].Name != "Second" {
			t.Errorf("order was lost: %q, %q", groups[0].Name, groups[1].Name)
		}
		// sendUrl arrives as 1, straight from SQLite.
		if !groups[0].MonitorList[0].SendURL.Value() {
			t.Error("sendUrl 1 should read as true")
		}
	})

	t.Run("the slug is lowercased and escaped", func(t *testing.T) {
		t.Parallel()

		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"publicGroupList":[]}`))
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		if _, err := client.GetStatusPageGroups(context.Background(), "MiXeD Case"); err != nil {
			t.Fatalf("GetStatusPageGroups: %v", err)
		}
		// The server lowercases slugs, so a request with capitals would 404.
		if !strings.Contains(gotPath, "mixed") {
			t.Errorf("path %q should carry a lowercased slug", gotPath)
		}
		if strings.Contains(gotPath, " ") {
			t.Errorf("path %q should be escaped", gotPath)
		}
	})

	t.Run("404 becomes not-found", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false,"msg":"Status Page Not Found"}`))
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		_, err := client.GetStatusPageGroups(context.Background(), "missing")
		if !kuma.IsNotFound(err) {
			t.Errorf("a 404 should classify as not-found, got %v", err)
		}
	})

	t.Run("a 500 is an error but not not-found", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		_, err := client.GetStatusPageGroups(context.Background(), "public")
		if err == nil {
			t.Fatal("expected an error")
		}
		// Treating a 500 as not-found would delete the resource from state over a
		// transient server problem.
		if kuma.IsNotFound(err) {
			t.Error("a 500 must not be mistaken for a missing page")
		}
	})

	t.Run("a body that is not JSON fails", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>not json</html>`))
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		if _, err := client.GetStatusPageGroups(context.Background(), "public"); err == nil {
			t.Error("expected a decode error")
		}
	})

	t.Run("an unreachable server fails", func(t *testing.T) {
		t.Parallel()

		client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
		if _, err := client.GetStatusPageGroups(context.Background(), "public"); err == nil {
			t.Error("expected a transport error")
		}
	})

	t.Run("an unparseable endpoint fails before dialing", func(t *testing.T) {
		t.Parallel()

		client := kuma.NewForHTTPTestOnly("http://[::1]:namedport")
		if _, err := client.GetStatusPageGroups(context.Background(), "public"); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("a cancelled context stops the request", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"publicGroupList":[]}`))
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := kuma.NewForHTTPTestOnly(server.URL)
		if _, err := client.GetStatusPageGroups(ctx, "public"); err == nil {
			t.Error("expected a context error")
		}
	})

	t.Run("a response with no group list yields none", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// A page with no groups omits the key entirely.
			_, _ = w.Write([]byte(`{"config":{"slug":"public"}}`))
		}))
		defer server.Close()

		client := kuma.NewForHTTPTestOnly(server.URL)
		groups, err := client.GetStatusPageGroups(context.Background(), "public")
		if err != nil {
			t.Fatalf("an absent list is not an error: %v", err)
		}
		if len(groups) != 0 {
			t.Errorf("got %d groups", len(groups))
		}
	})
}
