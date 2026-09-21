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
            self.assertEqual(config['model']['api_key'], '${HERMES_MODEL_API_KEY}')
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

    def test_new_ownership_cannot_delete_personal_descendants(self):
        for value in (2, None):
            with self.subTest(value=value), tempfile.TemporaryDirectory() as directory:
                home=Path(directory)
                original='compression:\n  personal_note: keep\n'
                (home/'config.yaml').write_text(original)
                with self.assertRaises(RuntimeError):
                    restore(home,self.bundle({'compression':value}),{'model-api-key':'synthetic'})
                self.assertEqual((home/'config.yaml').read_text(),original)
                self.assertFalse((home/'.operator/pending.json').exists())

    def test_literal_environment_and_reserved_home(self):
        from bootstrap import load_runtime_environment
        import os
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {}, clear=True):
            home=Path(directory)
            literal='prefix${HERMES_MISSING_TEST_VARIABLE}suffix'
            (home/'.env').write_text("UNRELATED='"+literal+"'\nHOME='/tmp/escape'\nHERMES_HOME='/tmp/escape'\nHERMES_PROFILE='other'\nPYTHON_DOTENV_DISABLED='false'\n")
            restore(home,self.bundle({'model':{'default':'declared'}}),{'model-api-key':literal})
            load_runtime_environment(home)
            self.assertEqual(os.environ['HERMES_MODEL_API_KEY'],literal)
            self.assertEqual(os.environ['UNRELATED'],literal)
            self.assertEqual(os.environ['HOME'],str(home))
            self.assertEqual(os.environ['HERMES_HOME'],str(home))
            self.assertNotIn('HERMES_PROFILE',os.environ)
            self.assertEqual(os.environ['PYTHON_DOTENV_DISABLED'],'1')
            from dotenv import dotenv_values
            persisted=dotenv_values(home/'.env',interpolate=False)
            self.assertEqual(persisted['UNRELATED'],literal)
            self.assertNotIn('HOME',persisted)

    def test_startup_lock_denies_second_owner_without_mutation(self):
        from bootstrap import acquire_startup_lock
        import os
        with tempfile.TemporaryDirectory() as directory:
            home=Path(directory)
            lock=acquire_startup_lock(home)
            try:
                with self.assertRaises(RuntimeError):
                    restore(home,self.bundle({'model':{'default':'declared'}}),{'model-api-key':'synthetic'})
                self.assertFalse((home/'config.yaml').exists())
            finally:
                os.close(lock)
            restore(home,self.bundle({'model':{'default':'declared'}}),{'model-api-key':'synthetic'})

    def test_first_adoption_replaces_only_administrative_reasoning_map(self):
        for desired in ({}, {'declared-model': 'xhigh'}):
            with self.subTest(desired=desired), tempfile.TemporaryDirectory() as directory:
                home=Path(directory)
                (home/'config.yaml').write_text(yaml.safe_dump({'agent':{'reasoning_overrides':{'locally-selected-model':'low'},'system_prompt':'keep'},'display':{'personality':'personal'}}))
                restore(home,self.bundle({'agent':{'reasoning_overrides':desired}}),{'model-api-key':'synthetic'})
                config=yaml.safe_load((home/'config.yaml').read_text())
                self.assertEqual(config['agent']['reasoning_overrides'],desired)
                self.assertEqual(config['agent']['system_prompt'],'keep')
                self.assertEqual(config['display']['personality'],'personal')

    def test_telegram_pairing_grants_reset_in_both_layouts(self):
        for layout in ('pairing','platforms/pairing'):
            with self.subTest(layout=layout), tempfile.TemporaryDirectory() as directory:
                home=Path(directory); pairing=home/layout; pairing.mkdir(parents=True)
                (pairing/'telegram-approved.json').write_text(json.dumps({'999':{'user_name':'not allowed'},'123':{'user_name':'allowed'}}))
                (pairing/'telegram-pending.json').write_text(json.dumps({'pending':{'user_id':'999'}}))
                (pairing/'discord-approved.json').write_text('{"other": {}}')
                restore(home,self.bundle({'gateway':{'platforms':{'telegram':{'extra':{'allow_from':['123']}}}}}),{'model-api-key':'synthetic'})
                self.assertEqual(json.loads((pairing/'telegram-approved.json').read_text()),{'123':{'user_name':'allowed'}})
                self.assertEqual(json.loads((pairing/'telegram-pending.json').read_text()),{})
                self.assertEqual((pairing/'discord-approved.json').read_text(),'{"other": {}}')

    def test_corrupt_dotenv_preserves_exact_home_until_repaired(self):
        import contextlib, io
        for adopted in (False, True):
            with self.subTest(adopted=adopted), tempfile.TemporaryDirectory() as directory:
                home = Path(directory)
                (home/'config.yaml').write_text('agent:\n  system_prompt: personal\n')
                (home/'SOUL.md').write_text('personal')
                (home/'.env').write_bytes(b"PERSONAL='synthetic-secret-unterminated\r\n")
                if adopted:
                    (home/'.operator').mkdir()
                    (home/'.operator/committed.json').write_text('{"ownedPaths":[],"ownedEnv":[]}')
                    (home/'.operator/pending.json').write_text('{"ownedPaths":[],"ownedEnv":["OLD"]}')
                def snapshot():
                    return {str(p.relative_to(home)): p.read_bytes() for p in home.rglob('*') if p.is_file()}
                before = snapshot()
                output = io.StringIO()
                with contextlib.redirect_stderr(output), contextlib.redirect_stdout(output):
                    with self.assertRaisesRegex(RuntimeError, '^startup restore failed; gateway was not started$'):
                        restore(home, self.bundle({'model': {'default': 'declared'}}), {'model-api-key': 'synthetic'})
                self.assertEqual(output.getvalue(), '')
                self.assertEqual(snapshot(), before)
                self.assertEqual((home/'.operator').exists(), adopted)
                (home/'.env').write_text("PERSONAL='repaired-literal'\n")
                restore(home, self.bundle({'model': {'default': 'declared'}}), {'model-api-key': 'synthetic'})
                from dotenv import dotenv_values
                self.assertEqual(dotenv_values(home/'.env', interpolate=False)['PERSONAL'], 'repaired-literal')
                self.assertEqual(yaml.safe_load((home/'config.yaml').read_text())['model']['default'], 'declared')

    def test_deprecated_cwd_cleanup_first_adoption_and_previous_ownership(self):
        from bootstrap import load_runtime_environment
        import os
        for adopted in (False, True):
            with self.subTest(adopted=adopted), tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'MESSAGING_CWD':'/wrong', 'TERMINAL_CWD':'/wrong'}):
                home = Path(directory)
                (home/'.env').write_text("MESSAGING_CWD='/old'\nTERMINAL_CWD='/old'\nPERSONAL='keep'\n")
                if adopted:
                    (home/'.operator').mkdir()
                    (home/'.operator/committed.json').write_text(json.dumps({'ownedPaths': [], 'ownedEnv':['MESSAGING_CWD','TERMINAL_CWD']}))
                restore(home, self.bundle({'terminal': {'cwd': '/opt/data/workspace'}}), {'model-api-key':'synthetic'})
                load_runtime_environment(home)
                from dotenv import dotenv_values
                env = dotenv_values(home/'.env', interpolate=False)
                for key in ('MESSAGING_CWD','TERMINAL_CWD'):
                    self.assertNotIn(key, env)
                    self.assertNotIn(key, os.environ)
                    self.assertNotIn(key, json.loads((home/'.operator/committed.json').read_text())['ownedEnv'])
                self.assertEqual(env['PERSONAL'], 'keep')
