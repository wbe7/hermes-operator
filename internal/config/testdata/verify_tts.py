"""Rendered Go bundle -> startup restore -> pinned native speech SDK/mode checks."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import yaml
sys.path[:0]=['/runtime','/opt/hermes']
from bootstrap import restore

native=r'''
import json,sys,threading,subprocess,types
from pathlib import Path
from http.server import BaseHTTPRequestHandler,HTTPServer
sys.path[:0]=['/runtime','/opt/hermes']
from bootstrap import load_runtime_environment
home=Path(sys.argv[1]);scenario=sys.argv[2];load_runtime_environment(home)
from hermes_cli.config import load_config
from gateway.run_voice import GatewayVoiceMixin
from gateway.config import Platform
from gateway.platforms.event import MessageType
from gateway.platforms.base import BasePlatformAdapter
from tools.tts_tool import text_to_speech_tool
cfg=load_config();tts=cfg['tts'];enabled=scenario!='disabled'
assert tts['provider']=='openai' and tts['use_gateway']==False
assert ('tts' in cfg['agent']['disabled_toolsets']) != enabled
assert cfg['voice']['auto_tts']==False
from model_tools import get_tool_definitions
toolsets=cfg.get('platform_toolsets',{}).get('telegram')
if scenario=='override':assert set(toolsets)=={'terminal','tts'}
schemas=get_tool_definitions(enabled_toolsets=toolsets,disabled_toolsets=cfg['agent']['disabled_toolsets'],quiet_mode=True,skip_tool_search_assembly=True)
assert ('text_to_speech' in {s['function']['name'] for s in schemas})==enabled
expected='off' if scenario in ('disabled','request') else 'all' if scenario=='override' else 'voice_only'
modes=json.loads((home/'gateway_voice_mode.json').read_text());assert modes and set(modes.values())=={expected},modes
assert 'telegram:99999999' not in modes, 'stale mode survived'
runner=GatewayVoiceMixin();runner._VOICE_MODE_PATH=home/'gateway_voice_mode.json';runner._voice_mode=runner._load_voice_modes()
adapter=types.SimpleNamespace(platform=Platform.TELEGRAM,_auto_tts_default=True,_auto_tts_enabled_chats={'99999999'},_auto_tts_disabled_chats=set())
adapter._should_auto_tts_for_chat=types.MethodType(BasePlatformAdapter._should_auto_tts_for_chat,adapter)
runner._sync_voice_mode_state_to_adapter(adapter)
runner._adapter_for_source=lambda source:adapter
runner._adapter_profile_for_source=lambda source:None
chat=next(iter(modes)).split(':',1)[1]
source=types.SimpleNamespace(chat_id=chat,platform=Platform.TELEGRAM)
for kind,want in [(MessageType.VOICE,expected!='off'),(MessageType.TEXT,expected=='all')]:
 event=types.SimpleNamespace(source=source,message_type=kind)
 assert runner._should_send_voice_reply(event,'Test',[],already_sent=True)==want
assert adapter._should_auto_tts_for_chat(chat)==(expected!='off')
assert not adapter._should_auto_tts_for_chat('99999999')
if not enabled:sys.exit(0)
# Real SDK speaks to a local server and receives a valid Ogg/Opus audio fixture.
audio=subprocess.check_output(['ffmpeg','-v','error','-f','lavfi','-i','sine=frequency=440:duration=0.2','-c:a','libopus','-f','ogg','pipe:1'])
received=[]
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*a):pass
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
  received.append((self.path,self.headers.get('Authorization'),body))
  self.send_response(200);self.send_header('Content-Type','audio/ogg');self.send_header('Content-Length',str(len(audio)));self.end_headers();self.wfile.write(audio)
server=HTTPServer(('127.0.0.1',18961),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
import os
os.environ['HERMES_SESSION_PLATFORM']='telegram'
try:result=json.loads(text_to_speech_tool('Проверка голоса.'))
finally:server.shutdown();server.server_close()
assert result.get('success'),result
assert Path(result['file_path']).read_bytes()[:4]==b'OggS'
assert len(received)==1,received
path,auth,body=received[0]
assert path==('/speech/v1/audio/speech' if scenario=='override' else '/v1/audio/speech')
key={'override':'separate-tts-key','none':'no-key-required'}.get(scenario,'SENTINEL${HOME}')
assert auth=='Bearer '+key
assert body['model']==('speech-custom' if scenario=='override' else 'fish-s2-pro')
assert body['voice']==('custom' if scenario=='override' else 'default')
assert body['response_format']=='opus'
assert body.get('speed')==(1.25 if scenario=='override' else None)
assert body.get('lang_code')==('ru' if scenario=='override' else None)
'''
with tempfile.TemporaryDirectory() as directory:
 home=Path(directory);(home/'SOUL.md').write_text('personal identity');(home/'workspace').mkdir();(home/'workspace/keep.txt').write_text('personal file')
 for scenario in ('inherited','override','request','none','disabled','inherited'):
  # Old ownership manifest deliberately has no TTS leaves.
  (home/'.operator').mkdir(exist_ok=True)
  (home/'.operator/committed.json').write_text(json.dumps({'ownedPaths':[],'ownedEnv':[]}))
  for old in (None,{'openai':None,'personal_note':'keep'}):
   (home/'config.yaml').write_text(yaml.safe_dump({'tts':old,'voice':{'auto_tts':True,'personal_note':'keep'}}))
   (home/'gateway_voice_mode.json').write_text(json.dumps({'telegram:99999999':'all'}))
   data=json.loads((Path('/fixtures')/(scenario+'.json')).read_text())
   restore(home,data['bundle'],data['credentials'])
   result=subprocess.run([sys.executable,'-I','-c',native,str(home),scenario],capture_output=True)
   assert result.returncode==0,(scenario,result.stderr.decode())
   cfg=yaml.safe_load((home/'config.yaml').read_text())
   if old is not None:assert cfg['tts']['personal_note']=='keep'
   assert cfg['voice']['personal_note']=='keep'
   assert (home/'SOUL.md').read_text()=='personal identity'
   assert (home/'workspace/keep.txt').read_text()=='personal file'
print('Native TTS SDK, response modes, disable/re-enable, null migration and personal state passed')
