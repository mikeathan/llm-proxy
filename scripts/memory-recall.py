#!/usr/bin/env python3
"""Recall test (R1-R6): seeds facts through the memory API, then asks fresh-conversation questions that need them.

usage: scripts/memory-recall.py <base-url> <label> <model>

Use a SCRATCH backend (own --data dir, started from a directory outside backend/), as for scripts/memory-eval.py. The
script creates workspaces "eval" and "eval2", sets the primary model, deletes the memories it seeded (and any the model
saved) before and after, and writes recall-<label>.json next to this script. The facts are seeded through the operator
API, so this tests recall, not the save-then-recall loop. Set EVAL_EXCLUDE_TOOLS=execute_terminal_command (comma list)
to keep an unattended run from stalling on a guardrail approval; leave it unset to also count wandering tool steps (R4).
"""
import json, os, re, sys, time, urllib.request, urllib.error

BASE, LABEL, MODEL = sys.argv[1].rstrip('/'), sys.argv[2], sys.argv[3]
OUT = os.path.dirname(os.path.abspath(__file__))
EXCLUDE = [t for t in os.environ.get('EVAL_EXCLUDE_TOOLS', '').split(',') if t]
POLL_S = float(os.environ.get('EVAL_POLL_S', '1'))
ONLY = {c for c in os.environ.get('EVAL_ONLY', '').split(',') if c}  # run just these questions, e.g. R2,R5


def call(method, path, body=None, timeout=60):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method, headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:300]


SEEDS = [
    ('eval', {'content': 'We always deploy through the vertex host.', 'scope': 'workspace', 'mode': 'on_demand', 'keep': 'permanent'}),
    ('eval', {'content': 'I prefer tabs in Go files.', 'scope': 'user', 'mode': 'on_demand', 'keep': 'permanent'}),
    ('eval', {'content': 'I like answers in one short sentence.', 'scope': 'user', 'mode': 'always', 'keep': 'permanent'}),
]
# (id, workspace, question, check name, check(answer) -> bool)
sentences = lambda t: len([s for s in re.split(r'(?<=[.!?])\s+', t.strip()) if s.strip()])
QUESTIONS = [
    ('R1 keyword recall', 'eval', 'Which host do we deploy through?', 'mentions vertex', lambda a: 'vertex' in a.lower()),
    ('R2 paraphrase', 'eval', 'Where do we ship releases?', 'mentions vertex', lambda a: 'vertex' in a.lower()),
    ('R3 user fact in another workspace', 'eval2', 'What indentation should you use in my Go code?', 'says tabs', lambda a: 'tab' in a.lower()),
    ('R4 workspace fact stays in its workspace', 'eval2', 'Which host do we deploy through?', 'does NOT claim vertex', lambda a: 'vertex' not in a.lower()),
    ('R5 hot preference shapes the answer', 'eval', 'Explain what gofmt does.', 'one sentence', lambda a: sentences(a) <= 1),
    ('R6 unrelated question', 'eval', 'What is the capital of France?', 'says Paris', lambda a: 'paris' in a.lower()),
]


def real_user_idx(h):
    idx = []
    for i, m in enumerate(h):
        c = m.get('content') or ''
        if m['role'] != 'user' or c.startswith('SYSTEM:') or '[Context for this message' in c:
            continue
        if idx and h[idx[-1]]['content'].strip() == c.strip():
            continue
        idx.append(i)
    return idx


def ask(ws, conv, msg):
    payload = {'workspace_id': ws, 'conversation_id': conv, 'message': msg}
    if EXCLUDE:
        payload['exclude_tools'] = EXCLUDE
    st, d = call('POST', '/admin/api/conversation/message', payload)
    if st not in (200, 202):
        return {'error': f'send {st} {d}'}
    t0 = time.time()
    while time.time() - t0 < 900:
        time.sleep(POLL_S)
        _, act = call('GET', f'/admin/api/workspaces/{ws}/active-runs')
        st, d = call('GET', f'/admin/api/conversation/sessions/{ws}/{conv}')
        h = d['history'] if st == 200 and isinstance(d, dict) else []
        if h and not (act and act.get('assistant_running')) and h[-1]['role'] == 'assistant' and not h[-1].get('tool_calls') and real_user_idx(h):
            idx = real_user_idx(h)[0]
            seg = h[idx:]
            calls = [(tc['function']['name'], tc['function']['arguments'][:90]) for m in seg for tc in (m.get('tool_calls') or [])]
            return {'seconds': round(time.time() - t0, 1), 'answer': (h[-1].get('content') or ''), 'tool_calls': calls}
    return {'error': 'timeout'}


def main():
    call('POST', '/admin/api/dispatcher/workspaces', {'id': 'eval2'})
    print(f'== {LABEL} / {MODEL}: primary ->', call('PUT', '/admin/api/config', {'primary_model': MODEL, 'fallback_model': MODEL})[0], flush=True)
    for ws in ('eval', 'eval2', 'global'):
        st, d = call('GET', f'/admin/api/memory/{ws}')
        for e in (d if st == 200 and isinstance(d, list) else []):
            call('DELETE', f'/admin/api/memory/{ws}/{e["id"]}')
    for ws, body in SEEDS:
        call('POST', f'/admin/api/memory/{ws}', body)
    results = []
    for qid, ws, q, check, fn in QUESTIONS:
        if ONLY and qid[:2] not in ONLY:
            continue
        r = ask(ws, f'{LABEL}-{qid[:2].lower()}-{int(time.time())}', q)
        ok = (not r.get('error')) and bool(r.get('answer', '').strip()) and fn(r['answer'])
        results.append({'id': qid, 'ws': ws, 'question': q, 'check': check, 'pass': ok, **r})
        print(f"  {qid:44} {'PASS' if ok else 'FAIL'}  tools={[c[0] for c in r.get('tool_calls', [])]} {r.get('seconds')}s | {(r.get('answer') or r.get('error'))[:110]!r}", flush=True)
    for ws in ('eval', 'eval2', 'global'):
        st, d = call('GET', f'/admin/api/memory/{ws}')
        for e in (d if st == 200 and isinstance(d, list) else []):
            call('DELETE', f'/admin/api/memory/{ws}/{e["id"]}')
    with open(f"{OUT}/recall-{LABEL}.json", 'w') as f:
        json.dump({'label': LABEL, 'model': MODEL, 'results': results}, f, indent=1)


main()
