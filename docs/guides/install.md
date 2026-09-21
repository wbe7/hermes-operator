# Установка

## Предварительные условия

- Kubernetes 1.34 или новее и рабочий default/выбранный StorageClass.
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

Установите или обновите CRD явно, затем chart:

```bash
kubectl apply --server-side -f config/crd/bases/hermes.wbe7.github.io_hermes.yaml
helm upgrade --install hermes-operator ./charts/hermes-operator \
  --namespace hermes-system --create-namespace --values operator-values.yaml
kubectl rollout status deployment/hermes-operator -n hermes-system
```

Helm устанавливает CRD из `crds/` при первом install, но Helm не обновляет уже установленный CRD автоматически. Поэтому явный `kubectl apply` обязателен перед upgrade.

До публикации release image соберите operator image из этого репозитория, загрузите его в доступный кластеру registry или локальный cluster image store и задайте `image.repository`, `image.tag`, `image.pullPolicy`. Публичного гарантированного operator tag документация пока не обещает.

Создайте namespace/RBAC из [примера](../../examples/namespace-admin-rbac.yaml), замените значения в [Secret template](../../examples/hermes-secret.yaml), затем примените [минимальный Hermes](../../examples/hermes-minimal.yaml). Secret и CR должны находиться в одном namespace.

```bash
kubectl apply -f examples/namespace-admin-rbac.yaml
kubectl apply -f examples/hermes-secret.yaml
kubectl apply -f examples/hermes-minimal.yaml
kubectl get hermes maria -n hermes-users -o yaml
```

Перед использованием замените все `REPLACE_WITH_...`, endpoint, Telegram sender IDs и RBAC subject. Ни fixture, ни chart не содержат настоящих credentials.
