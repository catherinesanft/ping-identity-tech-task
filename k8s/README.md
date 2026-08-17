# Deploying stockticker to minikube

## Quick start

```sh
# 1. Start cluster and enable ingress
minikube start --driver=docker
minikube addons enable ingress

# 2. Build and load the image (from repo root)
docker build -t stockticker:latest .
minikube image load stockticker:latest

# 3. Apply manifests (ConfigMap/Secret first, since Deployment needs them via envFrom)
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml

# 4. Verify
kubectl get pods
kubectl logs -l app=stockticker
```

## Reach the service

Simplest way in is a port-forward:

```sh
kubectl port-forward service/stockticker-service 8080:80

curl http://localhost:8080/
curl http://localhost:8080/healthz
```

If you'd rather go through the Ingress the way it'd actually be reached in
a real cluster:

```sh
echo "127.0.0.1 stockticker.local" | sudo tee -a /etc/hosts
minikube tunnel   # leave running in its own terminal, will prompt for sudo

curl http://stockticker.local/
curl http://stockticker.local/healthz
```

## Why it's set up this way

`imagePullPolicy: Never` because there's no registry involved in this
exercise — the cluster is never told to pull the image from anywhere.
That means the image has to be loaded into minikube's local store
(`minikube image load`) before the Deployment goes in, or pods sit in
`ImagePullBackOff`. A real environment would just pull from a registry
with a normal `IfNotPresent`/`Always` policy instead.

Single replica because the TTL cache is per-process and in-memory. Running
more than one replica wouldn't break anything, but each pod would hit
Alpha Vantage independently, which defeats the point of caching against a
quota-limited upstream. A shared cache like Redis would be the fix if this
ever needed to scale horizontally.

The Secret uses `stringData` purely for write-time convenience, so the API
key can be committed as plain text in the manifest rather than
hand-base64-encoded. Kubernetes still stores it base64-encoded at rest
(`kubectl get secret -o yaml` will show that) — that's encoding, not
encryption, so real secrets management (Vault, sealed-secrets,
external-secrets-operator, etc.) would replace this in production.

ClusterIP + Ingress rather than NodePort/LoadBalancer, to keep the Service
internal-only and route everything through a single Ingress controller —
closer to how this would actually be exposed in a real cluster than
relying on NodePort's ephemeral port range.

Both probes point at `/healthz` for simplicity here. A production setup
would likely split them — readiness checking whether the upstream API is
actually reachable, liveness checking only that the process itself is
alive — so a slow upstream doesn't pull healthy pods out of rotation.

## Troubleshooting

**`ImagePullBackOff` / `ErrImageNeverPull`** — image wasn't loaded into
minikube, or was loaded under a different tag:

```sh
docker build -t stockticker:latest .
minikube image load stockticker:latest
kubectl rollout restart deployment/stockticker
```

**Pod `Running` but never `Ready`** — usually means `APIKEY`/`SYMBOL`/`NDAYS`
isn't wired up correctly:

```sh
kubectl describe pod -l app=stockticker
kubectl logs -l app=stockticker
kubectl get configmap stockticker-config -o yaml
kubectl get secret stockticker-secret -o yaml
```

**`minikube tunnel` fails with `TUNNEL_ALREADY_RUNNING`** — usually a stale
lock file left over from a previous tunnel process:

```sh
rm ~/.minikube/tunnels.json
minikube tunnel
```
