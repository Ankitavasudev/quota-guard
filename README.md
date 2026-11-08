# quota-guard

A Kubernetes operator that watches per-namespace CPU and memory **request budgets** and keeps the namespace honest.

Declare one `ResourceBudget` per namespace, and quota-guard continuously sums the pod requests scheduled there, writes the result back into the CR's status, emits a Kubernetes event and Prometheus metrics, and flips a `WithinBudget` condition — so a namespace that quietly outgrows its budget is visible in `kubectl get rb`, not in a surprise bill.

![CI](https://github.com/Ankitavasudev/quota-guard/actions/workflows/ci.yml/badge.svg)
![Go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)
![license](https://img.shields.io/badge/license-MIT-green)

## Why this exists

Kubernetes lets you set per-container `requests`, but it never tells you that the *namespace* as a whole has outgrown the requests you reserved. kubecost tells you the cost; the scheduler refuses to place new pods when requests lie. quota-guard is the missing signal between those two:

- **Input:** one `ResourceBudget` custom resource per namespace.
- **Observed:** sum of `resources.requests` across all Running pods in the namespace.
- **Output:** CR status (`observedCpu`, `observedMemory`, `pods`, `withinBudget`, conditions), an Event on transition, and three Prometheus gauges.

It deliberately does **not** block scheduling — that's what ResourceQuota and admission webhooks are for. quota-guard gives you the visibility layer: run it next to ResourceQuota, or against namespaces where quota hasn't been set yet.

## Architecture

```
┌─────────────────────┐        watches        ┌────────────────────────┐
│  ResourceBudget CR  │ ───────────────────►  │  quota-guard operator  │
│  (per namespace)    │                       │                        │
└─────────────────────┘                       │  reconcile loop:       │
                                                │  1. list pods          │
┌─────────────────────┐        watches        │  2. sum requests       │
│  core/v1 Pods        │ ───────────────────►  │  3. compare w/ budget  │
└─────────────────────┘                        │  4. write status       │
                                                │  5. emit Event + metrics│
                                                └────────────────────────┘
```

Built with [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime): one reconciler, a predicate filter on Pod events, and a manager with a metrics endpoint and health probes.

## Quickstart (kind)

```bash
# create a local cluster
kind create cluster --name quota-guard-demo

# apply the CRD + RBAC + controller
make deploy

# create a demo namespace with a budget of 1500m CPU / 2Gi memory
make demo-apply

# watch the status fill in
watch kubectl get rb -n demo
```

Example output:

```
NAME   CPU USED   MEMORY USED   PODS   WITHIN BUDGET
main   500m       512Mi         2      true
```

If the budget gets exceeded you'll see an Event:

```
quota-guard  Warning  OverBudget  pod requests (1500m CPU, 3.5Gi memory) exceed the budget (1500m CPU, 2Gi memory)
```

## CRD

```yaml
apiVersion: guard.ankaita.dev/v1alpha1
kind: ResourceBudget
metadata:
  name: main
spec:
  cpu: "1500m"
  memory: "2Gi"
```

| Field | Meaning |
|---|---|
| `spec.cpu` | Aggregate CPU request capacity for the namespace (`500m`, `4`, …) |
| `spec.memory` | Aggregate memory request capacity (`512Mi`, `2Gi`, …) |
| `status.observedCpu` | Sum of pod CPU requests right now |
| `status.observedMemory` | Sum of pod memory requests right now |
| `status.withinBudget` | `true` while pods fit |
| `status.conditions[WithinBudget]` | `True`/`False` + reason (`WithinBudget`/`OverBudget`) |

Succeeded/failed pods are excluded; only Running pods count toward the budget.

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `quota_guard_cpu_request_millicores` | gauge | `namespace` |
| `quota_guard_memory_request_bytes` | gauge | `namespace` |
| `quota_guard_over_budget` | gauge | `namespace` (1/0) |

## Development

```bash
make test   # unit tests (fake client — no cluster needed)
make vet
make build
```

Tests use `controller-runtime/pkg/client/fake`; they cover within/over budget, cross-namespace isolation, succeeded-pod exclusion, missing-budget no-op, and condition wiring.

## What this demonstrates

- Writing a real Kubernetes operator: CRD schema, status subresource, reconcile loop, events, metrics
- API design: one small custom resource, clear spec/status split
- Production plumbing: health/readiness probes, RBAC, distroless image, CI, kustomize-ready manifests
- Honest telemetry: conditions follow the `meta.SetStatusCondition` convention — kubectl plugin-friendly

## Roadmap

- [ ] Admission webhook to soft-reject pods that would exceed the budget
- [ ] `--enforce=webhook` mode with cert-manager wiring
- [ ] Gatekeeper constraint templates equivalent

## License

MIT
