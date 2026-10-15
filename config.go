package main

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Config struct{ Listen, Upstream, APIKey string }

var safeKey = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]+$`)

func validPort(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n > 0 && n <= 65535
}
func readConfig(getenv func(string) string) (Config, error) {
	key := getenv("SUI_GRPC_API_KEY")
	if len(key) > 4096 || !safeKey.MatchString(key) {
		return Config{}, errors.New("SUI_GRPC_API_KEY must be a nonempty token (letters, digits, . _ ~ + / = - only)")
	}
	listen := getenv("FORWARDER_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:29010"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil || !validPort(port) || net.ParseIP(host) == nil {
		return Config{}, errors.New("FORWARDER_LISTEN must be an explicit IP and port")
	}
	allow := getenv("FORWARDER_ALLOW_NON_LOOPBACK")
	if allow != "" && allow != "true" && allow != "false" {
		return Config{}, errors.New("FORWARDER_ALLOW_NON_LOOPBACK must be true or false")
	}
	if !net.ParseIP(host).IsLoopback() && allow != "true" {
		return Config{}, errors.New("non-loopback listening requires FORWARDER_ALLOW_NON_LOOPBACK=true; protect access to this credential-bearing proxy")
	}
	upstream := getenv("SUI_GRPC_UPSTREAM")
	if upstream == "" {
		upstream = "https://sui-mainnet-grpc.giverep.com:443"
	}
	u, err := url.Parse(upstream)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(upstream, "#") || strings.HasSuffix(u.Host, ":") || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "{}\\ \t\r\n") {
		return Config{}, errors.New("SUI_GRPC_UPSTREAM must be an HTTPS origin without credentials, path, query or fragment")
	}
	if u.Port() != "" && !validPort(u.Port()) {
		return Config{}, errors.New("invalid upstream port")
	}
	return Config{listen, upstream, key}, nil
}
