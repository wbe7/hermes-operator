# Berger Apps: проверка окружения

Дата: 2026-09-18. Пользователь разрешил отладку и deployment Hermes Operator в этом кластере. Проверка выполнена с явным kubeconfig `/Users/mtik/.kube/berger-apps.yaml`; default context не менялся.

## Наблюдения

| Параметр | Фактическое значение |
| --- | --- |
| Context / API | berger-apps / 192.168.0.202:6443 |
| Node | berger-apps-1, Ready, Kubernetes v1.34.7+k3s1, Linux amd64 |
| CNI backend annotation | flannel VXLAN |
| Node PodCIDR | 10.42.0.0/24 |
| Kubernetes Service IP / actual ServiceCIDR | 10.43.0.1 / 10.43.0.0/16 (ServiceCIDR resource) |
| Cluster DNS Service IP | 10.43.0.10, kube-system, k8s-app=kube-dns |
| Storage classes | openebs-lvm-fast, openebs-lvm-bulk, openebs-lvm-runner-ephemeral |
| Storage capabilities | WaitForFirstConsumer, expansion enabled |
| Test namespace | hermes-operator-test, PSA restricted/v1.34, sidecar injection disabled |

Node PodCIDR не является доказательством полного cluster Pod CIDR; перед final values необходимо проверить install-time cluster CIDRs. Доступность IP не означает разрешение обращаться к существующим внутренним сервисам при сетевых тестах.

## NetworkPolicy enforcement

Созданы два ограниченных тестовых Pod: network-target (Python HTTP server :8080) и network-client, оба UID/GID 10000, без SA token, capabilities и writable root filesystem.

1. Прямое HTTP-соединение client → target: 200.
2. Применена policy только к network-client: Egress, пустой список разрешений.
3. Target продолжает отвечать локально: 200; соединение от client отклоняется errno 111.
4. Удалена только тестовая policy; client → target снова 200.

Вывод: egress NetworkPolicy применяется в этом окружении, с REJECT вместо ожидавшегося timeout. Первоначальный тест, принимавший только timeout, завершился ошибкой проверки; контроль target и повтор после снятия policy подтвердили сетевой запрет.

Это доказательство одного правила на контролируемых Pod. Публичный доступ, исключения IP, DNS, Service DNAT и hosting-node поведение требуют теста окончательно сгенерированных policies. Приватные рабочие сервисы не сканировались.

## Runtime staging

Созданы runtime-smoke Pod и отдельный runtime-smoke-data PVC, 10Gi, openebs-lvm-runner-ephemeral. PVC успешно Bound при WaitForFirstConsumer. Pod использует официальный pinned Hermes image и запускает только Python sleep, без credentials и Telegram. Сначала создана ingress/egress deny-all policy для этого Pod.

Официальный amd64-контейнер успешно перешёл в Running. Проверено внутри контейнера: UID/GID 10000, запись/чтение тестового файла на PVC, отсутствует ServiceAccount token, CapEff=0, NoNewPrivs=1. Фактический запуск gateway, сохранность сессий и probes этим staging не подтверждаются; их результаты записываются отдельно после исполнения runtime tests.

## Локальная сборочная среда

Docker Desktop работает на linux/arm64. Pinned official image успешно скачан по digest из спецификации. Docker credential helper завис при чтении credentials для публичного pull; использован отдельный пустой временный Docker config для anonymous public pull, основной credential store не менялся.

Для Go подготовлен изолированный toolchain go1.27.1 darwin/arm64; системный go1.25.5 не менялся. Build caches расположены в /tmp/hermes-go-mod и /tmp/hermes-go-cache.

## Выделенные development credentials

По прямому запросу пользователя значения сохранены только в игнорируемом .env (mode 0600) и в Secret hermes-smoke-hermes-secret в тестовом namespace. Read-only Telegram getMe успешен; model list содержит qwen38-27b. Короткий запрос к предоставленному inference с reasoning_effort=xhigh вернул HTTP 200. Это подтверждает принятие параметра API, но не измеряет фактическую глубину reasoning модели. Из кластера hostname inference.s2technologies.ru разрешается в 192.168.0.210; требуется точечное разрешение TCP/443.

## Runtime egress smoke

From the restricted official-image test Pod with a smoke policy: TLS to `api.telegram.org` and `inference.s2technologies.ru` succeeded; DNS to `10.43.0.10` passed over UDP and TCP. The controlled private target `10.42.0.228:8080` was rejected (`ECONNREFUSED`, errno 111), while its unrestricted control had returned HTTP 200. The inference exception is the exact address `192.168.0.210` on TCP 443. This is preliminary CNI/runtime evidence; final generated-policy acceptance remains separate.

## Native gateway on amd64

After Task1 review fixes (`1f52a21`), the sleep-only smoke Pod was replaced with a gateway Pod using the same PVC. It reached `Running/Ready` with zero restarts, UID `aef8974a-9c88-4b29-baa4-33808a42143a`. Readiness invokes the committed adapter and confirms live Telegram connection/session storage. Runtime scripts are an immutable ConfigMap; inputs and selected credentials are separate read-only mounts. This is a runtime integration Pod, not yet a controller-created workload.

## Development build prerequisite

A temporary restricted Pod compiled and executed a minimal static Go binary as UID/GID 65532, with read-only root, dropped capabilities and no ServiceAccount token. Output: `go version go1.27.1 linux/amd64`, `rootless source build PASS`; Pod phase Succeeded, then cleaned up. Official builder digest: `docker.io/library/golang@sha256:4cb7ac979db5fcc41cae44b2227ba5ab8a51e8807f40d9ba4dee20a0ad960b5b`. This permits a development-only source-build deployment when no operator image has been published; production installation still uses the operator Dockerfile/release image.

Effective native loaders in the amd64 gateway Pod confirmed the model `qwen38-27b`, provider `custom`, declared endpoint and exact mounted credential, reasoning `xhigh`, Telegram token and user allowlist, disabled groups/multiplexing. Credential values were compared inside the Pod and not printed.

Same-Pod restart passed: a synthetic native SessionStore session with a temporary model override/transcript and a personal config leaf was created; model/reasoning were changed temporarily, then PID1 received SIGTERM. Pod UID stayed `aef8974a-9c88-4b29-baa4-33808a42143a`, restartCount changed `0 -> 1`, last exit code was `1`, and readiness recovered. Native resolver returned the declared model/credential and `xhigh`; the same session ID/transcript remained, active model override was cleared, and 60 file hashes (including SOUL, workspace marker and installed skills) were unchanged. A personal extra config leaf also remained. Test scripts: `/tmp/hermes-operator-deploy/restart-fixture.py`, `verify-restart.py`; these are local integration evidence, not yet the committed final e2e harness.

## Known cluster API destinations

Two bounded TCP connect/close checks used the already known API endpoints of this authorized cluster, with no HTTP request, credentials or range scan. From unrestricted network-client both `10.43.0.1:443` and hosting-node `192.168.0.202:6443` were reachable. From runtime-smoke with its isolation policy both connections were rejected with errno 111. This verifies those specific Service/node routes on current k3s enforcement; it does not generalize to every hosting-node service or another CNI.

## API admission

Committed API schema `27e7ceb`, CRD SHA256 `8d78e92e314c0795fe0d61861d61be9804dd8a7dac37a8fd2111e14a39d2fc22`, was installed by server-side apply and Established on Kubernetes `v1.34.7+k3s1`. All three design examples passed server-side dry-run after substituting only the test namespace. Returned objects confirmed `xhigh`, `Retain`, disabled groups, agent budgets 50/600, terminal timeout 300, requests 100m/512Mi/256Mi and limits 2/2Gi/2Gi. No Hermes CR was actually created by these dry-runs. An earlier development schema failed the optional group-list CEL expression; that failure was fixed and covered by the final envtest and live pass.

The committed resource-positivity fix was also verified on the live API with four separate server-side dry-runs: zero/negative CPU requests and zero/negative CPU limits were all rejected.

## Exact private exception

The controlled network-target exposed TCP 8080 and 8081; unrestricted network-client connected to both. A temporary rule allowed only the target's exact `/32` on TCP 8080 for runtime-smoke: 8080 became reachable while 8081 remained denied. The original policy was restored in a `finally` block, then denial of 8080 was confirmed again. Script: `/tmp/hermes-operator-deploy/check-exact-egress.py`. No other destination was opened by this test.

Public UDP egress was checked with one DNS query for `example.com` to `1.1.1.1:53`; both unrestricted control and isolated runtime-smoke received a valid response. No user content was sent.

For development installation, a conservative configured pod deny aggregate `10.42.0.0/16` contains the observed allocation `10.42.0.0/24`; this is not reported as independently verified server allocation. The baseline already denies all RFC1918 space. Service CIDR `10.43.0.0/16` is API-verified; `192.168.0.0/24` contains the known node and local infrastructure. Changes to cluster/public infrastructure networks remain installer-managed inputs.

## Native inference request

The real official Hermes CLI in the restricted gateway Pod completed a one-shot request using its restored native configuration (no command-line model/provider override), returning `готов` and exit code 0. It used a hidden `tool` source session and ignored user rules for this bounded smoke. No Telegram message was sent. This verifies the native Hermes request pipeline to the supplied inference with the configured `xhigh` default, beyond the direct HTTP API check.

## Generated compiler policy on the actual Hermes Pod

After Task4 review (`90c5ce8`), the policy compiled from the real Hermes CR and validated installation settings was applied as `hermes-smoke-hermes`. The manual gateway Pod received the CR UID selector label; the earlier hand-built `runtime-smoke-isolation` policy was removed. Only the generated policy remained for this installation.

An unrestricted fixture proved both controlled target ports 8080/8081, Service API `10.43.0.1:443` and hosting-node API `192.168.0.202:6443` reachable. From the actual Hermes container all four were rejected with errno 111. Exact inference `192.168.0.210:443`, configured DNS UDP/TCP and public TCP/UDP remained reachable. Script `check-compiled-network.py` and `network-compiled-results.json` are in `/tmp/hermes-operator-deploy`. This is actual generated-policy CNI proof on Berger Apps; creation by the reconciler still awaits Task6.

Later observation found two additional liveness-triggered restarts beyond the deliberate restart test (last at 13:26:31 UTC, count 3). No OOM or application traceback was present in previous logs. Twenty subsequent live-probe calls all passed, with no further restart during the bounded check; the cause is not declared fixed. Final workload probes use the specified 30-second liveness interval rather than the preliminary manual 10 seconds. Further runtime acceptance must track this observation.

## Builder-based StatefulSet runtime

Reviewed Task5 builders (`049109d`) created all six resources with zero replicas first. The manual gateway stopped fully before the CR was resumed and the StatefulSet activated. The new Pod `hermes-smoke-hermes-0` reached Ready, UID `cbbc1fe9-55cb-41b7-bd95-f71a3c53996a`, revision `e003e14b3a56e23a`, using the same externally owned PVC. It runs current Task3 runtime scripts in `hermes-smoke-rt-815e9c2f43c99722`; immutable inputs use the same final revision. No controller was running for this stage: a local typed API helper applied builder output after checking policy identity.

Before replacement, 335 file hashes were captured. Only `cron/ticker_heartbeat` and `cron/ticker_last_success` changed; the original `cron/jobs.py` writes these periodic operational timestamps. All remaining 333 personal files, native session ID/history and personal config leaf were unchanged. The effective native resolver confirmed declared model, xhigh and mounted credential.

A subsequent same-Pod SIGTERM test passed with current generated resources: Pod UID unchanged, restart count `0 -> 1`, readiness recovered, administrative model/reasoning restored, and the 333 hashes/session/history preserved. Generated-policy network checks also passed from the new Pod. A real native CLI request then returned `готов`, exit 0, with the restored configuration. It emitted deprecation notices for operator-provided MESSAGING_CWD/TERMINAL_CWD environment settings; these are recorded for cleanup, not hidden as pristine output. No Telegram message was sent by this check.

## Controlled Telegram cold-start outage

On the builder-based Pod, public egress was temporarily removed while DNS and the exact inference exception remained. After ordinary SIGTERM, the same Pod restarted once and remained running throughout 11 observations over roughly 110 seconds. Readiness stayed false. Local liveness became true around the fifth 10-second sample; the test initially asserted a shorter 30-second threshold and failed that assertion. This was within the configured 600-second startup budget, not a kubelet liveness failure. The original generated policy was restored in `finally`. Native Telegram retry connected at 14:27:35 UTC; readiness and local probes then returned true, restartCount 3 unchanged. Evidence: `check-telegram-outage.py`, `telegram-outage-results.json`.

The baseline before this test was already restartCount 2, reflecting a separate liveness-triggered restart at 14:19:31 UTC. That unexplained prior event remains an investigation item; the outage test does not claim to fix it.

## Liveness investigation: test helper polluted process identity

The prior restarts were traced to an integration assertion helper. Constructing upstream `SessionStore` from a separate `kubectl exec` process invokes `gateway/session_db_recovery.py:_publish_health`, which calls `write_runtime_status(session_store=...)`. That replaces the top-level status identity with the short-lived assertion process. Our probes correctly reject it as not the running gateway.

A bounded live reproduction began with healthy PID 1; constructing/closing SessionStore changed status PID to exec PID 426 and both probes returned false. Restoring only this test-polluted status snapshot in `finally` immediately returned both true. No upstream or operator runtime patch was applied. The post-restart helper now uses read-only SQLite/JSON instead; native model/credential/xhigh, same session ID/history and all 333 file hashes passed again without altering status PID 1.

A clean ordinary SIGTERM restart, without the old assertion helper, was observed for 25 samples over roughly four minutes: restartCount `3 -> 4` once, heartbeat age below 30 seconds, local loop witness responding and both probes true after startup. Evidence: `sample-health-restart.py`, `health-restart-samples.json`, `reproduce-sessionstore-health.py`, `sessionstore-health-repro.txt`, corrected `verify-builder-state.py` under the temporary deployment directory. Final e2e assertions must avoid constructing SessionStore against a live gateway; fixture creation before immediate controlled shutdown remains appropriate.

## Controller deployment and live lifecycle acceptance

The reviewed controller (`9b367e3`, exact source snapshot `dbd3f12`) was deployed to `hermes-operator-system` on September 18 using the documented development-only rootless source-build approach. Initial `/tmp` size 128Mi was insufficient for Go compilation and the build Pod was evicted; increasing only development build `/tmp` to 2Gi (cache 3Gi), cleaning the terminated build Pod, and retrying succeeded. This is an executed source deployment, not a published operator image or Helm installation.

Controller adoption passed without replacing the existing gateway Pod or external PVC. The CR acquired its finalizer, applied revision and current Ready conditions; storage origin is Existing. Evidence: `controller-adoption.json`.

A combined invalid-version/credential revocation test passed: unsupported version reported NotReady while preserving the previous Pod; removal of the development source Secret then scaled the StatefulSet to zero and the old Pod disappeared normally. Recreating the same credential values kept it stopped while the CR was invalid. Restoring the original version produced a new revision/Pod and recovered Ready on the same PVC. Values stayed in memory and were never printed or written to the report. Evidence: `check-controller-revocation.py`, `controller-revocation.json`. New Pod UID `c6b47bf9-2a8a-48ac-a3f5-febc51dce575`, revision `2b74bf710af5de27`, unchanged PVC UID `a0a97776-8dd2-436c-b8fc-6e4aade348b9`.

In separate `hermes-operator-test-secondary`, synthetic unschedulable installations proved cluster-wide reconciliation and WaitForFirstConsumer behavior: a Pending PVC did not prevent StatefulSet creation. A controlled rootless consumer bound the volume, wrote a marker and retained it across actual CSI expansion from 1Gi to 2Gi. Default Retain preserved the same claim UID after CR deletion. Explicit attachment as existingClaim preserved it after deletion; admission rejected Delete policy for existingClaim. A separate operator-created Delete-mode claim was removed after normal Pod shutdown. Synthetic fixture claims were cleaned up only after these assertions. Evidence: `check-storage-lifecycle.py`, `storage-lifecycle.json`; all cases passed.

Fresh observation on September 21: controller and gateway both Ready, zero container restarts after approximately 2 days 18 hours. Original version, xhigh and active state were restored; the external 10Gi PVC remains Bound. No actual test-user Telegram message is recorded yet; native request/authorization tests must not be presented as proof of a real user's DM conversation.

On September 21, the corrected read-only persistence check passed again after controller-managed replacement: declared model/xhigh/selected credential, original native session ID/history and all 333 personal file hashes remained. A deliberate manager-container SIGTERM then proved restart reconstruction without changing the gateway Pod UID, StatefulSet resourceVersion, applied revision or PVC identity. Adding/removing an unused source-Secret key likewise caused no workload rollout. The controller main container restart count is now one solely due to this test; gateway remains zero. Evidence: `check-manager-restart.py`, `manager-restart.json`.

## Provider connectivity outage and ServiceAccount permissions

On September 21, removing only the exact inference exception from the test CR caused the reconciler to deny TCP connectivity to `192.168.0.210:443`. Across six observations over approximately 100 seconds, local liveness and Telegram readiness remained true; Pod UID, applied revision and restart count (zero) stayed unchanged. The original exception was restored in `finally`, then endpoint reachability was confirmed. Evidence: `check-provider-outage.py`, `provider-outage.json`. This checks provider unavailability while the gateway is idle; it does not claim an active conversational request or provider-retry test.

Actual API authorization checks, impersonating the agent ServiceAccount `hermes-operator-test:hermes-smoke-hermes`, returned `no` for reading Secrets and creating Pods in its namespace. The agent's missing token mount and restricted runtime were independently verified earlier. These checks describe the tested cluster's effective grants; unrelated future RBAC bindings could change them.

## Corrupt home and permission failures on a disposable PVC

Three September 21 cases used a new operator-created 1Gi Delete-mode PVC in `hermes-operator-test-secondary`, fake credentials and the original pinned amd64 Hermes image. The real StatefulSet container failed with exit 1 and the sanitized `startup restore failed; gateway was not started` diagnostic for malformed YAML, non-SQLite database bytes, and a nonwritable `.operator` directory. None reported Ready or disclosed the fake token.

After normal suspension, a restricted helper confirmed unchanged SOUL/workspace markers and the unchanged failed input (including permissions). Only the deliberately broken fixture was repaired. A new original-image Pod with the exact generated runtime/config/credential mounts then executed `bootstrap.py --restore-only`, exited 0, restored the declared model and committed ownership, and preserved the personal markers. This proves bootstrap recovery on the actual volume; fake credentials do not establish Telegram readiness.

All three cases passed. The CR finalized normally and its exact created PVC was deleted; the temporary fake Secret and helper Pods were removed. The real `hermes-smoke` home was not involved. Evidence: `check-corrupt-home.py`, `corrupt-home.json` under `/tmp/hermes-operator-deploy`.

## Retiring an extra managed leaf

A separate disposable created PVC verified A06 with the real reconciler and original-image `--restore-only` Pods. `extraConfig.compression.threshold: 0.61` produced an immutable input revision and was applied alongside a pre-existing `compression.personal_note` and SOUL marker. Removing `extraConfig` from the CR produced a different revision. On the next restoration the previously managed threshold disappeared, while its personal sibling and SOUL remained on the same PVC. Both restoration Pods exited 0. No Telegram consumer or real credentials were needed for this startup ownership check. The CR, exact Delete-mode claim and helpers were removed normally after assertions. Evidence: `check-extra-removal.py`, `extra-removal.json`.
