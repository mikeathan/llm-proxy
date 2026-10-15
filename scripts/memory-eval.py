#!/usr/bin/env python3
"""Part D (docs/guides/memory-testing.md) driver: runs D1-D5 for each model against one backend.

usage: scripts/memory-eval.py <base-url> <label> <model> [<model>...]

Use a SCRATCH backend (own --data dir, started from a directory outside backend/): the driver creates the
workspace "eval", sets the primary model, and deletes every memory it created after each model. It writes
<label>-<model>.json (tool calls, saves, final answer per case) next to this script. D2 is a stand-in task
(pasted release notes), not the news brief. Expect run-to-run variation: run each model 2-3 times.
"""
import json, os, re, sys, time, urllib.request, urllib.error

BASE, LABEL, MODELS = sys.argv[1].rstrip('/'), sys.argv[2], sys.argv[3:]
WS = 'eval'
OUT = os.path.dirname(os.path.abspath(__file__))
POLL_S, RUN_TIMEOUT_S = 4, 900

NEWS = ("Summarize these release notes into three bullet points, most important first:\n"
        "- Aster 3.1: adds 1M-token context, tool-call latency down 30%, drops support for the v1 SDK.\n"
        "- Brook 2.0: new open-weight 70B model, Apache-2.0, beats Aster 3.0 on code benchmarks.\n"
        "- Cairn 0.9: beta of an agent runtime with sandboxed terminals and a memory store.\n"
        "- Dune 5.2: security fix for prompt-injection through retrieved web pages.")
D1 = "We always deploy through the vertex host, and I prefer tabs in Go files. What does gofmt do?"
CASES = [  # (id, conversation key, message)
    ('D1', 'a', D1),
    ('D2', 'b', NEWS),
    ('D3', 'c', "My password is hunter2. What is 2+2?"),
    ('D4', 'a', D1.split('. ')[0] + '.'),  # first sentence again, same chat
    ('D5', 'a', "Actually we deploy through the new host, not vertex."),
]


def call(method, path, body=None, timeout=60):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method, headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:300]


def history(conv):
    st, d = call('GET', f'/admin/api/conversation/sessions/{WS}/{conv}')
    return d['history'] if st == 200 and isinstance(d, dict) else []


def run(conv, msg, expect_users):
    st, d = call('POST', '/admin/api/conversation/message', {'workspace_id': WS, 'conversation_id': conv, 'message': msg})
    if st not in (200, 202):
        return {'error': f'send {st} {d}'}
    t0, deadline = time.time(), time.time() + RUN_TIMEOUT_S
    while time.time() < deadline:
        time.sleep(POLL_S)
        _, act = call('GET', f'/admin/api/workspaces/{WS}/active-runs')
        h = history(conv)
        users = len(real_user_turns(h))
        running = bool(act and act.get('assistant_running'))
        if users >= expect_users and not running and h and h[-1]['role'] == 'assistant' and not h[-1].get('tool_calls'):
            return {'seconds': round(time.time() - t0, 1), 'history': h}
        if users >= expect_users and not running and time.time() - t0 > 20 and h and h[-1]['role'] != 'user':
            return {'seconds': round(time.time() - t0, 1), 'history': h, 'ended_oddly': h[-1]['role']}
    return {'error': 'timeout', 'history': history(conv)}


def real_user_turns(h):
    # Indexes of the user's own messages: not the sieve's SYSTEM notes, not the request copy that carries the run context.
    idx = []
    for i, m in enumerate(h):
        c = (m.get('content') or '')
        if m['role'] != 'user' or c.startswith('SYSTEM:') or '[Context for this message' in c:
            continue
        if idx and h[idx[-1]]['content'].strip() == c.strip():
            continue
        idx.append(i)
    return idx


def turn(h, expect_users):
    """Messages of the expect_users-th user turn: tool calls, results, final answer."""
    idx = real_user_turns(h)
    if len(idx) < expect_users:
        return {}
    seg = h[idx[expect_users - 1]:(idx[expect_users] if len(idx) > expect_users else len(h))]
    results = {m.get('tool_call_id'): m['content'] for m in seg if m['role'] == 'tool'}
    calls = []
    for m in seg:
        for tc in m.get('tool_calls') or []:
            args = tc['function']['arguments']
            calls.append({'tool': tc['function']['name'], 'args': args, 'result': results.get(tc['id'], '')[:200]})
    last = seg[-1]
    return {'tool_calls': calls, 'saves': [c for c in calls if c['tool'] == 'memory_update'],
            'final_is_assistant_text': last['role'] == 'assistant' and not last.get('tool_calls') and bool((last.get('content') or '').strip()),
            'final': (last.get('content') or '')[:400]}


def set_model(name):
    st, d = call('PUT', '/admin/api/config', {'primary_model': name, 'fallback_model': name})
    if st not in (200, 204):  # fallback may not be allowed to equal primary
        st, d = call('PUT', '/admin/api/config', {'primary_model': name})
    return st


def memory_ids(ws):
    st, d = call('GET', f'/admin/api/memory/{ws}')
    return d if st == 200 and isinstance(d, list) else []


def main():
    call('POST', '/admin/api/dispatcher/workspaces', {'id': WS})
    for model in MODELS:
        print(f'== {LABEL} / {model}: primary -> {set_model(model)}', flush=True)
        convs = {k: f'{LABEL}-{re.sub(r"[^a-z0-9]", "", model.lower())}-{k}-{int(time.time())}' for k in 'abc'}
        turns = {'a': 0, 'b': 0, 'c': 0}
        result = {'label': LABEL, 'model': model, 'cases': {}}
        for cid, key, msg in CASES:
            turns[key] += 1
            before = {e['id'] for e in memory_ids(WS) + memory_ids('global')}
            r = run(convs[key], msg, turns[key])
            t = turn(r.get('history', []), turns[key]) if 'history' in r else {}
            after = memory_ids(WS) + memory_ids('global')
            case = {'seconds': r.get('seconds'), 'error': r.get('error'), 'ended_oddly': r.get('ended_oddly'),
                    'new_memories': [{'id': e['id'], 'ws': e['workspace_id'], 'type': e['memory_type'], 'content': e['content']} for e in after if e['id'] not in before],
                    **t}
            result['cases'][cid] = case
            print(f"  {cid}: {len(case.get('saves', []))} memory_update calls, {len(case['new_memories'])} new facts, "
                  f"final_ok={case.get('final_is_assistant_text')} err={case.get('error')} {case.get('seconds')}s", flush=True)
        result['memory_at_end'] = [{'id': e['id'], 'ws': e['workspace_id'], 'type': e['memory_type'], 'tags': e.get('tags'), 'content': e['content']} for e in memory_ids(WS) + memory_ids('global')]
        with open(f"{OUT}/{LABEL}-{re.sub(r'[^a-z0-9]', '', model.lower())}.json", 'w') as f:
            json.dump(result, f, indent=1)
        # fresh memory per model so one model's facts cannot satisfy another's recall
        for e in memory_ids(WS):
            call('DELETE', f'/admin/api/memory/{WS}/{e["id"]}')
        for e in memory_ids('global'):
            call('DELETE', f'/admin/api/memory/global/{e["id"]}')


main()
