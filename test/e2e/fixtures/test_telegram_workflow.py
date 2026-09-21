"""Offline verifier tests; never connect to Telegram or Kubernetes."""
import importlib.util
from pathlib import Path
import unittest
import asyncio
import tempfile
from types import ModuleType, SimpleNamespace
from unittest.mock import patch

path = Path(__file__).resolve().parents[3] / 'hack/e2e-telegram.py'
spec = importlib.util.spec_from_file_location('telegram_workflow', path)
workflow = importlib.util.module_from_spec(spec)
spec.loader.exec_module(workflow)

class AcceptanceProofTests(unittest.TestCase):
    def test_send_requires_explicit_opt_in(self):
        with self.assertRaises(ValueError): workflow.require_send({})
        with self.assertRaises(ValueError): workflow.require_send({'E2E_TELEGRAM_SEND':'yes'})
        workflow.require_send({'E2E_TELEGRAM_SEND':'yes','E2E_LIVE_DEDICATED':'yes'})

    def test_silence_without_transport_is_not_denial_proof(self):
        with self.assertRaises(ValueError): workflow.verify_denial(False, True, {'matches':0})
        with self.assertRaises(ValueError): workflow.verify_denial(True, False, {'matches':0})
        with self.assertRaises(AssertionError): workflow.verify_denial(True, True, {'matches':1})
        workflow.verify_denial(True, True, {'matches':0})
        before={'priorMessages':0,'usageTokens':{}}
        with self.assertRaises(AssertionError):workflow.verify_denial(True,True,{'matches':0,'priorMessages':1,'usageTokens':{}},before)
        with self.assertRaises(AssertionError):workflow.verify_denial(True,True,{'matches':0,'priorMessages':0,'usageTokens':{'model':1}},before)

    def test_reply_without_native_model_accounting_fails(self):
        valid={'matches':1,'identityMatch':True,'assistantNonce':True,'accountedModels':['declared'],'soulNonce':True,'userNonce':True}
        workflow.verify_positive(valid, 'declared', personalization=True)
        for key in ['identityMatch','assistantNonce','soulNonce','userNonce']:
            invalid=dict(valid,**{key:False})
            with self.assertRaises(AssertionError):workflow.verify_positive(invalid,'declared',personalization=True)
        with self.assertRaises(AssertionError):workflow.verify_positive(dict(valid,accountedModels=['wrong']),'declared')


    def test_wrong_account_never_sends(self):
        sends=[]
        class FakeClient:
            async def connect(self):pass
            async def disconnect(self):pass
            async def is_user_authorized(self):return True
            async def get_me(self):return SimpleNamespace(bot=False,id=999)
            async def send_message(self,*args,**kwargs):sends.append(args)
        module=ModuleType('telethon');module.__version__='1.45.0'
        module.TelegramClient=lambda *args:FakeClient();module.utils=SimpleNamespace()
        with tempfile.TemporaryDirectory() as folder:
            session=Path(folder)/'dedicated.session';session.touch(mode=0o600)
            config={'allowed_user_id':1,'unauthorized_user_id':2,'bot_id':3,'allowed_session':str(session),'unauthorized_session':str(session),'api_id':1,'api_hash':'synthetic'}
            with patch.dict('sys.modules',{'telethon':module}):
                with self.assertRaises(ValueError):asyncio.run(workflow.execute(config,{},None))
        self.assertEqual(sends,[])

if __name__=='__main__':unittest.main()
