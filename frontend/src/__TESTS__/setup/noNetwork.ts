// Component tests run under happy-dom, whose fetch dials the page origin (localhost:3000) for real. A component that polls (the brief dot,
// the stats panel) and a test that did not mock its service then made a refused connection each time, and every refusal printed an
// AggregateError: about 2,800 lines in a full run. Here an un-mocked fetch fails at once, without the network, with an error that names the
// address. A test that mocks fetch (vi.spyOn, vi.stubGlobal) or its service still replaces this.
globalThis.fetch = (input: RequestInfo | URL): Promise<Response> => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  return Promise.reject(new TypeError(`fetch is not mocked in this test: ${url}`))
}
