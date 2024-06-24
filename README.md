# kubeimageswap

a kubernetes mutating webhook that automatically swap image based on predefined rules. It does not require features such as syncing or copying registries to a local registry.

## command line arguments

```bash
--filters  stringslices of jmespath query expression
--mapping  registries mapping, like 'docker.io=harbor.example.local/docker.io', NOTICED it will only matched image reference that starts with docker.io
```

## How to

```bash
# TODO: create an umbrella chart
helm -n operator-tools upgrade --install --create-namespace kubeimageswap -f charts/kubeimageswap.values.yaml charts/umbrella
```
