package proxy


import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func NewReverseProxy(target string) *httputil.ReverseProxy {
	u, _ := url.Parse(target)

	proxy := httputil.NewSingleHostReverseProxy(u)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
	}

	return proxy
}
