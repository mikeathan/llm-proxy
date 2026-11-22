package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	stacksdk "llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/models"
)

type noFetch struct{}

func (noFetch) Get(context.Context, stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	return stacksdk.FetchResponse{}, errors.New("no network in this test")
}

type noModel struct{}

func (noModel) Complete(context.Context, stacksdk.ModelRequest) (stacksdk.ModelResponse, error) {
	return stacksdk.ModelResponse{}, errors.New("no model in this test")
}

func newNewsroomToolsRig(t *testing.T) (*toolregistry.LocalToolRegistry, *memNewsroom) {
	t.Helper()
	st := newsroomStack()
	policy := &memNewsroom{}
	host, err := newStackHost(st, stacksdk.Host{
		Config:   stackport.Config[models.NewsroomConfig]{Store: policy},
		Log:      stackport.Log{Stack: "newsroom"},
		DataDir:  stackport.DataDir{Dir: t.TempDir(), Files: st.Manifest().Storage},
		Fetch:    noFetch{},
		Model:    noModel{},
		Notice:   stackport.Notice{},
		Schedule: stackport.Schedule{Parse: func(expr string) (interface{ Next(time.Time) time.Time }, error) { return automation.ParseCron(expr) }},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	host.Run(ctx)
	t.Cleanup(func() { cancel(); _ = st.Stop(context.Background()) })
	return toolregistry.NewLocalToolRegistry(nil, nil, nil, nil, nil, nil).Use(newsroomToolModule(host)), policy
}

func listedNames(t *testing.T, r *toolregistry.LocalToolRegistry, ctx context.Context) []string {
	t.Helper()
	list, err := r.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list {
		names = append(names, tool.Function.Name)
	}
	return names
}

func call(name, args string) proxy.ToolCall {
	return proxy.ToolCall{Function: proxy.FunctionCall{Name: name, Arguments: args}}
}

// The core names the tools (models) and the stack declares them; the two must not drift, or the assistant would call a tool the stack does not have.
func TestNewsroomTools_TheCoreNamesAreTheNamesTheStackDeclares(t *testing.T) {
	var declared []string
	for _, tool := range newsroomStack().Manifest().Tools {
		declared = append(declared, tool.Name)
	}
	slices.Sort(declared)
	want := []string{models.ToolNewsroomFollow, models.ToolNewsroomWatches}
	slices.Sort(want)
	if !slices.Equal(declared, want) {
		t.Errorf("the stack declares %v, the core names %v", declared, want)
	}
}

func TestNewsroomTools_ChatFollowsAWatchFeedOnlyAndListsIt(t *testing.T) {
	r, _ := newNewsroomToolsRig(t)
	ctx := context.Background()
	names := listedNames(t, r, ctx)
	for _, want := range []string{models.ToolNewsroomFollow, models.ToolNewsroomWatches} {
		if !slices.Contains(names, want) {
			t.Fatalf("%s is not offered to an attended chat: %v", want, names)
		}
	}
	out, err := r.ExecuteTool(ctx, call(models.ToolNewsroomFollow, `{"name":"Go releases","sources":[{"kind":"feed","target":"https://go.dev/blog/feed.atom"}]}`))
	if err != nil || !strings.Contains(out.(string), "Go releases") {
		t.Fatalf("follow = %v, %v", out, err)
	}
	if strings.Contains(out.(string), `"channel":"`) && !strings.Contains(out.(string), `"channel":""`) {
		t.Errorf("a watch made in chat must have no channel: %v", out)
	}
	list, err := r.ExecuteTool(ctx, call(models.ToolNewsroomWatches, ``))
	if err != nil || !strings.Contains(list.(string), "Go releases") {
		t.Errorf("watches = %v, %v", list, err)
	}
	if _, err := r.ExecuteTool(ctx, call(models.ToolNewsroomFollow, `{"name":"","sources":[]}`)); err == nil {
		t.Error("a watch with no name and no source must be refused in the stack's words")
	}
}

// An unattended run may be steered by what it read, and a watch makes the service read from then on, so the tools are hidden and refused there.
func TestNewsroomTools_AnUnattendedRunNeitherSeesNorCallsThem(t *testing.T) {
	r, _ := newNewsroomToolsRig(t)
	unattended := models.WithUnattendedRun(context.Background())
	if names := listedNames(t, r, unattended); slices.Contains(names, models.ToolNewsroomFollow) || slices.Contains(names, models.ToolNewsroomWatches) {
		t.Errorf("offered to an unattended run: %v", names)
	}
	if _, err := r.ExecuteTool(unattended, call(models.ToolNewsroomFollow, `{"name":"x","sources":[{"kind":"page","target":"https://example.com"}]}`)); err == nil {
		t.Error("an unattended run created a watch")
	}
}

func TestNewsroomTools_AreHiddenAndRefusedWhileTheModuleIsOff(t *testing.T) {
	r, policy := newNewsroomToolsRig(t)
	_ = policy.Update(func(c *models.NewsroomConfig) error { c.Disabled = true; return nil })
	ctx := context.Background()
	if slices.Contains(listedNames(t, r, ctx), models.ToolNewsroomFollow) {
		t.Error("offered while the module is off")
	}
	if _, err := r.ExecuteTool(ctx, call(models.ToolNewsroomWatches, ``)); err == nil {
		t.Error("answered while the module is off")
	}
}
