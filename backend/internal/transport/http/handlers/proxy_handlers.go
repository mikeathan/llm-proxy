package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"llm-proxy/internal/platform/network"
	"llm-proxy/models"
)

type ProxyHandlers struct {
	runtime RuntimeService
}

func NewProxyHandlers(runtime RuntimeService) *ProxyHandlers {
	return &ProxyHandlers{runtime: runtime}
}

var reverseProxyFactory = func(target string) http.Handler {
	return NewReverseProxy(target)
}

func SetReverseProxyFactory(f func(string) http.Handler) func() {
	orig := reverseProxyFactory
	reverseProxyFactory = f
	return func() {
		reverseProxyFactory = orig
	}
}

// OpenAI-compatible /v1/models response types (DTOs at the edge — the wire
// shape mirrors llama.cpp so external OpenAI-format clients parse it without
// client-side changes; adding a capability later = adding a struct field).
type openAIModelMeta struct {
	NctxTrain     int `json:"n_ctx_train,omitempty"`
	Nctx          int `json:"n_ctx,omitempty"`
	ContextLength int `json:"context_length,omitempty"`
	MaxTokens     int `json:"max_tokens,omitempty"`
}

type openAIModelEntry struct {
	ID                  string          `json:"id"`
	Object              string          `json:"object"`
	Created             int64           `json:"created"`
	OwnedBy             string          `json:"owned_by"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Meta                openAIModelMeta `json:"meta"`
}

type openAIModelList struct {
	Object string             `json:"object"`
	Data   []openAIModelEntry `json:"data"`
}

// ModelsListHandler serves the OpenAI-compatible GET /v1/models listing for the
// models this proxy manages. An external OpenAI-format client (e.g. the
// assistant UI's "openai" provider pointed at this proxy instead of at
// llama-server directly) discovers the same metadata a llama.cpp server would
// publish, so metadata keeps being "calculated" from the real model:
//   - owned_by "llamacpp" + meta.n_ctx_train keep the client's local-workload
//     fingerprint (provider_openai_compatible.listingServesLocalWorkload);
//   - meta.n_ctx / context_length is the REAL serving window (the model's
//     launch --ctx-size, or Metadata.Nctx when a /slots probe recorded it) —
//     the client's budget keys on the serving context, not n_ctx_train;
//   - max_tokens / max_completion_tokens publish the output cap.
//
// The ids are the registry names, which is also what the chat proxy routes on
// (EnsureModelProxyHandler), so a client must use these names in requests.
func (h *ProxyHandlers) ModelsListHandler(w http.ResponseWriter, r *http.Request) {
	managed := h.runtime.ListModels()
	list := openAIModelList{Object: "list", Data: make([]openAIModelEntry, 0, len(managed))}
	for _, m := range managed {
		meta := openAIModelMeta{}

		trainCtx := m.MaxTokens
		if m.Metadata != nil && m.Metadata.ContextLength > 0 {
			trainCtx = m.Metadata.ContextLength
		}
		if trainCtx > 0 {
			meta.NctxTrain = trainCtx
		}

		servingCtx := 0
		if m.Metadata != nil && m.Metadata.Nctx > 0 {
			servingCtx = m.Metadata.Nctx
		} else if ctx := servingCtxFromArgs(m.Args); ctx > 0 {
			servingCtx = ctx
		}
		if servingCtx > 0 {
			meta.Nctx = servingCtx
			meta.ContextLength = servingCtx
		} else if trainCtx > 0 {
			meta.Nctx = trainCtx
			meta.ContextLength = trainCtx
		}

		entry := openAIModelEntry{
			ID:      m.Name,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "llamacpp",
			Meta:    meta,
		}
		if m.MaxTokens > 0 {
			entry.MaxTokens = m.MaxTokens
			entry.MaxCompletionTokens = m.MaxTokens
			meta.MaxTokens = m.MaxTokens
		}
		list.Data = append(list.Data, entry)
	}
	respondJSON(w, list)
}

// servingCtxFromArgs extracts the serving context window from a local model's
// launch args (--ctx-size N / -c N). Returns 0 when absent.
func servingCtxFromArgs(args []string) int {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ctx-size", "--context-size", "-c":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

func peekModelName(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return ""
	}
	return payload.Model
}

func (h *ProxyHandlers) EnsureModelProxyHandler(w http.ResponseWriter, r *http.Request) {
	model := r.Header.Get("X-Model-Name")
	if model == "" {
		model = peekModelName(r)
	}
	if model == "" {
		primary, _ := h.runtime.SelectModels()
		if primary == "" {
			http.Error(w, "missing model name and no default configured", http.StatusBadRequest)
			return
		}
		model = primary
	}

	mi, err := h.runtime.EnsureModel(r.Context(), model)
	if err == models.ErrModelStarting {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-LLM-Status", "starting")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"starting"}`))
		return
	}

	if err != nil {
		http.Error(w, "model error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.runtime.RecordActivity(model)

	target := mi.URL
	isCloud := true
	if target == "" {
		target = network.FormatURL(mi.Host, mi.Port)
		isCloud = false
	}

	rp := reverseProxyFactory(target)

	// If we have custom headers (e.g. for cloud providers), we need to inject them.
	inner := rp
	rp = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For cloud providers, the mi.URL now contains the FULL endpoint (e.g. .../v1/chat/completions).
		// We must override the incoming path (/v1/chat/completions) with the target's path.
		if isCloud {
			// For cloud providers, the target already contains the full path (e.g., /v1/chat/completions).
			// We MUST clear the request path so the ReverseProxy doesn't double it.
			r.URL.Path = ""
			r.URL.RawPath = ""
		}

		for k, vv := range mi.Headers {
			for _, v := range vv {
				r.Header.Set(k, v)
			}
		}
		inner.ServeHTTP(w, r)
	})

	rp.ServeHTTP(w, r)
}

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
