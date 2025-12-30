package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/rs/zerolog/log"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func ensureAdvertiseServices(ctx context.Context, lc localClient, desired []tailcfg.ServiceName) error {
	prefs, err := lc.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}

	existing := make(map[string]struct{}, len(prefs.AdvertiseServices))
	for _, s := range prefs.AdvertiseServices {
		existing[s] = struct{}{}
	}
	for _, s := range desired {
		existing[s.String()] = struct{}{}
	}

	merged := make([]string, 0, len(existing))
	for s := range existing {
		merged = append(merged, s)
	}
	sort.Strings(merged)

	if equalStringSlices(merged, prefs.AdvertiseServices) {
		return nil
	}

	_, err = lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		AdvertiseServicesSet: true,
		Prefs: ipn.Prefs{
			AdvertiseServices: merged,
		},
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	log.Info().Strs("services", merged).Msg("Updated AdvertiseServices")
	return nil
}

func removeAdvertiseServices(ctx context.Context, lc localClient, toRemove []tailcfg.ServiceName) error {
	prefs, err := lc.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}

	removeSet := make(map[string]struct{}, len(toRemove))
	for _, s := range toRemove {
		removeSet[s.String()] = struct{}{}
	}

	kept := make([]string, 0, len(prefs.AdvertiseServices))
	for _, s := range prefs.AdvertiseServices {
		if _, ok := removeSet[s]; ok {
			continue
		}
		kept = append(kept, s)
	}

	if equalStringSlices(kept, prefs.AdvertiseServices) {
		return nil
	}

	_, err = lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		AdvertiseServicesSet: true,
		Prefs: ipn.Prefs{
			AdvertiseServices: kept,
		},
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	log.Info().Strs("services", kept).Msg("Updated AdvertiseServices (removed)")
	return nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
