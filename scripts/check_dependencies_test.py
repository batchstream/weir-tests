import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location('dependencies', Path(__file__).with_name('check_dependencies.py'))
dependencies = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dependencies)


class BoundaryTests(unittest.TestCase):
    def setUp(self):
        self.pins = dict(sdk='v0.2.0', protocol='v0.1.0')
        self.modules = [dict(Path=dependencies.SDK, Version='v0.2.0'), dict(Path=dependencies.PROTOCOL, Version='v0.1.0')]

    def test_released_public_contracts(self):
        dependencies.check_modules(self.modules, self.pins)

    def test_server_floating_client_and_replacements_are_rejected(self):
        cases = [self.modules + [dict(Path=dependencies.SERVER, Version='v0.1.0')],
                 [dict(Path=dependencies.SDK, Version='main'), self.modules[1]],
                 [dict(Path=dependencies.SDK, Version='v0.2.0', Replace=dict(Path='../sdk')), self.modules[1]],
                 self.modules[:1]]
        for case in cases:
            with self.subTest(case=case):
                with self.assertRaises(ValueError):
                    dependencies.check_modules(case, self.pins)

    def test_unused_and_transitive_server_edges_are_rejected(self):
        graph = 'indirect@v1.0.0 ' + dependencies.SERVER + '@v0.1.0\n'
        with self.assertRaises(ValueError):
            dependencies.check_graph(graph)

    def test_server_package_closure_is_rejected(self):
        package = dict(ImportPath=dependencies.SERVER + '/internal/app', Module=dict(Path=dependencies.SERVER))
        with self.assertRaises(ValueError):
            dependencies.check_packages([package])


if __name__ == '__main__':
    unittest.main()
