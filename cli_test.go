package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestNewCLI(t *testing.T) {
	t.Run("CLI creation", func(t *testing.T) {
		cmd := NewCLI(nil)

		assert.NotNil(t, cmd)
		assert.Equal(t, "tsgw", cmd.Name)
		assert.Equal(t, "1.0.0", cmd.Version)
		assert.Contains(t, cmd.Description, "Tailscale")
		assert.NotEmpty(t, cmd.Flags)
	})
}

func TestCLI_Flags(t *testing.T) {
	t.Run("required flags", func(t *testing.T) {
		cmd := NewCLI(nil)

		// Check that required flags are present
		flagNames := make(map[string]bool)
		for _, flag := range cmd.Flags {
			if f, ok := flag.(*cli.StringFlag); ok {
				flagNames[f.Name] = true
			}
			if f, ok := flag.(*cli.StringSliceFlag); ok {
				flagNames[f.Name] = true
			}
		}

		// Check for key required flags
		assert.True(t, flagNames["tsnet-dir"], "tsnet-dir flag should be present")
		assert.True(t, flagNames["route"], "route flag should be present")
		assert.True(t, flagNames["tailscale-domain"], "tailscale-domain flag should be present")
		assert.True(t, flagNames["oauth-client-id"], "oauth-client-id flag should be present")
		assert.True(t, flagNames["oauth-client-secret"], "oauth-client-secret flag should be present")
	})
}

func TestCLI_CommandStructure(t *testing.T) {
	t.Run("command structure", func(t *testing.T) {
		cmd := NewCLI(nil)

		assert.NotNil(t, cmd, "CLI should be created")
		assert.Empty(t, cmd.Commands, "CLI should not have sub-commands")
		// Action may be nil when no action is provided
	})
}
