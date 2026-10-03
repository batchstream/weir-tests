import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location('prepare_weir', Path(__file__).with_name('prepare_weir.py'))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class SourceTests(unittest.TestCase):
    def setUp(self):
        self.pins = dict(weir_source='a' * 40, weir_version='v0.1.1-0.20261003014747-aaaaaaaaaaaa', weir_sum='h1:source')
        self.module = dict(Path='github.com/batchstream/weir', Version=self.pins['weir_version'],
                           Sum=self.pins['weir_sum'], Origin=dict(Hash=self.pins['weir_source']))

    def test_exact_source(self):
        prepare.validate_source(self.pins, self.module)

    def test_floating_source_and_wrong_artifact_are_rejected(self):
        for key, value in [('Path', 'another/module'), ('Version', 'main'), ('Sum', 'h1:different'),
                           ('Origin', dict(Hash='b' * 40)), ('Replace', dict(Path='../weir')), ('Error', 'missing')]:
            with self.subTest(key=key):
                changed = dict(self.module)
                changed[key] = value
                with self.assertRaises(ValueError):
                    prepare.validate_source(self.pins, changed)
        changed = dict(self.pins, weir_source='main')
        with self.assertRaises(ValueError):
            prepare.validate_source(changed, self.module)


if __name__ == '__main__':
    unittest.main()
