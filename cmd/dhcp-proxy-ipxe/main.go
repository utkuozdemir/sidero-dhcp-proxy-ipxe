// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main implements the main entrypoint for the server.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/server"
	"github.com/utkuozdemir/dhcp-proxy-ipxe/internal/version"
)

var (
	serverOptions = server.DefaultOptions()
	debug         bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     version.Name,
	Short:   "Run the server",
	Version: version.Tag,
	Args:    cobra.NoArgs,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true // if the args are parsed fine, no need to show usage
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger, err := initLogger()
		if err != nil {
			return fmt.Errorf("failed to create logger: %w", err)
		}

		defer logger.Sync() //nolint:errcheck

		return run(cmd.Context(), logger)
	},
}

func initLogger() (*zap.Logger, error) {
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	if !debug {
		loggerConfig.Level.SetLevel(zap.InfoLevel)
	}

	return loggerConfig.Build(zap.AddStacktrace(zapcore.FatalLevel)) // only print stack traces for fatal errors)
}

func run(ctx context.Context, logger *zap.Logger) error {
	prov := server.New(serverOptions, logger)

	if err := prov.Run(ctx); err != nil {
		return fmt.Errorf("failed to run server: %w", err)
	}

	return nil
}

func main() {
	if err := runCmd(); err != nil {
		log.Fatalf("failed to run: %v", err)
	}
}

func runCmd() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode & logs.")

	rootCmd.Flags().StringVar(&serverOptions.APIListenAddress, "api-listen-address", serverOptions.APIListenAddress,
		"The IP address to listen on. If not specified, the server will listen on all interfaces.")
	rootCmd.Flags().StringVar(&serverOptions.APIAdvertiseAddress, "api-advertise-address", serverOptions.APIAdvertiseAddress,
		"The IP address to advertise. Required if the server has more than a single routable IP address. If not specified, the single routable IP address will be used.")
	rootCmd.Flags().IntVar(&serverOptions.APIPort, "api-port", serverOptions.APIPort, "The port to run the api server on.")
	rootCmd.Flags().StringVar(&serverOptions.DHCPProxyIfaceOrIP, "dhcp-proxy-iface-or-ip", serverOptions.DHCPProxyIfaceOrIP,
		"The interface name or the IP address on the interface to run the DHCP proxy server on. "+
			"If it is an IP address, the DHCP proxy server will run on the interface that has the IP address. "+
			"If not specified, defaults to the API advertise address.")
	rootCmd.Flags().StringVar(&serverOptions.ImageFactoryBaseURL, "image-factory-base-url", serverOptions.ImageFactoryBaseURL,
		"The base URL of the image factory.")
	rootCmd.Flags().StringVar(&serverOptions.ImageFactoryPXEBaseURL, "image-factory-pxe-base-url", serverOptions.ImageFactoryPXEBaseURL,
		"The base URL of the image factory PXE server.")
	rootCmd.Flags().BoolVar(&serverOptions.SecureBootEnabled, "secure-boot-enabled", serverOptions.SecureBootEnabled,
		"Serve secure boot UKI from the iPXE endpoint. The UKI can be used to boot a machine without secure boot, but it is required to boot a machine with secure boot.",
	)

	rootCmd.Flags().BoolVar(&serverOptions.DisableDHCPProxy, "disable-dhcp-proxy", serverOptions.DisableDHCPProxy,
		"Disable the DHCP proxy server.")

	rootCmd.Flags().StringSliceVar(&serverOptions.Extensions, "extensions", serverOptions.Extensions,
		"List of Talos extensions to use. The extensions will be used to generate schematic ID from the image factory.")
	rootCmd.Flags().StringSliceVar(&serverOptions.ExtraKernelArgs, "extra-kernel-args", serverOptions.ExtraKernelArgs,
		"List of extra kernel arguments to use. The arguments will be used to generate schematic ID from the image factory.")

	rootCmd.Flags().StringVar(&serverOptions.TalosVersion, "talos-version", serverOptions.TalosVersion, "The Talos version to use.")

	// machine config options

	rootCmd.Flags().StringVar(&serverOptions.MachineConfig.OmniSiderolinkAPIURL, "omni-siderolink-api-url", serverOptions.MachineConfig.OmniSiderolinkAPIURL,
		"The Omni Siderolink API URL to use in the machine config, e.g., \"https://<YOUR-INSTANCE>.siderolink.omni.siderolabs.io?jointoken=<YOUR-JOIN-TOKEN>\". "+
			"This can be specified to connect the machine to an Omni instance.")
	rootCmd.Flags().IntVar(&serverOptions.MachineConfig.OmniEventsPort, "omni-events-port", serverOptions.MachineConfig.OmniEventsPort,
		"The port to use for the Omni events sink. This is required for Omni to receive events from the machine. No-op if the Omni Siderolink API URL is not specified.")
	rootCmd.Flags().IntVar(&serverOptions.MachineConfig.OmniKmsgLogPort, "omni-kmsg-log-port", serverOptions.MachineConfig.OmniKmsgLogPort,
		"The port to use for the Omni kmsg log. This is required for Omni to receive kernel messages from the machine. No-op if the Omni Siderolink API URL is not specified.")
}
