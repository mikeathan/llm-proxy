package context

import "net/http"

type NodeHerderClient struct {
	baseUrl string
	client  *http.Client
}
