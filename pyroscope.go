package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/pyroscope-go"
	"github.com/rs/zerolog/log"
)

type Pyroscope struct {
	profiler *pyroscope.Profiler
}

type zerologPyroscopeLogger struct{}

func (zerologPyroscopeLogger) Infof(format string, args ...interface{}) {
	log.Info().Msgf(format, args...)
}

func (zerologPyroscopeLogger) Debugf(format string, args ...interface{}) {
	log.Debug().Msgf(format, args...)
}

func (zerologPyroscopeLogger) Errorf(format string, args ...interface{}) {
	log.Error().Msgf(format, args...)
}

func SetupPyroscope(_ context.Context, config *Config) (*Pyroscope, error) {
	if config == nil || !config.Pyroscope.Enabled {
		log.Info().Msg("Pyroscope is disabled")
		return &Pyroscope{profiler: nil}, nil
	}

	if strings.TrimSpace(config.Pyroscope.ServerAddress) == "" {
		return nil, fmt.Errorf("pyroscope-server-address is required when Pyroscope is enabled")
	}

	appName := strings.TrimSpace(config.Pyroscope.ApplicationName)
	if appName == "" {
		appName = "tsgw"
	}

	profileTypes, err := parsePyroscopeProfileTypes(config.Pyroscope.ProfileTypes)
	if err != nil {
		return nil, err
	}

	pyroCfg := pyroscope.Config{
		ApplicationName:   appName,
		ServerAddress:     config.Pyroscope.ServerAddress,
		AuthToken:         config.Pyroscope.AuthToken,
		BasicAuthUser:     config.Pyroscope.BasicAuthUser,
		BasicAuthPassword: config.Pyroscope.BasicAuthPass,
		TenantID:          config.Pyroscope.TenantID,
		Tags:              config.Pyroscope.Tags,
		HTTPHeaders:       config.Pyroscope.HTTPHeaders,
		DisableGCRuns:     config.Pyroscope.DisableGCRuns,
		Logger:            zerologPyroscopeLogger{},
	}
	if config.Pyroscope.UploadRate > 0 {
		pyroCfg.UploadRate = config.Pyroscope.UploadRate
	}
	if len(profileTypes) > 0 {
		pyroCfg.ProfileTypes = profileTypes
	}

	log.Info().
		Str("application_name", pyroCfg.ApplicationName).
		Str("server_address", pyroCfg.ServerAddress).
		Str("tenant_id", config.Pyroscope.TenantID).
		Bool("basic_auth", config.Pyroscope.BasicAuthUser != "" || config.Pyroscope.BasicAuthPass != "").
		Bool("auth_token", config.Pyroscope.AuthToken != "").
		Int("tags", len(config.Pyroscope.Tags)).
		Int("headers", len(config.Pyroscope.HTTPHeaders)).
		Msg("Starting Pyroscope profiler")

	profiler, err := pyroscope.Start(pyroCfg)
	if err != nil {
		return nil, fmt.Errorf("pyroscope start: %w", err)
	}

	log.Info().Msg("Pyroscope setup completed")
	return &Pyroscope{profiler: profiler}, nil
}

func (p *Pyroscope) Shutdown(_ context.Context) error {
	if p == nil || p.profiler == nil {
		return nil
	}
	return p.profiler.Stop()
}

func parsePyroscopeProfileTypes(in []string) ([]pyroscope.ProfileType, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]pyroscope.ProfileType, 0, len(in))
	for _, raw := range in {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		switch pyroscope.ProfileType(v) {
		case pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration:
			out = append(out, pyroscope.ProfileType(v))
		default:
			return nil, fmt.Errorf("invalid pyroscope profile type: %q", raw)
		}
	}
	return out, nil
}
