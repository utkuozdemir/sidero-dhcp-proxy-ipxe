// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package server implements the DHCP + iPXE server servers.
package server

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/config"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/dhcp"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/imagefactory"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/ip"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/ipxe"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/machineconfig"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/server"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server/tftp"
)

// Server implements the server.
type Server struct {
	logger *zap.Logger

	options Options
}

// New creates a new Server.
func New(options Options, logger *zap.Logger) *Server {
	return &Server{
		options: options,
		logger:  logger,
	}
}

// Run runs the server.
//
//nolint:gocyclo,cyclop
func (s *Server) Run(ctx context.Context) error {
	apiAdvertiseAddress, err := s.determineAPIAdvertiseAddress()
	if err != nil {
		return fmt.Errorf("failed to determine API advertise address: %w", err)
	}

	dhcpProxyIfaceOrIP := s.options.DHCPProxyIfaceOrIP
	if dhcpProxyIfaceOrIP == "" {
		dhcpProxyIfaceOrIP = apiAdvertiseAddress
	}

	configServerEnabled := s.options.MachineConfig.OmniSiderolinkAPIURL != ""

	s.logger.Info("starting server",
		zap.Any("options", s.options),
		zap.String("api_advertise_address", apiAdvertiseAddress),
		zap.String("dhcp_proxy_iface_or_ip", dhcpProxyIfaceOrIP),
	)

	var configHandler http.Handler

	if configServerEnabled {
		var machineConfig []byte

		if machineConfig, err = machineconfig.Build(s.options.MachineConfig); err != nil {
			return fmt.Errorf("failed to build machine config: %w", err)
		}

		if configHandler, err = config.NewHandler(machineConfig, s.logger.With(zap.String("component", "config_handler"))); err != nil {
			return fmt.Errorf("failed to create config handler: %w", err)
		}
	}

	imageFactoryClient, err := imagefactory.NewClient(s.options.ImageFactoryBaseURL, s.options.ImageFactoryPXEBaseURL,
		s.options.SecureBootEnabled, s.logger.With(zap.String("component", "image_factory_client")))
	if err != nil {
		return fmt.Errorf("failed to create image factory client: %w", err)
	}

	ipxeHandler, err := ipxe.NewHandler(configServerEnabled, imageFactoryClient, ipxe.HandlerOptions{
		APIAdvertiseAddress: apiAdvertiseAddress,
		APIPort:             s.options.APIPort,
		Extensions:          s.options.Extensions,
		ExtraKernelArgs:     s.options.ExtraKernelArgs,
		TalosVersion:        s.options.TalosVersion,
	}, s.logger.With(zap.String("component", "ipxe_handler")))
	if err != nil {
		return fmt.Errorf("failed to create iPXE handler: %w", err)
	}

	tftpServer := tftp.NewServer(s.options.APIListenAddress, s.logger.With(zap.String("component", "tftp_server")))
	srvr := server.New(ctx, s.options.APIListenAddress, s.options.APIPort, configHandler, ipxeHandler, s.logger.With(zap.String("component", "server")))

	components := []component{
		{srvr.Run, "server"},
		{tftpServer.Run, "TFTP server"},
	}

	if !s.options.DisableDHCPProxy {
		dhcpProxy := dhcp.NewProxy(apiAdvertiseAddress, s.options.APIPort, dhcpProxyIfaceOrIP, s.logger.With(zap.String("component", "dhcp_proxy")))

		components = append(components, component{dhcpProxy.Run, "DHCP proxy"})
	}

	return s.runComponents(ctx, components)
}

type component struct {
	run  func(context.Context) error
	name string
}

// runComponents runs the long-running components in their own goroutines.
//
// It will terminate all components when one of them terminates, irrespective of whether it terminates with an error.
func (s *Server) runComponents(ctx context.Context, components []component) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)

	for _, comp := range components {
		logger := s.logger.With(zap.String("component", comp.name))

		eg.Go(func() error {
			defer cancel() // cancel the parent context, so all other components are also stopped even if this one does not return an error

			logger.Info("start component")

			if err := comp.run(ctx); err != nil {
				logger.Error("failed to run component", zap.Error(err))

				return err
			}

			logger.Info("component stopped")

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to run components: %w", err)
	}

	return nil
}

func (s *Server) determineAPIAdvertiseAddress() (string, error) {
	if s.options.APIAdvertiseAddress != "" {
		return s.options.APIAdvertiseAddress, nil
	}

	routableIPs, err := ip.RoutableIPs()
	if err != nil {
		return "", fmt.Errorf("failed to get routable IPs: %w", err)
	}

	if len(routableIPs) != 1 {
		return "", fmt.Errorf(`expected exactly one routable IP, got %d: %v. specify API advertise address explicitly`, len(routableIPs), routableIPs)
	}

	return routableIPs[0], nil
}
