package kuma

import (
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/client"
)

// Test-only hooks, re-exported so tests import one package.

type (
	SessionForTest      = client.SessionForTest
	TLSWebSocketForTest = client.TLSWebSocketForTest
)

var (
	ConnectBackoffForTest        = client.ConnectBackoffForTest
	DecodeAckForTest             = client.DecodeAckForTest
	NewForHTTPTestOnly           = client.NewForHTTPTestOnly
	BuildForTest                 = client.BuildForTest
	NewWithoutBaseContextForTest = client.NewWithoutBaseContextForTest
	NewTLSWebSocketForTest       = client.NewTLSWebSocketForTest
	DialForTest                  = client.DialForTest
	HTTPClientForTest            = client.HTTPClientForTest
	PoolKeyForTest               = client.PoolKeyForTest
	SeedPoolForTest              = client.SeedPoolForTest
	ResetPoolForTest             = client.ResetPoolForTest
)
