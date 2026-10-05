"""Go bundle -> real restore/loaders -> real STT SDK against a local HTTP fixture."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

sys.path[:0] = ['/runtime', '/opt/hermes']
import yaml
from bootstrap import restore

native = r'''
import sys, json, threading, wave, struct, math
sys.path[:0] = ['/runtime', '/opt/hermes']
from pathlib import Path
from bootstrap import load_runtime_environment
from http.server import BaseHTTPRequestHandler, HTTPServer
from email.parser import BytesParser
from email.policy import default
home=Path(sys.argv[1]); scenario=sys.argv[2]
load_runtime_environment(home)
from hermes_cli.config import load_config
from gateway.config import load_gateway_config
from tools.transcription_tools import transcribe_audio, _get_provider
from tools.transcription_cloud import _resolve_openai_audio_client_config
cfg=load_config(); stt=cfg['stt']; gateway=load_gateway_config()
enabled=scenario!='disabled'
assert stt['enabled']==enabled and gateway.stt_enabled==enabled, (stt['enabled'],gateway.stt_enabled,cfg.get('stt_enabled'))
assert gateway.stt_echo_transcripts==(scenario!='override')
if not enabled:
 assert _get_provider(stt)=='none'
 assert not stt['openai']['api_key']
 sys.exit(0)
assert _get_provider(stt)=='openai'
key,url=_resolve_openai_audio_client_config()
assert key==({'none':'no-key-required','override':'separate-stt-key'}.get(scenario,'SENTINEL${HOME}'))
assert url==('http://127.0.0.1:18961/speech/v1' if scenario=='override' else 'http://127.0.0.1:18961/v1')
received=[]
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*a): pass
 def do_POST(self):
  body=self.rfile.read(int(self.headers['Content-Length']))
  msg=BytesParser(policy=default).parsebytes(('Content-Type: '+self.headers['Content-Type']+'\r\nMIME-Version: 1.0\r\n\r\n').encode()+body)
  fields={p.get_param('name',header='Content-Disposition'):p.get_payload(decode=True) for p in msg.iter_parts()}
  received.append((self.path,self.headers.get('Authorization'),fields))
  result=json.dumps({'text':'Проверка сорок два.'}).encode()
  self.send_response(200); self.send_header('Content-Type','application/json'); self.send_header('Content-Length',str(len(result)));self.end_headers();self.wfile.write(result)
server=HTTPServer(('127.0.0.1',18961),Handler)
threading.Thread(target=server.serve_forever,daemon=True).start()
audio=home/'probe.wav'
with wave.open(str(audio),'wb') as wav:
 wav.setparams((1,2,16000,0,'NONE','not compressed'))
 wav.writeframes(b''.join(struct.pack('<h',int(6000*math.sin(2*math.pi*440*n/16000))) for n in range(16000)))
try: result=transcribe_audio(str(audio))
finally: server.shutdown();server.server_close()
assert result.get('success') and result['transcript']=='Проверка сорок два.',result
assert len(received)==1
path,header,fields=received[0]
assert path==('/speech/v1/audio/transcriptions' if scenario=='override' else '/v1/audio/transcriptions')
assert header=='Bearer '+key
assert fields['model']==(b'asr-custom' if scenario=='override' else b'qwen3-asr-1.7b')
assert fields.get('language')==({'auto':None,'override':b'en'}.get(scenario,b'ru'))
assert fields['response_format']==b'json' and fields['file']
'''

with tempfile.TemporaryDirectory() as directory:
    home = Path(directory)
    (home/'SOUL.md').write_text('personal identity')
    (home/'workspace').mkdir()
    (home/'workspace/keep.txt').write_text('personal file')
    for scenario in ('inherited', 'override', 'auto', 'none', 'disabled', 'inherited'):
        # Simulate user edits and stale gateway settings before each restart.
        path = home/'config.yaml'
        current = yaml.safe_load(path.read_text()) if path.exists() else {}
        current.update(stt_enabled=True, stt_echo_transcripts=False)
        current['stt']={'enabled':True,'provider':'nous','use_gateway':True,'language':'de','echo_transcripts':False,
                        'openai':{'language':'fr','model':'wrong','base_url':'https://wrong.invalid/v1','api_key':'stale-key'},
                        'personal_note':'keep'}
        path.write_text(yaml.safe_dump(current))
        (home/'gateway.json').write_text(json.dumps({'stt_enabled':True,'stt_echo_transcripts':False}))
        (home/'.env').write_text("HERMES_LOCAL_STT_LANGUAGE='es'\nVOICE_TOOLS_OPENAI_KEY='unrelated-key'\nSTT_OPENAI_BASE_URL='https://wrong.invalid/v1'\n")
        data=json.loads((Path('/fixtures')/(scenario+'.json')).read_text())
        restore(home,data['bundle'],data['credentials'])
        result=subprocess.run([sys.executable,'-I','-c',native,str(home),scenario],capture_output=True)
        assert result.returncode==0, (scenario,result.stderr.decode())
        assert (home/'SOUL.md').read_text()=='personal identity'
        assert (home/'workspace/keep.txt').read_text()=='personal file'
        assert yaml.safe_load(path.read_text())['stt']['personal_note']=='keep'
        assert "unrelated-key" in (home/'.env').read_text()
print('STT native SDK, inheritance/overrides, language, keyless, disable/re-enable and persistence passed')
