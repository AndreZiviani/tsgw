package main

import (
	"context"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

// localClient is the subset of tailscale.com/client/local.Client we use.
type localClient interface {
	GetPrefs(ctx context.Context) (*ipn.Prefs, error)
	EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error)
	GetServeConfig(ctx context.Context) (*ipn.ServeConfig, error)
	SetServeConfig(ctx context.Context, cfg *ipn.ServeConfig) error
	StatusWithoutPeers(ctx context.Context) (*ipnstate.Status, error)
}
