"""Exercise the actual CI queue observation helpers without starting services."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('verify-ci.sh').read_text(encoding='utf-8')
HELPERS = SCRIPT[SCRIPT.index('queue_state() {'):SCRIPT.index('start_role() {')]
BASH = os.environ.get('R5_MODEL_BASH') or shutil.which('bash')

MOCK = r'''
docker() {
  local observation
  observation="$(cat "$R5_MODEL_COUNTER")"
  printf '%s\n' "$((observation+1))" >"$R5_MODEL_COUNTER"
  case "$R5_MODEL_SCENARIO" in
    delayed)
      if [[ "$observation" -eq 0 ]]; then printf 'r5.reports\tquorum\ttrue\t0\t0\t0\n'
      elif [[ "$observation" -eq 1 ]]; then printf 'r5.reports\tquorum\ttrue\t3\t0\t1\n'
      else printf 'r5.reports\tquorum\ttrue\t3\t0\t0\n'; fi ;;
    zero) printf 'r5.reports\tquorum\ttrue\t0\t0\t0\n' ;;
    missing) printf 'another.queue\tquorum\ttrue\t3\t0\t0\n' ;;
    classic) printf 'r5.reports\tclassic\ttrue\t3\t0\t0\n' ;;
    volatile) printf 'r5.reports\tquorum\tfalse\t3\t0\t0\n' ;;
    unacknowledged) printf 'r5.reports\tquorum\ttrue\t3\t1\t0\n' ;;
    failed_cli) return 1 ;;
    invalid_count) printf 'r5.reports\tquorum\ttrue\tunknown\t0\t0\n' ;;
    *) return 2 ;;
  esac
}
sleep() { :; }
'''


class QueueObservationTests(unittest.TestCase):
    def run_model(self, scenario, command='await_queue_backlog before_restart 3 3'):
        if not BASH:
            self.fail('Bash is required to test the actual queue helpers')
        with tempfile.TemporaryDirectory(prefix='r5-queue-model-') as directory:
            counter = Path(directory, 'observations')
            counter.write_text('0\n', encoding='ascii')
            environment = os.environ.copy()
            environment.update(R5_MODEL_SCENARIO=scenario, R5_MODEL_COUNTER=counter.as_posix())
            return subprocess.run([BASH, '-c', 'set -euo pipefail\n'+HELPERS+MOCK+'\n'+command],
                                  env=environment, text=True, capture_output=True, timeout=10)

    def test_delayed_metric_requires_confirmed_count_and_no_consumer(self):
        result = self.run_model('delayed')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), '3')
        self.assertIn('"observation":3', result.stderr)

    def test_restart_observation_waits_for_same_confirmed_backlog(self):
        result = self.run_model('delayed', 'await_queue_backlog after_restart 3 3')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), '3')
        self.assertIn('"stage":"after_restart"', result.stderr)

    def test_missing_backlog_and_invalid_topology_still_fail(self):
        for scenario in ('zero', 'missing', 'classic', 'volatile', 'unacknowledged', 'failed_cli', 'invalid_count'):
            with self.subTest(scenario=scenario):
                result = self.run_model(scenario, 'await_queue_backlog before_restart 3 2')
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')
                self.assertIn('within the bounded before_restart window', result.stderr)

    def test_unavailable_metric_is_not_an_empty_queue(self):
        result = self.run_model('failed_cli', 'if ready="$(queue_ready)" && [[ "$ready" -eq 0 ]]; then exit 99; fi')
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == '__main__':
    unittest.main()
