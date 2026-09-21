# Установка

## Предварительные условия

- Kubernetes, удовлетворяющий ограничению chart `>=1.34.0-0`, и рабочий default/выбранный StorageClass. Фактически проверенные версии и границы перечислены в [compatibility](../reference/compatibility.md); ограничение chart не обещает проверку каждой новой версии.
- CNI, который реально применяет ingress и egress NetworkPolicy для всех используемых IP families.
- Реальные Pod, Service, node и infrastructure CIDR, а также DNS selector и/или точные resolver IP. Хотя бы одна DNS-форма обязательна; каждое указанное направление нужно проверить.
- Доступ узлов к registry, Telegram и выбранному inference endpoint.

Скопируйте values и заполните сети вашего кластера. `enforcementConfirmed: true` — подтверждение администратора, а не автоматический тест CNI. Все четыре CIDR arrays обязательны; только `infrastructureCIDRs` может быть `[]`.

```yaml
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

`podSelector` и `resolverIPs` можно использовать вместе, например для Pod DNS и NodeLocal resolver. Оператор разрешает этим destinations только TCP/UDP 53; он не обнаруживает DNS автоматически и не открывает всю private-сеть.

Сохраните выбранные значения в `operator-values.yaml`. Установите одну копию
cluster-wide оператора на кластер. Релиз `0.1.0` экспериментальный; артефакты
публичны, отдельный registry Secret не требуется.

Скачайте файлы релиза, проверьте их контрольные суммы, затем примените CRD и chart:

```bash
gh release download operator-v0.1.0 --repo wbe7/hermes-operator \
  --dir hermes-operator-0.1.0
(cd hermes-operator-0.1.0 && sha256sum --check SHA256SUMS)
kubectl apply --server-side -f hermes-operator-0.1.0/hermes.crd.yaml
helm upgrade --install hermes-operator oci://ghcr.io/wbe7/charts/hermes-operator \
  --version 0.1.0 --namespace hermes-system --create-namespace \
  --values operator-values.yaml --wait --timeout 5m
kubectl rollout status deployment/hermes-operator -n hermes-system
```

На macOS вместо `sha256sum --check` доступен `shasum -a 256 --check`.
Все команды выполняются в выбранном kube-context; перед работой проверьте
`kubectl config current-context` или задайте kubeconfig/context явно.

Helm устанавливает CRD из `crds/` при первом install, но не обновляет существующую
CRD при `helm upgrade`. Перед каждым upgrade явно применяйте CRD **выбранной
версии**, затем обновляйте chart с сохранёнными cluster values. Не берите CRD из
произвольного текущего `main` для старого релиза. CRD/CR/PVC не удаляются как
способ обновления; совместимость схемы и возврата данных проверяется отдельно.

Для закрепления проверенного multiarch image `0.1.0` добавьте в values:

```yaml
image:
  tag: "0.1.0@sha256:6725c42f2bc0717dfdfc289fbeafae22402636d44bb4a4d5a12169444935991b"
```

Проверка установленной версии и сохранённых overrides:

```bash
helm history hermes-operator -n hermes-system
helm get values hermes-operator -n hermes-system
kubectl get deployment hermes-operator -n hermes-system \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

Для разработки можно использовать локальный `./charts/hermes-operator` и
собственный operator image через `image.repository`, `image.tag` и
`image.pullPolicy`. Это не требуется для установки опубликованного релиза.

Создайте namespace/RBAC из [примера](../../examples/namespace-admin-rbac.yaml), замените значения в [Secret template](../../examples/hermes-secret.yaml), затем примените [минимальный Hermes](../../examples/hermes-minimal.yaml). Secret и CR должны находиться в одном namespace.

```bash
kubectl apply -f examples/namespace-admin-rbac.yaml
kubectl apply -f examples/hermes-secret.yaml
kubectl apply -f examples/hermes-minimal.yaml
kubectl get hermes maria -n hermes-users -o yaml
```

Перед использованием замените все `REPLACE_WITH_...`, endpoint, Telegram sender IDs и RBAC subject. Ни fixture, ни chart не содержат настоящих credentials.

## Первое знакомство в Telegram

Дождитесь `Ready=True` у CR и напишите боту с разрешённого `allowedUserIDs`.
Первичная персонализация выполняется в диалоге: имя, стиль общения, SOUL, память
и свои skills. Эти данные остаются на PVC; управляемая модель/reasoning
восстанавливаются из CR перед каждым запуском.

На чистой инсталляции Hermes может сообщить `No home channel is set for Telegram`.
Это предложение выбрать чат для результатов cron и других уведомлений, а не
ошибка запуска. Отправьте `/sethome` в нужном разрешённом личном чате или
пропустите этот шаг. Выбор хранится в пользовательской конфигурации на PVC и
сохраняется после рестарта, если администратор явно не управляет этим полем.
