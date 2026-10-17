"""Shared pieces of the eval drivers (memory-eval.py, prompt-eval.py): HTTP calls to a scratch backend, reading a
conversation's turns, asking one question and waiting for the run to end, and the memory store cleanup.

Always point a driver at a SCRATCH backend (own --data dir, started from a directory outside backend/)."""
import json, os, time, urllib.request, urllib.error

POLL_S = float(os.environ.get('EVAL_POLL_S', '1'))
RUN_TIMEOUT_S = 900
FINAL_CHARS = 4000  # how much of the final answer is kept: enough to judge whether it ends cleanly
EXCLUDE = [t for t in os.environ.get('EVAL_EXCLUDE_TOOLS', '').split(',') if t]  # e.g. execute_terminal_command: a run stalls on a guardrail approval


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
    """Messages of the expect_users-th user turn: tool calls, results, final answer, and the run record."""
    idx = real_user_turns(h)
    if len(idx) < expect_users:
        return {}
    start = idx[expect_users - 1]
    seg = h[start:(idx[expect_users] if len(idx) > expect_users else len(h))]
    results = {m.get('tool_call_id'): m['content'] for m in seg if m['role'] == 'tool'}
    calls = []
    for m in seg:
        for tc in m.get('tool_calls') or []:
            calls.append({'tool': tc['function']['name'], 'args': tc['function']['arguments'], 'result': results.get(tc['id'], '')[:200]})
    last = seg[-1]
    return {'tool_calls': calls, 'saves': [c for c in calls if c['tool'] == 'memory_update'],
            'run': h[start].get('run') or {},
            'system_chars': len(h[0].get('content') or '') if h and h[0]['role'] == 'system' else 0,
            'final_is_assistant_text': last['role'] == 'assistant' and not last.get('tool_calls') and bool((last.get('content') or '').strip()),
            'final': (last.get('content') or '')[:FINAL_CHARS]}


class Backend:
    def __init__(self, base, memory_workspaces=()):
        self.base = base
        self.memory_workspaces = memory_workspaces

    def api(self, method, path, body=None):
        return call(self.base, method, path, body)

    def history(self, ws, conv):
        st, d = self.api('GET', f'/admin/api/conversation/sessions/{ws}/{conv}')
        return d['history'] if st == 200 and isinstance(d, dict) else []

    def ask(self, ws, conv, msg, expect_users, extra=None):
        """Send one message and wait for the run to end. extra adds request fields (e.g. profile)."""
        payload = {'workspace_id': ws, 'conversation_id': conv, 'message': msg, **(extra or {})}
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
        for ws in self.memory_workspaces:
            st, d = self.api('GET', f'/admin/api/memory/{ws}')
            out += d if st == 200 and isinstance(d, list) else []
        return out

    def clear(self):
        for e in self.memories():
            self.api('DELETE', f'/admin/api/memory/{e["workspace_id"]}/{e["id"]}')
