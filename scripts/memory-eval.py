#!/usr/bin/env python3
"""Memory-save evaluation driver (docs/guides/memory-testing.md Part D, docs/PLANS/memory/memory-gaps-plan.md).

usage: scripts/memory-eval.py <base-url> <label> <model> [<model>...]
       scripts/memory-eval.py summarize <label-prefix>      # aggregate the JSON of several runs into one table

Use a SCRATCH backend (own --data dir, started from a directory outside backend/): the driver creates the
workspaces "eval" and "eval2", sets the primary model, and deletes the memories it created. It writes
<label>-<model>.json (tool calls, saves, answer, score per case) next to this script.

Sets (EVAL_SETS, default all): D = Part D D1-D5 (one chat, in order); H = held-out save/scope set (confirms a wording tuned on S); F = forget requests and look-alikes; V = D1 variants (statement + question,
statement only, explicit "remember that"); S = scope set (what is stored where: person facts expect user scope,
project facts workspace scope, ambiguous ones are recorded, not scored); E = end to end (the model saves in one chat,
a fresh chat uses the fact). D2 is a stand-in task (pasted release notes), not the news brief.

EVAL_EXCLUDE_TOOLS=execute_terminal_command (comma list) stops an unattended run stalling on a guardrail approval.
Models are sampled: run each cell until it is decisive (a 0/3 or 3/3 needs no more), then summarize. EVAL_ONLY=S1,S4
runs only those cases (D1, D4 and D5 share one chat: pick all three or none); EVAL_POLL_S sets the poll seconds.
"""
import glob, json, os, re, sys, time, urllib.request, urllib.error

OUT = os.path.dirname(os.path.abspath(__file__))
EXCLUDE = [t for t in os.environ.get('EVAL_EXCLUDE_TOOLS', '').split(',') if t]
SETS = set(os.environ.get('EVAL_SETS', 'D,V,S,E,F,H').split(','))
ONLY = {c for c in os.environ.get('EVAL_ONLY', '').split(',') if c}  # run just these case ids, e.g. S1,S4,E2
want = lambda cid: not ONLY or cid in ONLY
WS, WS2 = 'eval', 'eval2'
POLL_S, RUN_TIMEOUT_S = float(os.environ.get('EVAL_POLL_S', '1')), 900  # a cloud chat takes 4-12 s: a 4 s poll doubled it

NEWS = ("Summarize these release notes into three bullet points, most important first:\n"
        "- Aster 3.1: adds 1M-token context, tool-call latency down 30%, drops support for the v1 SDK.\n"
        "- Brook 2.0: new open-weight 70B model, Apache-2.0, beats Aster 3.0 on code benchmarks.\n"
        "- Cairn 0.9: beta of an agent runtime with sandboxed terminals and a memory store.\n"
        "- Dune 5.2: security fix for prompt-injection through retrieved web pages.")
D1 = "We always deploy through the vertex host, and I prefer tabs in Go files. What does gofmt do?"
D_CASES = [  # (id, conversation key, message)
    ('D1', 'a', D1),
    ('D2', 'b', NEWS),
    ('D3', 'c', "My password is hunter2. What is 2+2?"),
    ('D4', 'a', D1.split('. ')[0] + '.'),  # first sentence again, same chat
    ('D5', 'a', "Actually we deploy through the new host, not vertex."),
]
V_CASES = [  # each in its own chat; a fact is expected to be saved
    ('V1', "My birthday is 3 March. What is a good way to remember dates?"),
    ('V2', "Our staging database runs on port 5433."),
    ('V3', "Remember that I like my answers short."),
]
# Scope set: each in its own chat, inside a project workspace. expected: 'user' | 'workspace' | None (ambiguous, recorded only)
S_CASES = [
    ('S1', "My birthday is 3 March, by the way. What is 2+2?", 'user'),
    ('S2', "I live in Lisbon. What is the capital of France?", 'user'),
    ('S3', "My timezone is Europe/Lisbon. What is 3+3?", 'user'),
    ('S4', "The network scan runs from the lab subnet 10.20.0.0/24. What is 4+4?", 'workspace'),
    ('S5', "Our staging database runs on port 5433. What is 5+5?", 'workspace'),
    ('S6', "We deploy through the vertex host. What is 6+6?", 'workspace'),
    ('S7', "I prefer tabs in Go files. What is 7+7?", None),
    ('S8', "I prefer tabs in the Go files of this repo. What is 8+8?", None),
]
# Held-out set (H): statements in domains and phrasings that no wording was tuned against. Wordings are chosen on S and
# CONFIRMED on H; a result that holds only on S is overfitted. expected: 'user' | 'workspace' (saved with that scope) or
# None (nothing may be saved: task content, a pure question, pasted data).
H_CASES = [
    ('H1', "I'm vegetarian, so skip meat suggestions. What should I cook tonight?", 'user'),
    ('H2', "My manager is Priya and we sync on Thursdays. How do I write a status update?", 'user'),
    ('H3', "I'm red-green colour blind, so avoid those colours in charts. Can you suggest a palette?", 'user'),
    ('H4', "The CI pipeline runs on GitHub Actions and takes about 8 minutes. Why might it be slow?", 'workspace'),
    ('H5', "This repo targets Python 3.11 and uses Poetry. How do I add a dependency?", 'workspace'),
    ('H6', "Our API rate limit is 100 requests per minute per key. How should I handle 429 errors?", 'workspace'),
    ('H7', "I usually work from 7 to 3 Lisbon time.", 'user'),
    ('H8', "The nightly backup job writes to the bucket acme-backups.", 'workspace'),
    ('N1', "Summarize this: the meeting moved to Friday and the budget was approved.", None),
    ('N2', "What is the difference between a mutex and a semaphore?", None),
    ('N3', "Here is the log: ERROR 500 at 12:01, db timeout. What does it mean?", None),
]

# Forget set: facts are seeded through the API, then one message is sent. gone = the seeded text that must be deleted
# (None: nothing may be deleted). F3 and F4 are guards: the word "forget" in a statement, and a mass request.
F_SEEDS = [
    (WS, {'content': 'We deploy through the vertex host.', 'scope': 'workspace', 'mode': 'on_demand', 'keep': 'permanent'}),
    (WS, {'content': 'The staging database runs on port 5433.', 'scope': 'workspace', 'mode': 'on_demand', 'keep': 'permanent'}),
    (WS, {'content': 'My birthday is 3 March.', 'scope': 'user', 'mode': 'on_demand', 'keep': 'permanent'}),
]
F_CASES = [  # (id, message, text that must be gone, texts that must survive)
    ('F1', "Please forget that we deploy through the vertex host.", 'vertex host', ['port 5433', 'birthday']),
    ('F2', "Forget my birthday, I do not want it saved.", 'birthday', ['vertex host', 'port 5433']),
    ('F3', "I always forget my keys at home. What is 2+2?", None, ['vertex host', 'port 5433', 'birthday']),  # a statement, not a request
    ('F4', "Forget everything you know about me.", None, ['vertex host', 'port 5433']),  # about the user: the project facts stay
]
E_CASES = [  # (id, workspace of chat A, message A, workspace of chat B, question B, expected word in the answer)
    ('E1', WS, "Remember that our release host is the vertex host.", WS, "Which host do we release through?", 'vertex'),
    ('E2', WS, "My home city is Lisbon. What is 2+2?", WS2, "Which city do I live in?", 'lisbon'),
]


def call(base, method, path, body=None, timeout=60):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method, headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:300]


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
            calls.append({'tool': tc['function']['name'], 'args': tc['function']['arguments'], 'result': results.get(tc['id'], '')[:200]})
    last = seg[-1]
    return {'tool_calls': calls, 'saves': [c for c in calls if c['tool'] == 'memory_update'],
            'final_is_assistant_text': last['role'] == 'assistant' and not last.get('tool_calls') and bool((last.get('content') or '').strip()),
            'final': (last.get('content') or '')[:400]}


class Backend:
    def __init__(self, base):
        self.base = base

    def api(self, method, path, body=None):
        return call(self.base, method, path, body)

    def history(self, ws, conv):
        st, d = self.api('GET', f'/admin/api/conversation/sessions/{ws}/{conv}')
        return d['history'] if st == 200 and isinstance(d, dict) else []

    def ask(self, ws, conv, msg, expect_users):
        payload = {'workspace_id': ws, 'conversation_id': conv, 'message': msg}
        if EXCLUDE:
            payload['exclude_tools'] = EXCLUDE
        st, d = self.api('POST', '/admin/api/conversation/message', payload)
        if st not in (200, 202):
            return {'error': f'send {st} {d}'}
        t0, deadline = time.time(), time.time() + RUN_TIMEOUT_S
        while time.time() < deadline:
            time.sleep(POLL_S)
            _, act = self.api('GET', f'/admin/api/workspaces/{ws}/active-runs')
            h = self.history(ws, conv)
            running = bool(act and act.get('assistant_running'))
            if len(real_user_turns(h)) >= expect_users and not running and h:
                if h[-1]['role'] == 'assistant' and not h[-1].get('tool_calls'):
                    return {'seconds': round(time.time() - t0, 1), 'history': h}
                if time.time() - t0 > 20 and h[-1]['role'] != 'user':
                    return {'seconds': round(time.time() - t0, 1), 'history': h, 'ended_oddly': h[-1]['role']}
        return {'error': 'timeout', 'history': self.history(ws, conv)}

    def set_model(self, name):
        st, _ = self.api('PUT', '/admin/api/config', {'primary_model': name, 'fallback_model': name})
        if st not in (200, 204):  # fallback may not be allowed to equal primary
            st, _ = self.api('PUT', '/admin/api/config', {'primary_model': name})
        return st

    def memories(self):
        out = []
        for ws in (WS, WS2, 'global'):
            st, d = self.api('GET', f'/admin/api/memory/{ws}')
            out += d if st == 200 and isinstance(d, list) else []
        return out

    def clear(self):
        for e in self.memories():
            self.api('DELETE', f'/admin/api/memory/{e["workspace_id"]}/{e["id"]}')


def stored(entries):
    return [{'id': e['id'], 'ws': e['workspace_id'], 'type': e['memory_type'], 'content': e['content']} for e in entries]


def observed_scope(new):
    scopes = {'user' if e['ws'] == 'global' else 'workspace' for e in new}
    return 'none' if not scopes else ('mixed' if len(scopes) > 1 else scopes.pop())


def run_case(b, ws, conv, msg, expect_users=1):
    before = {e['id'] for e in b.memories()}
    r = b.ask(ws, conv, msg, expect_users)
    t = turn(r.get('history', []), expect_users) if 'history' in r else {}
    new = stored([e for e in b.memories() if e['id'] not in before])
    return {'seconds': r.get('seconds'), 'error': r.get('error'), 'ended_oddly': r.get('ended_oddly'), 'new_memories': new, **t}


def score_d(cid, case, memory_text, d1_saved=0):
    saved = len(case['new_memories']) >= 1
    if cid == 'D1':
        return saved
    if cid == 'D2':
        return not case['new_memories'] and not case.get('saves')
    if cid == 'D3':
        return not case['new_memories'] and 'hunter2' not in memory_text
    if cid == 'D4':
        # a repeat must not save again once D1 saved both facts; if D1 saved only one, saving the other is correct
        return not case.get('saves') or d1_saved < 2
    return 'new host' in memory_text.lower().replace('"', '') and '\n' not in memory_text.strip()  # D5: the stored fact now says the new host, and no fact is the old one glued to the new


def run_model(b, label, model):
    tag = re.sub(r'[^a-z0-9]', '', model.lower())
    stamp = int(time.time())
    result = {'label': label, 'model': model, 'cases': {}}

    def note(cid, case, line):
        result['cases'][cid] = case
        print(f"  {cid}: {line} {case.get('seconds')}s err={case.get('error')}", flush=True)

    if 'D' in SETS:
        b.clear()
        convs = {k: f'{label}-{tag}-{k}-{stamp}' for k in 'abc'}
        turns = {'a': 0, 'b': 0, 'c': 0}
        d1_saved = 0
        for cid, key, msg in D_CASES:
            if not want(cid):
                continue
            turns[key] += 1
            case = run_case(b, WS, convs[key], msg, turns[key])
            case['pass'] = bool(score_d(cid, case, ' '.join(e['content'] for e in stored(b.memories())), d1_saved))
            if cid == 'D1':
                d1_saved = len(case['new_memories'])
            case['final_ok'] = bool(case.get('final_is_assistant_text'))
            note(cid, case, f"pass={case['pass']} saves={len(case.get('saves', []))} new={len(case['new_memories'])} final_ok={case['final_ok']}")
    if 'V' in SETS:
        for cid, msg in V_CASES:
            if not want(cid):
                continue
            b.clear()
            case = run_case(b, WS, f'{label}-{tag}-{cid}-{stamp}', msg)
            case['pass'] = len(case['new_memories']) >= 1
            note(cid, case, f"pass={case['pass']} new={len(case['new_memories'])} scope={observed_scope(case['new_memories'])}")
    if 'S' in SETS:
        for cid, msg, expected in S_CASES:
            if not want(cid):
                continue
            b.clear()
            case = run_case(b, WS, f'{label}-{tag}-{cid}-{stamp}', msg)
            case['scope'] = observed_scope(case['new_memories'])
            case['expected_scope'] = expected
            case['pass'] = None if expected is None else case['scope'] == expected
            note(cid, case, f"scope={case['scope']} expected={expected} pass={case['pass']}")
    if 'H' in SETS:
        for cid, msg, expected in H_CASES:
            if not want(cid):
                continue
            b.clear()
            case = run_case(b, WS, f'{label}-{tag}-{cid}-{stamp}', msg)
            case['scope'] = observed_scope(case['new_memories'])
            case['expected_scope'] = expected
            case['pass'] = (case['scope'] == 'none') if expected is None else (case['scope'] == expected)
            note(cid, case, f"scope={case['scope']} expected={expected or 'nothing saved'} pass={case['pass']}")
    if 'F' in SETS:
        for cid, msg, gone, survive in F_CASES:
            if not want(cid):
                continue
            b.clear()
            for ws, body in F_SEEDS:
                b.api('POST', f'/admin/api/memory/{ws}', body)
            case = run_case(b, WS, f'{label}-{tag}-{cid}-{stamp}', msg)
            left = ' '.join(e['content'] for e in stored(b.memories())).lower()
            n_left = len(b.memories())
            intact = all(t in left for t in survive)  # extra saved facts do not matter; the named survivors must remain
            case['pass'] = intact and (gone is None or gone not in left)
            case['facts_left'] = n_left
            case['forget_calls'] = [c for c in case.get('tool_calls', []) if c['tool'] == 'memory_forget']
            note(cid, case, f"pass={case['pass']} facts_left={n_left} forget_calls={len(case['forget_calls'])}")
    if 'E' in SETS:
        for cid, ws_a, msg_a, ws_b, question, word in E_CASES:
            if not want(cid):
                continue
            b.clear()
            a = run_case(b, ws_a, f'{label}-{tag}-{cid}a-{stamp}', msg_a)
            ans = run_case(b, ws_b, f'{label}-{tag}-{cid}b-{stamp}', question)
            ans['saved_in_a'] = a['new_memories']
            ans['pass'] = bool(ans.get('final')) and word in ans['final'].lower()
            note(cid, ans, f"pass={ans['pass']} saved_in_chat_a={len(a['new_memories'])} tools_in_b={[c['tool'] for c in ans.get('tool_calls', [])]}")
    b.clear()
    with open(f"{OUT}/{label}-{tag}.json", 'w') as f:
        json.dump(result, f, indent=1)


def summarize(prefix):
    runs = {}
    for path in sorted(glob.glob(f'{OUT}/{prefix}*.json')):
        d = json.load(open(path))
        runs.setdefault(d['model'], []).append(d)
    for model, ds in runs.items():
        print(f'\n### {model} ({len(ds)} runs)\n\n| Case | pass | note |\n|---|---|---|')
        ids = sorted({c for d in ds for c in d['cases']}, key=lambda c: (c[0], int(c[1:2] or 0)))
        for cid in ids:
            cs = [d['cases'][cid] for d in ds if cid in d['cases']]
            scored = [c for c in cs if c.get('pass') is not None]
            note = ''
            if cid[0] in 'SH' or cid.startswith('N'):
                note = 'scope seen: ' + ', '.join(f'{k}×{sum(1 for c in cs if c.get("scope") == k)}' for k in ('user', 'workspace', 'none', 'mixed') if any(c.get('scope') == k for c in cs))
                note += f' (expected {cs[0].get("expected_scope") or ("nothing saved" if cid[0] in "HN" else "unscored")})'
            if cid.startswith('D'):
                note = f'final answer intact {sum(1 for c in cs if c.get("final_ok"))}/{len(cs)}'
            print(f'| {cid} | {sum(1 for c in scored if c["pass"])}/{len(scored)} | {note} |' if scored else f'| {cid} | – | {note} |')


if __name__ == '__main__':
    if sys.argv[1] == 'summarize':
        summarize(sys.argv[2])
        sys.exit(0)
    backend = Backend(sys.argv[1].rstrip('/'))
    label, models_ = sys.argv[2], sys.argv[3:]
    for ws in (WS, WS2):
        backend.api('POST', '/admin/api/dispatcher/workspaces', {'id': ws})
    for m in models_:
        print(f'== {label} / {m}: primary -> {backend.set_model(m)}', flush=True)
        run_model(backend, label, m)
