"""Real upstream intake/authorization with only transport and dispatch replaced."""
import importlib.util
import os
from types import SimpleNamespace
import unittest
from unittest.mock import AsyncMock, patch
from datetime import datetime, timezone

@unittest.skipUnless(importlib.util.find_spec('gateway'), 'requires official Hermes image')
class TelegramTests(unittest.IsolatedAsyncioTestCase):
    async def test_all_handler_paths_apply_sender_and_chat_allowlists(self):
        from telegram import Update, Message, Chat, User, Sticker
        from gateway.config import PlatformConfig, Platform
        from gateway.session import SessionSource
        from gateway.authz_mixin import GatewayAuthorizationMixin
        from plugins.platforms.telegram.adapter import TelegramAdapter
        env={'TELEGRAM_ALLOWED_USERS':'123', 'TELEGRAM_GROUP_ALLOWED_USERS':'', 'TELEGRAM_GROUP_ALLOWED_CHATS':'', 'GATEWAY_ALLOWED_USERS':'', 'GATEWAY_ALLOW_ALL_USERS':'false', 'TELEGRAM_ALLOW_ALL_USERS':'false'}
        with patch.dict(os.environ, env):
            for groups in (False,True):
                for user,chat,kind,expected in [(123,123,'private',1),(999,999,'private',0),(123,-999,'supergroup',0),(123,-456,'supergroup',int(groups)),(999,-456,'supergroup',0)]:
                    for handler in ('text','command','media','callback'):
                        with self.subTest(groups=groups,user=user,chat=chat,handler=handler):
                            adapter=TelegramAdapter(PlatformConfig(enabled=True,extra={'allow_from':['123'], 'group_allow_from':['123'], 'allowed_chats':['-456'] if groups else ['__operator_dm_only__'], 'guest_mode':False,'require_mention':False}))
                            runner=GatewayAuthorizationMixin()
                            runner.adapters={Platform.TELEGRAM:adapter}
                            adapter.set_authorization_check(lambda uid,ct,cid,**kw: runner._is_user_authorized(SessionSource(platform=Platform.TELEGRAM,user_id=uid,chat_type=ct,chat_id=cid)))
                            dispatch=[]
                            async def collect(*args,**kwargs):
                                dispatch.append(args)
                                return 'ok'
                            adapter.handle_message=collect
                            adapter._enqueue_text_event=lambda event: dispatch.append(event)
                            adapter._ensure_forum_commands=AsyncMock()
                            adapter._cache_replied_media=AsyncMock()
                            adapter._handle_sticker=AsyncMock() # fake media download/vision transport
                            msg=Message(message_id=1,date=datetime.now(timezone.utc),chat=Chat(id=chat,type=kind),from_user=User(id=user,first_name='Synthetic',is_bot=False),text=None if handler=='media' else ('/help' if handler=='command' else 'hello'), sticker=Sticker(file_id='synthetic',file_unique_id='synthetic',width=1,height=1,is_animated=False,is_video=False,type='regular') if handler=='media' else None)
                            if handler=='callback':
                                query=SimpleNamespace(data='cp:0',from_user=msg.from_user,message=msg,answer=AsyncMock(),edit_message_text=AsyncMock())
                                adapter._choice_picker_state[str(chat)]={'choices':[{'value':'low'}],'on_choice_selected':collect}
                                update=SimpleNamespace(callback_query=query)
                                await adapter._handle_callback_query(update,None)
                            else:
                                update=Update(update_id=1,message=msg)
                                await getattr(adapter, {'text':'_handle_text_message','command':'_handle_command','media':'_handle_media_message'}[handler])(update,None)
                            # Accepted v1 upstream limitation: a pending picker owned by an
                            # authorized sender bypasses allowed_chats. Strangers remain denied.
                            callback_expected = int(user == 123) if handler == 'callback' else expected
                            self.assertEqual(len(dispatch), callback_expected)
