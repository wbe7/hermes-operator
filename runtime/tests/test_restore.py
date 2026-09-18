import json
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import yaml
from bootstrap import restore

class RestoreTests(unittest.TestCase):
    def bundle(self, config):
        from bootstrap import leaves
        return {'schema': 1, 'release': 'v2026.9.14', 'config': config, 'env': {'HERMES_MODEL_API_KEY': {'credential': 'model-api-key'}, 'CUSTOM_BASE_URL': ''}, 'ownedPaths': [list(path) for path, _ in leaves(config)], 'ownedEnv': ['HERMES_MODEL_API_KEY', 'CUSTOM_BASE_URL']}

    def test_restore_preserves_personal_data_and_reconciles_interrupted_ownership(self):
        with tempfile.TemporaryDirectory() as directory:
            home=Path(directory)
            (home/'config.yaml').write_text('agent:\n  system_prompt: personal\nretired: old\n')
            (home/'SOUL.md').write_text('personal')
            (home/'.env').write_text("UNRELATED='keep'\nCUSTOM_BASE_URL='old'\n")
            (home/'.operator').mkdir()
            (home/'.operator/pending.json').write_text(json.dumps({'ownedPaths': [['retired']], 'ownedEnv': []}))
            bundle=self.bundle({'model': {'default': 'cr-model', 'api_key': {'credential': 'model-api-key'}}})
            # Credential reference is one leaf in the bundle contract.
            bundle['ownedPaths']=[['model','default'],['model','api_key']]
            restore(home,bundle,{'model-api-key': "synthetic'key"})
            restore(home,bundle,{'model-api-key': "synthetic'key"})
            config=yaml.safe_load((home/'config.yaml').read_text())
            self.assertEqual(config['model']['api_key'], "synthetic'key")
            self.assertEqual(config['agent']['system_prompt'],'personal')
            self.assertNotIn('retired',config)
            self.assertEqual((home/'SOUL.md').read_text(),'personal')
            from dotenv import dotenv_values
            self.assertEqual(dotenv_values(home/'.env')['UNRELATED'],'keep')
            self.assertFalse((home/'.operator/pending.json').exists())

    def test_failure_keeps_pending_manifest_and_redacts_values(self):
        with tempfile.TemporaryDirectory() as directory:
            home=Path(directory)
            bundle=self.bundle({'model': {'default':'cr-model'}})
            with patch('adapters.v20260914.reset_model_overrides',side_effect=ValueError('secret-value')):
                with self.assertRaisesRegex(RuntimeError,'startup restore failed') as caught:
                    restore(home,bundle,{'model-api-key':'secret-value'})
            self.assertNotIn('secret-value',str(caught.exception))
            self.assertTrue((home/'.operator/pending.json').exists())

    def test_channel_routing_override_reset_keeps_personal_prompt(self):
        with tempfile.TemporaryDirectory() as directory:
            home=Path(directory)
            (home/'config.yaml').write_text(yaml.safe_dump({'gateway':{'platforms':{'telegram':{'channel_overrides':{'123':{'model':'temporary','provider':'temporary','system_prompt':'personal'}}}}}}))
            restore(home,self.bundle({'model':{'default':'declared'}}),{'model-api-key':'synthetic'})
            cfg=yaml.safe_load((home/'config.yaml').read_text())
            self.assertEqual(cfg['gateway']['platforms']['telegram']['channel_overrides']['123'],{'system_prompt':'personal'})
