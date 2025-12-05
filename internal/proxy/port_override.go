package proxy

import "llm-proxy/utils"

var portReadyFunc = utils.PortReady

// Allow tests to override
func SetPortReady(fn func(int) bool) func() {
    orig := portReadyFunc
    portReadyFunc = fn
    return func() { portReadyFunc = orig }
}