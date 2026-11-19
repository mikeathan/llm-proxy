package mcpclient

import "net/http"

// httpClient is the guarded client the stack was given; the alias keeps the connection code short.
type httpClient = *http.Client
