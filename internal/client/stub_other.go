//go:build !linux

// This stub lets the module build on non-Linux hosts (e.g. the Mac dev box).
// The real client only compiles for Linux; this file never runs in production.
package client

import "errors"

// Client is a stub on non-Linux platforms.
type Client struct{}

// New returns an error: the client only builds for Linux.
func New(cfg Config) (*Client, error) {
	return nil, errors.New("edgehop-client only builds for Linux")
}

// Close is a no-op stub.
func (c *Client) Close() error { return nil }

// Run always fails on non-Linux.
func (c *Client) Run() error {
	return errors.New("edgehop-client only builds for Linux")
}
