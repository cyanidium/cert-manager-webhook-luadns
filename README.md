# ACME webhook for luadns

The ACME issuer type supports an optional 'webhook' solver, which can be used
to implement custom DNS01 challenge solving logic. This is an implementation
for [luadns](https://www.luadns.com/).

## Building

Running `make build` will use podman/buildah to create the container image.

To install into a cluster with helm, first copy and edit the
charts/cert-manager-webhook-luadns/values.yaml` file, then run:

```shell
helm install cert-manager-webhook-luadns charts/cert-manager-webhook-luadns \
  -n cert-manager \
  -f values.yaml
```

## Running the test suite

You will need to [create an API token](https://app.luadns.com/users/api_keys)
before you can run the tests.

```bash
cp testdata/luadns-token.yaml.example testdata/luadns-token.yaml
```

Edit `testdata/luadns-token.yaml` to include the base64 encoded token

You can now run the test suite with:

```bash
TEST_ZONE_NAME=example.com. make test
```

> Make sure you change the domain to match the scope of your API key

## Credits

Based on the work from:

- [cert-manager webhook-example](https://github.com/cert-manager/webhook-example)
- [out-of-date luadns repo](https://github.com/luadns/certmanager-webhook-luadns)
- [work by adminios](https://github.com/adminios/certmanager-webhook-luadns)
- [a similar webhook for dnsimple](https://github.com/puzzle/cert-manager-webhook-dnsimple)
