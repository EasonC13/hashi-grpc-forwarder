package main

import (
	"flag"
	"fmt"
	"github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"io"
	"os"
	"os/signal"
	"syscall"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("hashi-grpc-forwarder", flag.ContinueOnError)
	flags.SetOutput(errOut)
	envFile := flags.String("env-file", "", "dotenv file (otherwise load .env if present); exported environment wins")
	check := flags.Bool("check", false, "validate configuration without opening a listener or contacting upstream")
	showVersion := flags.Bool("version", false, "print version")
	showLicenses := flags.Bool("licenses", false, "print embedded project and dependency licenses")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, "unexpected arguments; use --help")
		return 2
	}
	if *showLicenses {
		fmt.Fprintln(out, wrapperLicense, distributionNotice, dependencyLicenses)
		return 0
	}
	if *showVersion {
		fmt.Fprintln(out, "hashi-grpc-forwarder", version)
		return 0
	}
	if *envFile != "" {
		if err := loadEnvFile(*envFile); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
	} else {
		if _, err := os.Stat(".env"); err == nil {
			if err := loadEnvFile(".env"); err != nil {
				fmt.Fprintln(errOut, err)
				return 1
			}
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(errOut, "cannot inspect .env (details suppressed)")
			return 1
		}
	}
	config, err := readConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if *check {
		fmt.Fprintln(out, "configuration valid (no connectivity check)")
		return 0
	}
	data, err := buildCaddyConfig(config)
	if err != nil {
		fmt.Fprintln(errOut, "could not construct proxy configuration")
		return 1
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	if err := caddy.Load(data, true); err != nil {
		fmt.Fprintln(errOut, "proxy startup failed; check listener availability and configuration (details suppressed to protect credentials)")
		return 1
	}
	fmt.Fprintln(out, "gRPC forwarder listening on", config.Listen, "(upstream TLS verified; admin and config persistence disabled)")
	<-stop
	if err := caddy.Stop(); err != nil {
		fmt.Fprintln(errOut, "proxy shutdown failed")
		return 1
	}
	fmt.Fprintln(out, "gRPC forwarder stopped")
	return 0
}
