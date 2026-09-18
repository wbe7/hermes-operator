# Berger Apps: проверка окружения

Дата: 2026-09-18. Пользователь разрешил отладку и deployment Hermes Operator в этом кластере. Проверка выполнена с явным kubeconfig `/Users/mtik/.kube/berger-apps.yaml`; default context не менялся.

## Наблюдения

| Параметр | Фактическое значение |
| --- | --- |
| Context / API | berger-apps / 192.168.0.202:6443 |
| Node | berger-apps-1, Ready, Kubernetes v1.34.7+k3s1, Linux amd64 |
| CNI backend annotation | flannel VXLAN |
| Node PodCIDR | 10.42.0.0/24 |
| Kubernetes Service IP | 10.43.0.1 |
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
