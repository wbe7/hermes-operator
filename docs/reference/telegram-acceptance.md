# Dedicated live Telegram acceptance

This is a test-only client workflow. It uses two already authenticated dedicated
**user** accounts through Telethon 1.45.0; it never starts another Bot API poller,
reads the bot token, logs in interactively, or changes the operator. A normal
local e2e run sends nothing. Missing prerequisites produce per-case `not-run`
and exit code 2, never a denial pass based solely on silence.

## Prerequisites and authorization

Use a disposable dedicated Hermes installation and dedicated accounts/groups,
with no concurrent user traffic. The permitted account must appear in the CR's
`telegram.allowedUserIDs`; the other account must not. For first-contact proof,
the permitted account must have no prior native Telegram messages in that home.
The existing gateway is the only consumer of its bot token. Its real provider
must be usable and must persist native model/token accounting.

Create user sessions separately, following the official
[Telethon session documentation](https://docs.telethon.dev/en/stable/concepts/sessions.html).
Session files grant account access: keep them and the client configuration mode
0600, outside Git/artifacts. This harness only opens existing `.session` files;
it does not request phone numbers, login codes or passwords. Install dependencies
in a separate test environment, never the operator or Hermes image:

```sh
python3 -m venv /tmp/hermes-telegram-client
/tmp/hermes-telegram-client/bin/pip install -r test/e2e/requirements-telegram.txt
```

A protected JSON configuration contains these keys (illustrative placeholders;
never commit an actual file):

```json
{
  "api_id": 12345,
  "api_hash": "provided-via-private-file",
  "allowed_session": "/private/path/allowed.session",
  "unauthorized_session": "/private/path/unauthorized.session",
  "allowed_user_id": 111111,
  "unauthorized_user_id": 222222,
  "bot_id": 333333,
  "bot_username": "dedicated_test_bot",
  "denied_group_id": -100444444,
  "allowed_group_id": -100555555,
  "timeout_seconds": 180
}
```

Group IDs are optional individually. To execute group cases, the bot must already
be an **administrator** in each dedicated group, and the allowed test account
must be a member. All configured numeric account, bot and group identities,
permissions and CR allowlists are verified before the first send. The denied
group must not be admitted by the CR; the optional allowed group must be admitted.
The harness never creates groups, invites users or changes their permissions.

Telegram documents that bot administrators receive all group messages, removing
privacy mode as a cause of non-delivery for these tests; see the
[official bot FAQ](https://core.telegram.org/bots/faq#what-messages-will-my-bot-get).
The test uses a server-accepted sent message ID, verified identities/admin rights,
and successful native allowed-DM processing before and after each negative case.
This is bounded delivery evidence, not a raw Bot API update capture. Telegram
transport errors or missing barriers produce `not-run`, not successful denial.

## Run explicitly

Set the existing scoped live inputs (`E2E_LIVE_KUBECONFIG`, `E2E_LIVE_CONTEXT`,
`E2E_LIVE_NAMESPACE` beginning `hermes-operator-test`, `E2E_LIVE_HERMES`,
`E2E_LIVE_DEDICATED=yes`) and choose a private `E2E_DIR`. Then explicitly authorize
sending by setting `E2E_TELEGRAM_SEND=yes` and point `E2E_TELEGRAM_CONFIG` to the
protected file. `E2E_TELEGRAM_PYTHON` selects the isolated environment's Python.
Run `make test-e2e-telegram`.

This sends bounded nonce prompts to the configured dedicated bot/groups, an
inert text attachment, and a `/model@bot` command in the denied group. The
personalization prompt asks the agent to append a test marker to SOUL and persist
a preferred name in USER memory; those deliberate test changes remain in the
disposable home. No real-user destination is inferred. No sending was executed
while implementing this workflow.

The [Telethon client API](https://docs.telethon.dev/en/stable/modules/client.html)
provides `get_me`, `get_entity`, `get_permissions`, `send_message`, `send_file`
and `iter_messages`; signatures were verified against installed 1.45.0 offline.

## What passes and what remains unverified

`telegram-results.json` contains separate cases:

- Allowed DM: authenticated bot response containing the unique nonce, native
  Telegram user/chat identity, user/assistant history and a new token-accounting
  delta for the CR model. An echoed transport response alone does not pass.
- First contact: allowed-DM evidence plus zero prior native messages for that
  identity. An existing conversation is explicitly `not-run` for this case.
- Personalization: the same response/model proof plus actual nonce bytes in SOUL
  and persistent USER memory. The agent saying it saved something does not pass.
- Unauthorized DM: accepted sent event, positive processing barriers, matching
  native rejection log in the bounded interval, and no nonce in native history.
- Denied group text, command and media: verified bot-admin delivery prerequisites,
  server accepted event and processing barriers, no native nonce and no bot reply.
- Optional allowed group: nonce-correlated transport/native response and model
  accounting. Missing group ID leaves only this case `not-run`.
- Old inline callback: remains explicitly `not-run` here. v1 accepts the upstream
  limitation whereby an authorized sender can change model/reasoning through an
  old picker in a forbidden group; existing offline regression covers that
  boundary. This workflow neither invents a callback fixture nor claims a live pass.

Output contains IDs, hashes, status and sanitized reasons, not credentials,
message content, session files, raw gateway logs or database exports. Raw rejection
logs are inspected only in memory. The existing snapshot harness remains separate
and uses read-only SQLite; neither helper constructs SessionStore against the
running gateway.

## Bounded manual test with one authorized chat

When the user authorizes only one existing bot chat, use that exact chat through
an authenticated Telegram client. Do not open other contacts/groups or start a
second Bot API poller. The automated two-account harness above remains a
separate workflow; a manual run is not a claim that it executed.

Before sending, record a read-only native baseline. Use unique, harmless nonce
prompts, verify the visible reply, correlate native Telegram user/chat identity
and a new token-accounting delta for the declared model, then inspect actual
personalization bytes. A claim in the reply alone is insufficient. After the
turn finishes, snapshot personal files/history and verify them across the
controlled restart before sending a recall prompt.

A second Kubernetes installation can be checked sequentially with the same
test-only credentials: suspend A and wait for its Pod to disappear, run B with a
separate namespace/PVC, then stop B before resuming A. Compare effective settings
and isolation markers. This does not establish simultaneous operation with
independent credentials. Unauthorized identity and group cases remain `not-run`
without their separately authorized prerequisites.

Executed results and scope: [fresh smoke, 2026-09-21](../research/fresh-smoke-2026-09-21.md).
