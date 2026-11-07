// Package stacksdk (module llm-proxy-stacks/sdk, folder stacks/sdk) is the contract between the core and a stack (ADR-0032, Constitution IV.6). It holds interfaces, plain data
// types, the manifest and its validation, and nothing else: no feature logic, and no import of the service. A stack
// imports this package and the standard library and third-party libraries, never llm-proxy/internal/..., llm-proxy/models or
// another stack; its go.mod lets the compiler refuse most of that, and tests over `go list`, go.mod and the stack's source (backend/internal/archtest) enforce the rest.
//
// The shape is ports and adapters. A port is a small interface here, one per capability; the core supplies the adapters and hands
// them to a stack in a Host. Every request, result, event and configuration value that crosses a port is plain data: a struct of
// serializable fields (CheckPlainData; a test round-trips each one through JSON), so a stack could later run in another process
// without its callers changing. Two ports are capabilities rather than data and say so: Guard.DialContext returns a live
// connection, because there is no other way to open one, and it is the only way a stack may dial (Constitution I.1 to I.3).
//
// The sandbox is core (ADR-0033). The Isolation port has no field that relaxes it, and no manifest field declares a privilege.
package stacksdk
