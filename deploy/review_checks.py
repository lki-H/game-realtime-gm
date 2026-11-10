"""Portable source checks and explicit test-event accounting; never starts services."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
BASELINE = '401247d03832cb10775f166f2deeb71310acc150'
FORBIDDEN_PARTS = {'.git', 'node_modules', 'library', 'private', '__pycache__', 'evidence', 'bin', 'obj'}
FORBIDDEN_SUFFIXES = {'.exe', '.dll', '.pdb', '.dump', '.vhd', '.vhdx', '.log', '.pem', '.key', '.pyc'}
SECRET_PATTERNS = (
    re.compile(r'gh[pousr]_[A-Za-z0-9]{30,}'),
    re.compile(r'github_pat_[A-Za-z0-9_]{50,}'),
    re.compile(r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----'),
)
SHA = re.compile(r'^[0-9a-f]{40,64}$')


def git(root: Path, *args: str) -> bytes:
    value = subprocess.run(['git', '-C', str(root), *args], capture_output=True, check=False)
    if value.returncode:
        raise ValueError('Git source inspection failed: ' + args[0])
    return value.stdout


def source_files(root: Path) -> list[Path]:
    names = set(git(root, 'ls-files', '-z').decode('utf-8').strip('\0').split('\0'))
    names.update(git(root, 'ls-files', '--others', '--exclude-standard', '-z').decode('utf-8').strip('\0').split('\0'))
    return [root / name for name in sorted(names) if name]


def audit(root: Path) -> dict:
    root = root.resolve(strict=True)
    errors = []
    paths = source_files(root)
    links = 0
    for path in paths:
        name = path.relative_to(root).as_posix()
        if any(part.lower() in FORBIDDEN_PARTS for part in path.relative_to(root).parts) or path.suffix.lower() in FORBIDDEN_SUFFIXES:
            errors.append('forbidden_public_file:' + name)
            continue
        if path.name.startswith('.env') and not path.name.endswith('.example'):
            errors.append('private_environment_file:' + name)
        if name.startswith(('docs/learning/', 'docs/day/', 'docs/handoff/')) or name in {'AGENTS.md', 'docs/handoff-phase2.md'}:
            errors.append('internal_material:' + name)
        try:
            cursor = path.parent
            while cursor != root:
                info = cursor.lstat()
                if cursor.is_symlink() or getattr(info, 'st_file_attributes', 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT:
                    raise ValueError('reparse_public_ancestor:' + name)
                cursor = cursor.parent
            attributes = path.lstat()
            if path.is_symlink() or getattr(attributes, 'st_file_attributes', 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT:
                errors.append('reparse_public_file:' + name)
                continue
            if not path.is_file():
                errors.append('missing_public_file:' + name)
                continue
            data = path.read_bytes()
        except (OSError, ValueError):
            errors.append('unreadable_public_file:' + name)
            continue
        try:
            text = data.decode('utf-8-sig')
        except UnicodeDecodeError:
            text = ''
        if any(pattern.search(text) for pattern in SECRET_PATTERNS):
            errors.append('credential_like_literal:' + name)
        if path.suffix.lower() == '.md':
            for target in re.findall(r'!?\[[^\]]*\]\(([^)]+)\)', text):
                target = unquote(target.split('#', 1)[0].strip('<>'))
                if not target or re.match(r'^[a-z][a-z0-9+.-]*:', target, re.I):
                    continue
                linked = root / target.lstrip('/') if target.startswith('/') else path.parent / target
                links += 1
                if not linked.resolve().is_relative_to(root) or not linked.exists():
                    errors.append('broken_local_link:' + name + ':' + target)
    policy = root / 'docs/maintenance/protected-inputs.json'
    try:
        rows = json.loads(policy.read_text(encoding='utf-8-sig'))['files']
        baseline_names = git(root, 'ls-tree', '-r', '--name-only', BASELINE).decode('utf-8').splitlines()
        wire_names = {'experiments/r5-m6/proto/reporting.proto', 'experiments/r5-m6/proto/reporting.pb',
                      'experiments/r5-m6/proto/source.sha256', 'experiments/r5-m6/gen/csharp/Reporting.cs',
                      'experiments/r5-m6/gen/reporting/v1/reporting.pb.go', 'experiments/r5-m6/gen/reporting/v1/reporting_grpc.pb.go'}
        expected_names = {n for n in baseline_names if n.endswith('.sql') or n in wire_names}
        if not isinstance(rows, list) or not rows or len(rows) != len(expected_names) or {r['path'] for r in rows} != expected_names:
            raise ValueError('Protected input inventory differs from the published baseline')
        for row in rows:
            path = root / row['path']
            original = git(root, 'show', BASELINE + ':' + row['path'])
            if hashlib.sha256(original).hexdigest() != row['sha256']:
                errors.append('protected_policy_rewritten:' + row['path'])
            if not path.resolve().is_relative_to(root) or not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != row['sha256']:
                errors.append('protected_bytes_changed:' + row['path'])
    except (OSError, ValueError, KeyError, TypeError):
        errors.append('invalid_protected_input_policy')
        rows = []
    for name in ('LICENSE', 'SECURITY.md', 'THIRD_PARTY_NOTICES.md'):
        if not (root / name).is_file():
            errors.append('required_public_file_missing:' + name)
    return {'status': 'FAIL' if errors else 'PASS', 'files': len(paths), 'local_links': links,
            'protected_files': len(rows), 'errors': errors, 'services_started': False}


def test_events(file: Path, kind: str, required: list[str]) -> dict:
    events = []
    for line in file.read_text(encoding='utf-8-sig').splitlines():
        if not line.strip():
            continue
        try:
            value = json.loads(line)
        except ValueError:
            continue  # Compiler/module diagnostics can precede Go's JSON stream.
        if isinstance(value, dict) and 'Action' in value:
            events.append(value)
    failed = [e.get('Test', e.get('Package', '?')) for e in events if e['Action'] == 'fail']
    passed = [e for e in events if e['Action'] == 'pass' and 'Test' in e]
    skipped = [e for e in events if e['Action'] == 'skip' and 'Test' in e]
    errors = list(failed)
    if not passed:
        errors.append('no_tests_executed')
    if not any(e['Action'] == 'pass' and 'Test' not in e for e in events):
        errors.append('no_successful_package_completion')
    for event in skipped:
        output = ''.join(e.get('Output', '') for e in events if e.get('Test') == event['Test'] and e.get('Package') == event.get('Package'))
        allowed = kind in {'unit', 'r5-unit'} and (
            'set PVE_INTEGRATION=1' in output or 'set R5_INTEGRATION=1 with the isolated R5 configuration' in output or
            ('TestR4ActualBinarySwitchAndRollback' == event['Test'] and 'set both server binary paths' in output))
        if not allowed:
            errors.append('unexpected_skip:' + event['Test'])
    names = {e['Test'] for e in passed}
    errors.extend('required_test_missing:' + name for name in required if name not in names)
    return {'status': 'FAIL' if errors else 'PASS', 'kind': kind, 'pass_events': len(passed),
            'top_level_passes': len([e for e in passed if '/' not in e['Test']]),
            'skipped_tests': [e['Test'] for e in skipped], 'errors': errors}


def classify(root: Path, event: dict) -> dict[str, bool]:
    flags = {'backend': False, 'frontend': False, 'r5': False}
    if event.get('event_name') == 'workflow_dispatch':
        return dict.fromkeys(flags, True)
    if 'pull_request' in event:
        before = event['pull_request']['base']['sha']
        after = event['pull_request']['head']['sha']
    else:
        before, after = event.get('before', ''), event.get('after', '')
    if not SHA.fullmatch(before or '') or not SHA.fullmatch(after or '') or set(before) == {'0'}:
        return dict.fromkeys(flags, True)
    names = git(root, 'diff', '--name-only', before, after).decode('utf-8').splitlines()
    for name in names:
        recognized = False
        if name.startswith(('backend/', 'deploy/', '.github/')):
            flags['backend'] = flags['r5'] = True
            recognized = True
        if name.startswith('frontend/'):
            flags['frontend'] = True
            recognized = True
        if name.startswith('experiments/r5-m6/'):
            flags['r5'] = True
            recognized = True
        if name.startswith('.github/'):
            flags['frontend'] = True
        documentation = name in {'README.md', 'LICENSE', 'SECURITY.md', 'THIRD_PARTY_NOTICES.md'} or name.startswith('docs/') and name.endswith('.md')
        if not recognized and not documentation:
            flags = dict.fromkeys(flags, True)
    return flags


def vulnerability_events(file: Path) -> dict:
    text = file.read_text(encoding='utf-8-sig')
    decoder = json.JSONDecoder()
    values = []
    offset = 0
    while offset < len(text):
        if text[offset].isspace():
            offset += 1
            continue
        value, offset = decoder.raw_decode(text, offset)
        values.append(value)
    if not any('config' in row for row in values if isinstance(row, dict)):
        return {'status': 'FAIL', 'errors': ['vulnerability_scan_did_not_start']}
    findings = [row['finding'] for row in values if isinstance(row, dict) and 'finding' in row]
    called = [row for row in findings if any(frame.get('function') for frame in row.get('trace', []))]
    return {'status': 'FAIL' if called else 'PASS', 'findings': len(findings),
            'reachable_findings': len(called), 'reachable_advisories': sorted({row['osv'] for row in called}),
            'unreachable_findings_are_not_a_runtime_failure': True}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, default=ROOT)
    parser.add_argument('--json', type=Path)
    parser.add_argument('--test-events', type=Path)
    parser.add_argument('--kind', choices=('unit', 'integration', 'r5-unit', 'r5-integration'), default='unit')
    parser.add_argument('--required', action='append', default=[])
    parser.add_argument('--classify', action='store_true')
    parser.add_argument('--vulnerabilities', type=Path)
    args = parser.parse_args()
    try:
        if args.classify:
            event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text(encoding='utf-8'))
            event['event_name'] = os.environ.get('GITHUB_EVENT_NAME', '')
            flags = classify(args.root, event)
            with Path(os.environ['GITHUB_OUTPUT']).open('a', encoding='utf-8') as stream:
                for name, value in flags.items():
                    stream.write(f'{name}={str(value).lower()}\n')
            result = {'status': 'PASS', 'required_suites': flags}
        elif args.vulnerabilities:
            result = vulnerability_events(args.vulnerabilities)
        else:
            result = test_events(args.test_events, args.kind, args.required) if args.test_events else audit(args.root)
    except (OSError, ValueError, KeyError, TypeError) as error:
        result = {'status': 'FAIL', 'errors': [str(error)]}
    if args.json:
        args.json.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
