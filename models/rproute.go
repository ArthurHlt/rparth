package models

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/goccy/go-yaml"
)

type RPRoutes []*RPRoute

var ErrNoRoute = errors.New("no route found")

func (rts *RPRoutes) FindRoute(req *http.Request) (*RPRoute, error) {
	for _, route := range *rts {
		if route.Match(req) {
			return route, nil
		}
	}
	return nil, ErrNoRoute
}

func (rts *RPRoutes) UnmarshalYAML(data []byte) error {
	var routes []*RPRoute
	dec := yaml.NewDecoder(bytes.NewReader(data),
		yaml.CustomUnmarshaler[url.URL](unmarshalURL),
	)
	err := dec.Decode(&routes)
	if err != nil {
		return err
	}
	*rts = routes
	return rts.Validate()
}

func (rts *RPRoutes) Validate() error {
	existing := make(map[string]struct{})
	for _, route := range *rts {
		if _, ok := existing[route.Name]; ok {
			return errors.New("duplicate route name: " + route.Name)
		}
		existing[route.Name] = struct{}{}
		if err := route.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalURL(u *url.URL, data []byte) error {
	var s string
	if err := yaml.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("parse URL %q: %w", s, err)
	}
	*u = *parsed
	return nil
}

// RPRoute represents a route to be proxied
type RPRoute struct {
	Name string `yaml:"name"`
	// Host is the incoming request host used for matching, e.g. "api.example.com"
	// this is optional, if not set, all hosts will match
	Host string `yaml:"host"`
	// Prefix to match against, defaults to "/"
	Prefix string `yaml:"prefix"`
	// Target is the upstream URL to proxy matched requests to, e.g. "http://backend:8080"
	// YAML decoding for *url.URL is wired in config.ReadConfig via
	// yaml.CustomUnmarshaler[url.URL].
	Target *url.URL `yaml:"target"`
	// StripPrefix removes the matched prefix from the upstream request path, defaults to true
	StripPrefix *bool `yaml:"strip_prefix"`
	// Headers to be added to the upstream request
	// e.g. map[string][]string{"X-Foo": {"bar"}}
	// this is optional, if not set, no headers will be added
	Headers map[string][]string `yaml:"headers"`
	// Timeout in seconds, defaults to 30
	// I use uint here to avoid the need to check for below zero values
	Timeout uint `yaml:"timeout"`
}

func (r *RPRoute) Validate() error {
	if r.Name == "" {
		return errors.New("route name cannot be empty")
	}
	if r.Target == nil {
		return errors.New("route target cannot be empty")
	}
	if r.Prefix == "" {
		r.Prefix = "/"
	}
	if r.Timeout == 0 {
		r.Timeout = 30
	}
	if r.StripPrefix == nil {
		r.StripPrefix = new(true)
	}
	for k, v := range r.Headers {
		// canonicalize header name to ensure this is valid header
		r.Headers[textproto.CanonicalMIMEHeaderKey(k)] = v
	}
	return nil
}

func (r *RPRoute) Match(req *http.Request) bool {
	if !r.matchHost(req.Host) {
		return false
	}
	if req.URL.Path == "" || !strings.HasPrefix(req.URL.Path, r.Prefix) {
		return false
	}
	return true
}

func (r *RPRoute) matchHost(host string) bool {
	if r.Host == "" {
		return true
	}
	if host == "" {
		return false
	}
	reqHost, _, err := net.SplitHostPort(host)
	// if there is an error we assume that there is no port in the host
	// so we can safely compare the host
	if err != nil {
		reqHost = host
	}
	return reqHost == r.Host
}

func (r *RPRoute) String() string {
	return fmt.Sprintf("%s -> %s", r.Name, r.Target)
}
