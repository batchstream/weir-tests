import unittest

from run_write_batching import cases


class WriteBatchingPlanTest(unittest.TestCase):
    def test_calibration_changes_one_control_from_bounded128(self):
        controls = {name: (count, wait, slots) for name, count, wait, slots in cases('calibration')}
        anchor = controls['bounded128']
        for name in ('bounded32', 'bounded256', 'longer-wait', 'fewer-slots'):
            self.assertEqual(sum(a != b for a, b in zip(anchor, controls[name])), 1)
        self.assertEqual(controls['immediate'], (32, '0s', 0))

    def test_confirmation_is_frozen_and_does_not_select_from_results(self):
        self.assertEqual(cases('formal'), [('bounded128', 128, '5ms', 16)])
        with self.assertRaises(ValueError):
            cases('unknown')


if __name__ == '__main__':
    unittest.main()
