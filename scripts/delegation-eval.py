#!/usr/bin/env python3
"""B7 eval: does handing research angles to helpers (the "Use helpers" switch) beat a research chat that does the work itself?

Arm A is the Research profile alone; arm B is the same with use_helpers. Same local parent model, same prompts, runs
interleaved so server drift hits both arms. Records, per run, what the run record and history show (finished or not,
parent tokens, helper tokens, time, tool use, bad delegate calls) and the full final answer for blind judging.

Usage: delegation-eval.py BASE WORKSPACE OUT.json [--repeats N] [--model NAME]
Point it at a backend you may load for hours (a scratch workspace is created and deleted). Resumable: reruns skip
finished runs recorded in OUT.json."""
import argparse, json, os, sys, time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from evallib import Backend, turn  # noqa: E402

PROMPTS = {
    'b1': 'Compare the latest stable releases of llama.cpp, Ollama and vLLM: for each, the newest version, its release date, and one notable change. Cite sources.',
    'b2': 'Compare the current stable releases of Go, Rust and Zig: version, release date, and one headline feature each. Cite sources.',
    'b3': 'For Vue, React and Svelte: the current stable major version and how each handles reactivity, in two sentences each. Cite sources.',
    'b4': 'Compare the current API list prices per million input and output tokens for Anthropic, OpenAI and Google flagship models. Cite the pricing pages.',
    'b5': 'For the Qwen, Llama and DeepSeek open-weight model families: the latest release, the model sizes offered, and the license. Cite sources.',
    'b6': 'How do llama.cpp, vLLM and TensorRT-LLM each support KV cache quantization? One paragraph and one source per project.',
    'b7': 'Compare Tailscale, WireGuard and ZeroTier for a home lab: setup effort, performance, and pricing. Cite sources.',
    'b8': 'Compare SQLite backup approaches: the online backup API, Litestream and LiteFS. How each works and its main limit. Cite sources.',
    's1': 'What is the latest stable version of Go and when was it released?',
    's2': 'What port does PostgreSQL listen on by default and where is it configured?',
    's3': 'What does the HTTP 429 status code mean?',
    's4': 'Who maintains llama.cpp and under what license is it released?',
}
BAD_ARGS = 'INVALID ARGUMENTS'


def summarise(arm, pid, rep, r):
    h = r.get('history', [])
    t = turn(h, 1) if h else {}
    calls = t.get('tool_calls', [])
    run = t.get('run') or {}
    final = ''
    if h and h[-1]['role'] == 'assistant' and not h[-1].get('tool_calls'):
        final = h[-1].get('content') or ''
    subs = run.get('subagents') or []
    return {
        'arm': arm, 'prompt': pid, 'repeat': rep, 'error': r.get('error'), 'ended_oddly': r.get('ended_oddly'),
        'seconds': r.get('seconds'), 'finished': bool(final.strip()) and not r.get('error'),
        'parent_prompt_tokens': run.get('prompt_tokens'), 'parent_completion_tokens': run.get('completion_tokens'),
        'duration_ms': run.get('duration_ms'), 'model': run.get('model'),
        'context_overflows': run.get('context_overflows') or 0, 'helpers': len(subs), 'helper_failed': sum(1 for s in subs if s.get('status') != 'done'),
        'helper_prompt_tokens': sum(s.get('prompt_tokens') or 0 for s in subs),
        'helper_completion_tokens': sum(s.get('completion_tokens') or 0 for s in subs),
        'tools': [c['tool'] for c in calls],
        'delegate_calls': sum(1 for c in calls if c['tool'] == 'delegate_task'),
        'bad_delegate_calls': sum(1 for c in calls if c['tool'] == 'delegate_task' and BAD_ARGS in c['result']),
        'final': final,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('base'); ap.add_argument('workspace'); ap.add_argument('out')
    ap.add_argument('--repeats', type=int, default=2)
    ap.add_argument('--model', default='Qwen3.6 35B A3B')
    ap.add_argument('--arms', default='AB', help='arms to run, e.g. B to run only the helpers arm')
    ap.add_argument('--only', default='', help='comma-separated prompt ids, for a smoke run')
    a = ap.parse_args()
    b = Backend(a.base)
    results = json.load(open(a.out)) if os.path.exists(a.out) else []
    done = {(r['arm'], r['prompt'], r['repeat']) for r in results}
    _, lanes = b.api('GET', '/admin/api/active-runs')
    if any(l['running'] or l['waiting'] for l in lanes['lanes']):
        sys.exit('backend is busy; not starting')
    st, _ = b.api('POST', '/admin/api/dispatcher/workspaces', {'id': a.workspace})
    if st not in (200, 201, 409):
        sys.exit(f'create workspace failed: {st}')
    keep_global = {e['id'] for e in b.global_memories()}
    stamp = int(time.time())
    order = [p for p in PROMPTS if not a.only or p in a.only.split(',')]
    try:
        for rep in range(1, a.repeats + 1):
            for i, pid in enumerate(order):
                arms = ['A', 'B'] if (i + rep) % 2 == 0 else ['B', 'A']
                for arm in arms:
                    if (arm, pid, rep) in done or arm not in a.arms:
                        continue
                    extra = {'profile': 'research', 'model': a.model}
                    if arm == 'B':
                        extra['use_helpers'] = True
                    r = b.ask(a.workspace, f'{arm}-{pid}-{rep}-{stamp}', PROMPTS[pid], 1, extra)
                    row = summarise(arm, pid, rep, r)
                    results.append(row)
                    json.dump(results, open(a.out, 'w'), indent=1)
                    print(arm, pid, rep, 'ok' if row['finished'] else 'FAIL', row['seconds'], row['parent_prompt_tokens'],
                          'helpers', row['helpers'], flush=True)
    finally:
        b.forget_global_except(keep_global)
        b.api('DELETE', f'/admin/api/dispatcher/workspaces/{a.workspace}')
    print('done')


if __name__ == '__main__':
    main()
