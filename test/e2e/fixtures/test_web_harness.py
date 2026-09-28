import importlib.util
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('web_harness',Path(__file__).resolve().parents[3]/'hack/e2e-web.py')
web=importlib.util.module_from_spec(spec);spec.loader.exec_module(web)

class WebHarnessTests(unittest.TestCase):
    def test_only_clean_https_urls(self):
        for url in ('http://agents.example.com','https://u:p@agents.example.com','https://agents.example.com/?token=x','https://agents.example.com/#x','https://agents.example.com:9119'):
            with self.assertRaises(ValueError):web.validate_url(url)
        self.assertEqual(web.validate_url('https://agents.example.com/maria/'),'https://agents.example.com/maria')
