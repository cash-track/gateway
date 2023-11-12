# Gateway

API gateway for backend services. Uses transparent HTTP layer transition from client requests to backend services.

## Run

```bash
$ make run
```

## Health Checks

- HTTP `GET [host]/live` for liveness check if service started
- HTTP `GET [host]/ready` for readiness check if all dependencies ok

## Push to registry

```bash
$ docker build . -t cashtrack/gateway:latest --no-cache
$ docker push cashtrack/gateway:latest
```
