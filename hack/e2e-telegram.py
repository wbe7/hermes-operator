#!/usr/bin/env python3
"""Test-only dedicated Telegram clients. No Bot API polling, login or token read."""
import asyncio
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import time
import uuid

CASES = ('allowed_dm', 'first_contact', 'personalization', 'unauthorized_dm',
         'denied_group_text', 'denied_group_command', 'denied_group_media',
         'allowed_group', 'old_inline_callback')


def require_send(env):
    if env.get('E2E_TELEGRAM_SEND') != 'yes' or env.get('E2E_LIVE_DEDICATED') != 'yes':
        raise ValueError('explicit send opt-in and dedicated identities are required')


def verify_positive(native, expected_model, personalization=False):
    assert native['matches'] > 0 and native['identityMatch'], 'native Telegram identity/message absent'
    assert native['assistantNonce'], 'native assistant response absent'
    assert expected_model in native['accountedModels'], 'no new accounting for declared model'
    if personalization:
        assert native['soulNonce'] and native['userNonce'], 'personalization was not actually persisted'


def verify_denial(delivery, barrier, native, before=None):
    if not delivery or not barrier:
        raise ValueError('missing delivery/control evidence; silence is not a denial pass')
    assert native['matches'] == 0, 'denied message reached native history'
    if before is not None:
        assert native['priorMessages']==before['priorMessages'], 'denied identity acquired native history'
        assert native['usageTokens']==before['usageTokens'], 'denied identity acquired model accounting'


def protected_file(filename):
    path = Path(filename).expanduser().resolve(strict=True)
    if not path.is_file() or stat.S_IMODE(path.stat().st_mode) & 0o077:
        raise ValueError('credential/session files must be private regular files (0600)')
    return path


class Native:
    def __init__(self):
        for key in ('E2E_LIVE_KUBECONFIG', 'E2E_LIVE_CONTEXT', 'E2E_LIVE_NAMESPACE', 'E2E_LIVE_HERMES'):
            if not os.environ.get(key): raise ValueError('missing '+key)
        ns = os.environ['E2E_LIVE_NAMESPACE']
        if not ns.startswith('hermes-operator-test'): raise ValueError('dedicated test namespace required')
        self.name = os.environ['E2E_LIVE_HERMES']
        self.k = ['kubectl', '--kubeconfig', os.environ['E2E_LIVE_KUBECONFIG'],
                  '--context', os.environ['E2E_LIVE_CONTEXT'], '-n', ns]

    def call(self, args):
        return subprocess.check_output(self.k + args, text=True, stderr=subprocess.PIPE, timeout=45)

    def cr(self):
        return json.loads(self.call(['get', 'hermes', self.name, '-o', 'json']))

    def observe(self, nonce, user, chat):
        # Read-only SQL and plain files only: never instantiate SessionStore.
        code = r'''
import json,sqlite3,sys
from pathlib import Path
nonce,user,chat=json.loads(sys.argv[1]);home=Path('/opt/data')
db=sqlite3.connect('file:/opt/data/state.db?mode=ro',uri=True)
rows=db.execute("SELECT m.session_id,s.user_id,s.chat_id,s.source FROM messages m JOIN sessions s ON s.id=m.session_id WHERE m.role='user' AND instr(COALESCE(m.content,''),?)>0",(nonce,)).fetchall()
assistant=any(db.execute("SELECT count(*) FROM messages WHERE session_id=? AND role='assistant' AND instr(COALESCE(content,''),?)>0",(row[0],nonce)).fetchone()[0]>0 for row in rows)
usage={}
for model,tokens in db.execute("SELECT u.model,SUM(u.input_tokens+u.output_tokens) FROM session_model_usage u JOIN sessions s ON s.id=u.session_id WHERE s.source='telegram' AND s.user_id=? AND s.chat_id=? GROUP BY u.model",(str(user),str(chat))):usage[model]=tokens
prior=db.execute("SELECT count(*) FROM messages m JOIN sessions s ON s.id=m.session_id WHERE s.source='telegram' AND s.user_id=? AND s.chat_id=?",(str(user),str(chat))).fetchone()[0]
def contains(path):return path.is_file() and nonce in path.read_text(errors='replace')
print(json.dumps({'matches':len(rows),'identityMatch':bool(rows) and all(str(r[1])==str(user) and str(r[2])==str(chat) and r[3]=='telegram' for r in rows),'assistantNonce':assistant,'usageTokens':usage,'priorMessages':prior,'soulNonce':contains(home/'SOUL.md'),'userNonce':contains(home/'memories/USER.md') or contains(home/'USER.md')}))
db.close()
'''
        return json.loads(self.call(['exec', self.name+'-hermes-0', '--',
            '/opt/hermes/.venv/bin/python', '-I', '-c', code, json.dumps([nonce, user, chat])]))

    def blocked(self, user, since):
        # Match a pinned upstream rejection line in-memory; never retain raw logs.
        text = self.call(['logs', self.name+'-hermes-0', '--since-time', since])
        return ('Blocked unauthorized user '+str(user)+' in chat '+str(user)) in text


async def execute(config, results, native):
    import telethon
    from telethon import TelegramClient, utils
    if telethon.__version__ != '1.45.0': raise ValueError('test-only Telethon==1.45.0 required')
    allowed_id, denied_id = int(config['allowed_user_id']), int(config['unauthorized_user_id'])
    bot_id = int(config['bot_id'])
    if min(allowed_id, denied_id, bot_id) <= 0 or len({allowed_id, denied_id, bot_id}) != 3:
        raise ValueError('distinct exact positive identities required')
    clients = []
    try:
        # Both sessions and every configured target are verified before any send.
        for key, expected in [('allowed_session', allowed_id), ('unauthorized_session', denied_id)]:
            session = protected_file(config[key])
            if session.suffix != '.session': raise ValueError('existing .session file required; no login workflow')
            client = TelegramClient(str(session), int(config['api_id']), config['api_hash'])
            clients.append(client)
            await client.connect()
            if not await client.is_user_authorized(): raise ValueError('session is not authenticated; no login performed')
            me = await client.get_me()
            if me.bot or me.id != expected: raise ValueError('dedicated session identity mismatch')
        allowed, denied = clients
        bots = [await client.get_entity(config['bot_username']) for client in clients]
        if any(not bot.bot or bot.id != bot_id for bot in bots): raise ValueError('exact bot identity mismatch')
        cr = native.cr()
        users = cr['spec']['telegram']['allowedUserIDs']
        if str(allowed_id) not in users or str(denied_id) in users: raise ValueError('CR sender allowlist mismatch')
        groups = cr['spec']['telegram'].get('groups', {})
        resolved = {}
        for key in ('denied_group_id', 'allowed_group_id'):
            if not config.get(key): continue
            target = int(config[key])
            if target >= 0: raise ValueError('exact negative Telegram group ID required')
            entity = await allowed.get_entity(target)
            if utils.get_peer_id(entity) != target: raise ValueError('group identity mismatch')
            permissions = await allowed.get_permissions(entity, bots[0])
            if not permissions.is_admin: raise ValueError('bot must already be group admin for delivery proof')
            await allowed.get_permissions(entity, allowed_id)
            admitted = groups.get('enabled', False) and str(target) in groups.get('allowedChatIDs', [])
            if admitted != (key == 'allowed_group_id'): raise ValueError('CR group allowlist mismatch')
            resolved[key] = entity
        expected = cr['spec']['model']['name']
        timeout = min(max(int(config.get('timeout_seconds', 180)), 30), 600)
        from datetime import datetime, timezone

        async def reply(client, target, message_id, nonce):
            end = time.monotonic()+timeout
            while time.monotonic() < end:
                async for message in client.iter_messages(target, min_id=message_id, limit=100):
                    if message.sender_id == bot_id and nonce in (message.raw_text or ''):
                        return message.id
                await asyncio.sleep(2)
            raise TimeoutError('no nonce-correlated bot response')

        async def positive(target, chat, personal=False):
            nonce = 'hermes-e2e-'+uuid.uuid4().hex
            before = native.observe(nonce, allowed_id, chat)
            prompt = 'Reply with exactly '+nonce+'.'
            if personal:
                prompt = ('Personalize this dedicated test agent: append the marker '+nonce+
                    ' to SOUL.md, and remember in your persistent user profile that my preferred name is '+nonce+
                    '. Preserve existing text. After both writes, reply with '+nonce+'.')
            if chat < 0: prompt = '@'+config['bot_username'].lstrip('@')+' '+prompt
            sent = await allowed.send_message(target, prompt, parse_mode=None)
            if not sent.id or sent.sender_id != allowed_id: raise ValueError('no authoritative sent event')
            response_id = await reply(allowed, target, sent.id, nonce)
            end = time.monotonic()+30
            while True:
                observed = native.observe(nonce, allowed_id, chat)
                observed['accountedModels'] = [m for m,tokens in observed['usageTokens'].items()
                    if tokens > before['usageTokens'].get(m,0)]
                try:
                    verify_positive(observed, expected, personal)
                    break
                except AssertionError:
                    if time.monotonic() >= end: raise
                    await asyncio.sleep(2)
            return {'sentMessageID':sent.id,'replyMessageID':response_id,
                    'nonceSHA256':hashlib.sha256(nonce.encode()).hexdigest(),
                    'nativeModel':expected,'priorMessages':before['priorMessages']}

        async def case(name, operation):
            try:
                proof = await operation()
                results[name] = {'status':'passed','evidence':proof}
            except (AssertionError, TimeoutError) as exc:
                results[name] = {'status':'failed','reason':type(exc).__name__}
            except Exception as exc:
                # No arbitrary exception strings: transport errors can contain private data.
                results[name] = {'status':'not-run','reason':'missing usable transport/native proof: '+type(exc).__name__}

        await case('allowed_dm', lambda: positive(bots[0], allowed_id))
        if results['allowed_dm']['status'] != 'passed': return
        results['first_contact'] = ({'status':'passed','evidence':results['allowed_dm']['evidence']}
            if results['allowed_dm']['evidence']['priorMessages'] == 0 else
            {'status':'not-run','reason':'dedicated account already has native Telegram history'})
        await case('personalization', lambda: positive(bots[0], allowed_id, True))

        async def negative(client, target, chat, user, kind):
            before_barrier = await positive(bots[0], allowed_id)
            nonce = 'hermes-e2e-denied-'+uuid.uuid4().hex
            before_native = native.observe(nonce, user, chat)
            since = datetime.now(timezone.utc).isoformat().replace('+00:00','Z')
            text = ('@'+config['bot_username'].lstrip('@')+' ' if chat < 0 else '')+nonce
            if kind == 'command': text = '/model@'+config['bot_username'].lstrip('@')+' '+nonce
            if kind == 'media':
                data = io.BytesIO(('Dedicated e2e fixture '+nonce).encode());data.name = 'hermes-e2e.txt'
                sent = await client.send_file(target, data, caption=text, force_document=True)
            else: sent = await client.send_message(target, text, parse_mode=None)
            if not sent.id or sent.sender_id != user: raise ValueError('server accepted event missing')
            await asyncio.sleep(10)
            after_barrier = await positive(bots[0], allowed_id)
            observed = native.observe(nonce, user, chat)
            verify_denial(True, bool(before_barrier and after_barrier), observed, before_native)
            if chat > 0 and not native.blocked(user, since):
                raise ValueError('native rejection log absent; no denial pass')
            if chat < 0:
                async for message in client.iter_messages(target, min_id=sent.id, limit=100):
                    assert message.sender_id != bot_id, 'forbidden group command/message produced a bot response'
            return {'sentMessageID':sent.id,'nonceSHA256':hashlib.sha256(nonce.encode()).hexdigest(),
                    'nativeMatches':observed['matches'],'controlBefore':before_barrier,'controlAfter':after_barrier,
                    'deliveryScope':'Telegram server event; verified bot admin for group; allowed-DM processing barriers'}

        await case('unauthorized_dm', lambda: negative(denied,bots[1],denied_id,denied_id,'text'))
        if 'denied_group_id' in resolved:
            for kind in ('text','command','media'):
                await case('denied_group_'+kind, lambda kind=kind: negative(allowed,
                    resolved['denied_group_id'],int(config['denied_group_id']),allowed_id,kind))
        if 'allowed_group_id' in resolved:
            await case('allowed_group',lambda:positive(resolved['allowed_group_id'],int(config['allowed_group_id'])))
    finally:
        for client in clients: await client.disconnect()


def main():
    out = Path(os.environ.get('E2E_DIR','/tmp/hermes-telegram-evidence'))
    out.mkdir(parents=True,exist_ok=True);out.chmod(0o700)
    results = {name:{'status':'not-run','reason':'dedicated credentials/target prerequisite absent'} for name in CASES}
    results['old_inline_callback']={'status':'not-run','reason':'accepted v1 limitation; existing offline regression, no live callback fixture supplied'}
    try:
        require_send(os.environ)
        config = json.loads(protected_file(os.environ['E2E_TELEGRAM_CONFIG']).read_text())
        asyncio.run(execute(config, results, Native()))
    except Exception as exc:
        results['preflight']={'status':'not-run','reason':'explicit opt-in or usable prerequisites absent: '+type(exc).__name__}
    (out/'telegram-results.json').write_text(json.dumps(results,indent=2));print(json.dumps(results))
    if any(case['status']=='failed' for case in results.values()):return 1
    if 'preflight' in results:return 2
    if any(results[name]['status']!='passed' for name in CASES[:7]):return 2
    return 0

if __name__=='__main__':raise SystemExit(main())
