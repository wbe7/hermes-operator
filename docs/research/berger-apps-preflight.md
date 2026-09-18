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
