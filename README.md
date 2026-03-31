# Adze

A small webhook receiver that keeps your Docker Compose services up to date.

When a container image is pushed to your registry, Adze catches the webhook, finds any Compose projects running that image, and pulls + redeploys them. No cron, no polling. Updates are queued and processed one at a time so nothing steps on anything else.

Works with Forgejo and Gitea container registry webhooks out of the box, or anything that can send a JSON body with an `image` field.

## Configuration

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | `ADDR` | `:8080` | Address to listen on |
| `-secret` | `SECRET` | (required) | Shared secret(s) for webhook signatures, comma-separated |
| `-danger-endpoints` | `DANGER_ENDPOINTS` | `0` | Number of unauthenticated webhook endpoints to generate |

If your registry requires authentication, mount `~/.docker` or set `DOCKER_CONFIG` to point to a directory containing your Docker config.

### Unauthenticated endpoints

If you have services that can't send signed webhooks, you can generate unauthenticated endpoints with `-danger-endpoints <n>` (or `DANGER_ENDPOINTS`). This creates `n` endpoints at `/webhook/<guid>` that accept requests without signature validation. The GUIDs are derived from your first configured secret, so they remain stable across restarts.

```yaml
environment:
  - SECRET=your-webhook-secret
  - DANGER_ENDPOINTS=2
```

The generated paths are logged at startup:

```
info: danger endpoint path=/webhook/a1b2c3d4-e5f6-7890-abcd-ef1234567890
info: danger endpoint path=/webhook/f9e8d7c6-b5a4-3210-fedc-ba0987654321
```

These endpoints are effectively passwords — treat the paths as secret.

## Running it

You need Docker with the Compose plugin available on the host. Adze needs access to the Docker socket and the filesystem containing your Compose files (it reads them directly, not through the Docker API).

### Docker

```yaml
# compose.yml
services:
  adze:
    image: ghcr.io/greboid/adze:latest
    environment:
      - ADDR=:8080
      - SECRET=your-webhook-secret
      - DOCKER_CONFIG=/docker
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /path/to/your/compose/files:/path/to/your/compose/files:ro
      - /home/youruser/.docker:/docker:ro
```
```
docker compose up -d
```

### Binary

```
go build -o adze .
./adze -addr :8080 -secret your-webhook-secret
```

Then point your registry webhook at `http://<host>:8080/webhook`.
