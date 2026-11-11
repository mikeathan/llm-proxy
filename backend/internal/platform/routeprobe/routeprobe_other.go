//go:build !linux

package routeprobe

import "context"

// Probe is not available here: an unprivileged socket cannot read the routers' answers outside Linux.
func Probe(context.Context, Request) (Reply, error) { return Reply{}, ErrUnsupported }
