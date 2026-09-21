# Контракт установки и эксплуатации v1

Статус: экспериментальный operator/chart `0.1.0` опубликован, Berger Apps переведён на Helm. [Текущая установка](../guides/install.md), [доказательства](../research/release-0.1.0.md) и [оставшаяся приёмка](../research/v1-acceptance.md) описаны отдельно.

## Поддерживаемое окружение

- Linux Kubernetes. Целевая тестовая матрица первого релиза: Kubernetes 1.35, 1.36, 1.37; это цель испытаний, а не уже подтверждённая совместимость. Выбор следует поддерживаемым minor веткам на дату проектирования: [Kubernetes releases](https://kubernetes.io/releases/).
- CNI, реально применяющий ingress и egress NetworkPolicy, включая обе используемые IP families. Reference e2e — временный kind-кластер с Calico; default kind networking без проверки enforcement не подходит для сетевого доказательства.
- Dynamic provisioner либо заранее подготовленный Filesystem PVC. CSI должен поддерживать fsGroup или заранее подготовленный UID/GID 10000; SQLite требует корректных locking/fsync.
- Исходящий доступ к image registry, Telegram и выбранному inference. Доступ оператора к Kubernetes API — отдельно от egress агента.
- Контроллер: Go минимум 1.26.0, controller-runtime v0.25.1, Kubernetes Go modules v0.37.0, controller-tools v0.22.0. В сборке используются актуальные исправленные patch-релизы Go; зависимости фиксируются go.mod/go.sum. Версии совместимых модулей проверены по [controller-runtime go.mod](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.1/go.mod) и [controller-tools go.mod](https://github.com/kubernetes-sigs/controller-tools/blob/v0.22.0/go.mod).
- Hermes: amd64/arm64 только после smoke конкретного image digest на каждой публикуемой платформе. Наличие multiarch manifest не равно проверке обеих архитектур.

## Helm package

Исходники chart — `charts/hermes-operator`; опубликованный пакет — `oci://ghcr.io/wbe7/charts/hermes-operator`. Chart устанавливает Deployment оператора, ServiceAccount, cluster-wide RBAC, namespaced Lease RBAC для leader election и ConfigMap сетевых параметров. CRD поставляется отдельно в `config/crd/bases/hermes.wbe7.github.io_hermes.yaml`, а также включается в `crds/` chart для первого install.

Defaults оператора: одна реплика, leader election включён, requests cpu=100m/memory=128Mi, limits cpu=500m/memory=512Mi. Health endpoints доступны kubelet, metrics по умолчанию не публикуются через Service/Ingress. Для самого контроллера — non-root, drop ALL, seccomp RuntimeDefault, read-only root filesystem. Ему нужен собственный ServiceAccount token.

Две реплики оператора допустимы с leader election; это не увеличивает replicas Hermes. Лидер управляет CR во всех namespaces. Установка нескольких независимых операторов для одной API group не поддерживается.

Chart не меняет Pod Security labels существующих tenant namespaces автоматически и не создаёт широкие разрешающие NetworkPolicy для агентов. В deployment guide есть пример отдельного namespace с Pod Security Admission `restricted`.

## Установочные сетевые параметры

В каждой установке администратор задаёт полную сетевую конфигурацию. Её нельзя вывести только из CR агента.

```yaml
# Иллюстрация private cluster; заменить диапазоны фактическими перед установкой.
networkPolicy:
  enforcementConfirmed: true
  podCIDRs: ["10.244.0.0/16"]
  serviceCIDRs: ["10.96.0.0/12"]
  nodeCIDRs: ["192.168.1.0/24"]
  infrastructureCIDRs: []
  dns:
    podSelector:
      namespaceLabels:
        kubernetes.io/metadata.name: kube-system
      podLabels:
        k8s-app: kube-dns
    resolverIPs: ["10.96.0.10"]
```

`enforcementConfirmed` по умолчанию false: chart просит установщика подтвердить, что он выбрал CNI с enforcement. Это декларация установщика, не автоматическая аттестация сети. Все четыре CIDR arrays должны быть явно заданы; пустой infrastructureCIDRs допустим. Pod/Service/node arrays непустые и включают используемые IPv4/IPv6 сети. Отсутствующие параметры не заменяются выдуманными `10.*` defaults.

DNS: `podSelector` ограничивает namespace и Pod одновременно; `resolverIPs` разрешает точные IP только UDP/TCP 53, покрывая Service IP или NodeLocal DNS. Хотя бы одна форма обязательна; обе можно задать вместе, например для Pod DNS и точного NodeLocal resolver. Установщик обязан проверить каждое выбранное DNS-направление: автоматического discovery и общего разрешения private-сети нет. NodeLocal/host-network DNS требует реального теста поведения CNI. DNS exemption не открывает весь kube-system или всю node subnet.

Сети инфраструктуры с публичными адресами также включаются в infrastructureCIDRs. Иначе правило «разрешить публичный интернет» закономерно разрешит эти адреса. NAT64-only окружения исключены из первой матрицы, обычный IPv4 и native dual-stack проверяются отдельно.

Неверные CIDR, смешанная семья в ipBlock/except и неоднозначный DNS selector отвергаются при старте контроллера. В Helm values schema проверяется форма, в контроллере — семантика. Отсутствие валидной cluster network config запрещает создание agent workloads.

Изменение установочных сетей обновляет NetworkPolicy всех инсталляций без обязательного restart Hermes. Изменение набора разрешений не обещает немедленно закрыть уже установленные соединения на любом CNI.

## RBAC

| Субъект | Разрешения |
| --- | --- |
| Controller ClusterRole | Hermes get/list/watch; Hermes status и finalizers update/patch; создание/чтение/обновление/удаление StatefulSet, Service, ConfigMap, revision Secret, ServiceAccount, NetworkPolicy, созданных PVC. |
| Controller readers | Pods get/list/watch и delete только для исправления stuck rollout; Secret get/list/watch для references и ротации. Код должен читать содержимое только используемых Secrets; Kubernetes RBAC не выражает динамический список ссылок из всех CR. |
| Controller local Role | Lease leader election в namespace оператора; Events create/patch в обслуживаемых namespaces через соответствующий ClusterRole. |
| Hermes ServiceAccount | Без RoleBindings и без automounted token. |
| Namespace administrator/backend | Права на Hermes и исходные Secrets/PVC в назначенных namespaces; chart даёт пример Role/RoleBinding, не выдаёт доступ конечным пользователям. |

RBAC controller не включает cluster-admin, изменение Nodes, exec/attach Pod, namespaces delete, Role/RoleBinding create или wildcard resources/verbs. Контроллеру нужны широкие namespaced resource permissions по всему кластеру; Kubernetes не может ограничить delete Secret только generated-name шаблоном. Код проверяет owner UID перед мутацией зависимостей; именно эта проверка защищает чужие ресурсы от ошибки reconcile.

При watch Secret предпочтителен metadata-only cache и отдельный uncached GET выбранных ссылок. Это снижает объём содержимого Secrets в памяти, но не уменьшает фактические RBAC полномочия.

## Последовательность установки

1. Проверить CNI, storage class и реальные CIDR/DNS; подготовить values.
2. Установить CRD выбранного релиза и опубликованный OCI chart в отдельный namespace по [руководству установки](../guides/install.md). Оно содержит проверку контрольных сумм, закрепление версии/digest и команды upgrade. Локальный chart применяется для разработки.

3. Администратору создать namespace инсталляции и выделенный Secret. Получение Telegram bot token и model key — внешняя операция; оператор не генерирует эти значения.
4. Создать Hermes CR из примера и проверить его Conditions. Missing dependency диагностируется до запуска.
5. Проверить разрешённый DM, запрещённого sender и сетевой smoke из того же Pod context.

Документированные installation artifacts: минимальный CR/Secret template, custom inference с IP exception, existing PVC и namespace admin RBAC. Применяемые копии находятся в `examples/`; три Hermes CR проходят CRD admission test, а CI проверяет все примеры на временном API server без запуска фиктивных credentials.

## Обновление

- Обновление CRD выполняется явно перед chart upgrade. Helm не обновляет автоматически уже установленные CRD из `crds/`: [Helm CRD guide](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/).
- Изменение версии самого оператора не должно автоматически менять выбранный в каждом CR release Hermes. Изменение startup adapter вызывает новую revision тех workloads, которым оно нужно.
- Новый Hermes release добавляется в каталог после прохождения тестов; upgrade — изменение spec.version администратором. Во время обновления возможна пауза в Telegram.
- Возврат spec.version на прежнюю версию не объявляется rollback данных: upstream мог изменить SQLite/schema. В release notes указывается испытанный upgrade path и ограничения downgrade.
- Ротация исходного Secret создаёт новую revision и штатный rollout. Старые revision Secrets удаляются после исчезновения ссылок из Pod/StatefulSet.
- Недоступность оператора не останавливает уже работающих агентов. Новые изменения CR/Secret вступят в силу после восстановления reconcile.

## Удаление

Перед uninstall оператора администратор удаляет ненужные CR и дожидается завершения finalizers. При default Retain PVC остаются, Delete действует только на созданные оператором claims. Existing claims и исходные Secrets остаются всегда.

Helm uninstall сам по себе не должен удалять CRD, CR инсталляций или retained PVC. Если удалить controller раньше CR, обработать Delete/finalizers будет некому; штатный способ — вернуть оператор и завершить cleanup. Принудительное удаление finalizers не выдаётся за безопасную стандартную процедуру.

Удаление CRD уничтожает CR и запускает Kubernetes garbage collection зависимостей, обходя обычный пользовательский lifecycle; это отдельная разрушительная операция. Сохраняемый PVC не должен иметь ownerReference на удаляемый CR, но отсутствие ownerReference не защищает от удаления namespace.

## Доказательство сетевой изоляции

Тест запускается в выделенном временном кластере с реально включённым CNI. Контрольный Pod без restrictive policy сначала доказывает достижимость test destinations, чтобы timeout не был принят за успех запрета.

Затем из Pod инсталляции проверяются:

- DNS UDP и TCP к выбранному resolver, разрешённое публичное TCP/UDP назначение.
- Запрет достижимых private Pod, Service, node и infrastructure destinations.
- Отдельное исключение на один inference IP/port, запрет соседнего IP и другого порта.
- Link-local/metadata, IPv6 ULA/link-local и публичный IPv6 в dual-stack среде.
- Отсутствие Kubernetes SA token, host mounts и дополнительных capabilities.
- Поведение при удалении/reconcile policy и при наличии другой разрешающей policy.

Probe fixtures поднимаются на контролируемых адресах. Нельзя сканировать реальные private сети, metadata endpoints или чужую инфраструктуру ради теста.

Граница гарантий следует [NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/): политики складываются, API не гарантирует enforcement без CNI, порядок NAT и node traffic требуют проверки. Если конкретный CNI не закрывает hosting-node сервис, конфигурация не получает отметку прохождения этой проверки; используется документированное ограничение CNI или обычная защита узла. Добавление Kata/gVisor не является скрытым условием v1.

## Observability и поддержка

Сначала доступны Conditions, bounded Kubernetes Events, structured operator logs и стандартные controller-runtime metrics. Не добавляется отдельный observability stack или обязательный ServiceMonitor CRD.

Логи контроллера не содержат Secret values, полные CR payloads и user conversations. Необработанные сообщения upstream не копируются в status. Собственные логи Hermes принадлежат приложению: оператор не обещает отредактировать каждую строку неизменённого upstream.

Диагностическое руководство связывает reasons API с действиями: проверить dependency name/key, storage binding/permissions, image pull, bootstrap exit code, Telegram conflict, network policy или provider credentials. Не требует публиковать токены или содержимое домашних файлов.
