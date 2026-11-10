import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location('review_checks', Path(__file__).with_name('review_checks.py'))
CHECKS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKS)


class ReviewChecksTests(unittest.TestCase):
    def test_protected_inventory_cannot_be_emptied_or_rebased_to_changed_bytes(self):
        import hashlib
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            original = b'CREATE TABLE original;\n'
            name = 'schema/original.sql'
            (root / 'schema').mkdir()
            (root / name).write_bytes(original)
            policy = root / 'docs/maintenance/protected-inputs.json'
            policy.parent.mkdir(parents=True)
            for required in ('LICENSE', 'SECURITY.md', 'THIRD_PARTY_NOTICES.md'):
                (root / required).write_text('test fixture', encoding='utf-8')
            def source(_root, *arguments):
                return (name + '\n').encode() if arguments[0] == 'ls-tree' else original
            row = {'path': name, 'sha256': hashlib.sha256(original).hexdigest()}
            with patch.object(CHECKS, 'git', side_effect=source), patch.object(CHECKS, 'source_files', return_value=[]):
                policy.write_text(json.dumps({'files': [row]}), encoding='utf-8')
                self.assertEqual(CHECKS.audit(root)['status'], 'PASS')
                policy.write_text(json.dumps({'files': []}), encoding='utf-8')
                self.assertIn('invalid_protected_input_policy', CHECKS.audit(root)['errors'])
                changed = b'CREATE TABLE changed;\n'
                (root / name).write_bytes(changed)
                policy.write_text(json.dumps({'files': [{'path': name, 'sha256': hashlib.sha256(changed).hexdigest()}]}), encoding='utf-8')
                self.assertIn('protected_policy_rewritten:' + name, CHECKS.audit(root)['errors'])

    def test_vulnerability_output_requires_real_scan_and_checks_call_traces(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'vulnerabilities.json'
            path.write_text('{}', encoding='utf-8')
            self.assertEqual(CHECKS.vulnerability_events(path)['status'], 'FAIL')
            path.write_text(json.dumps({'config': {}}) + '\n' + json.dumps({'finding': {'osv': 'GO-example', 'trace': [{'module': 'example'}]}}), encoding='utf-8')
            self.assertEqual(CHECKS.vulnerability_events(path)['status'], 'PASS')
            path.write_text(json.dumps({'config': {}}) + '\n' + json.dumps({'finding': {'osv': 'GO-example', 'trace': [{'function': 'Affected'}]}}), encoding='utf-8')
            self.assertEqual(CHECKS.vulnerability_events(path)['status'], 'FAIL')

    def events(self, values, kind='integration', required=()):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'events.jsonl'
            path.write_text('\n'.join(json.dumps(row) for row in values), encoding='utf-8')
            return CHECKS.test_events(path, kind, list(required))

    def test_empty_or_missing_completion_is_rejected(self):
        self.assertEqual(self.events([])['status'], 'FAIL')
        self.assertEqual(self.events([{'Action': 'pass', 'Test': 'TestOne'}])['status'], 'FAIL')

    def test_pass_requires_named_test_and_package_completion(self):
        rows = [{'Action': 'pass', 'Test': 'TestOne'}, {'Action': 'pass', 'Package': 'sample'}]
        self.assertEqual(self.events(rows, required=['TestOne'])['status'], 'PASS')
        self.assertEqual(self.events(rows, required=['TestMissing'])['status'], 'FAIL')

    def test_integration_never_silently_skips(self):
        rows = [{'Action': 'pass', 'Test': 'TestOne'}, {'Action': 'skip', 'Test': 'TestStorage'}, {'Action': 'pass', 'Package': 'sample'}]
        self.assertEqual(self.events(rows)['status'], 'FAIL')

    def test_failure_always_rejects_even_after_passing_cases(self):
        rows = [{'Action': 'pass', 'Test': 'TestOne'}, {'Action': 'fail', 'Test': 'TestOther'}, {'Action': 'fail', 'Package': 'sample'}]
        self.assertEqual(self.events(rows)['status'], 'FAIL')

    def test_only_documented_unit_skips_are_allowed(self):
        rows = [{'Action': 'pass', 'Test': 'TestOne'}, {'Action': 'output', 'Test': 'TestStorage', 'Output': 'set PVE_INTEGRATION=1 for isolated integration'}, {'Action': 'skip', 'Test': 'TestStorage'}, {'Action': 'pass', 'Package': 'sample'}]
        self.assertEqual(self.events(rows, 'unit')['status'], 'PASS')
        rows[1]['Output'] = 'unknown shortcut'
        self.assertEqual(self.events(rows, 'unit')['status'], 'FAIL')

    def test_docs_only_change_classification_is_lightweight(self):
        from unittest.mock import patch
        event = {'before': 'a' * 40, 'after': 'b' * 40}
        with patch.object(CHECKS, 'git', return_value=b'README.md\ndocs/architecture.md\n'):
            self.assertFalse(any(CHECKS.classify(Path('.'), event).values()))

    def test_shared_and_unknown_changes_cannot_bypass_backend(self):
        from unittest.mock import patch
        event = {'before': 'a' * 40, 'after': 'b' * 40}
        with patch.object(CHECKS, 'git', return_value=b'backend/internal/auth/jwt.go\n'):
            flags = CHECKS.classify(Path('.'), event)
            self.assertTrue(flags['backend'] and flags['r5'])
        for name in (b'client/unity-demo/Assets/Scripts/PveControlClient.cs\n', b'go.work\n', b'docs/maintenance/protected-inputs.json\n'):
            with self.subTest(path=name), patch.object(CHECKS, 'git', return_value=name):
                self.assertTrue(all(CHECKS.classify(Path('.'), event).values()))
        self.assertTrue(all(CHECKS.classify(Path('.'), {}).values()))


if __name__ == '__main__':
    unittest.main()
