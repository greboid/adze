# Adze

A small webhook receiver that keeps your Docker Compose services up to date.

When a container image is pushed to your registry, Adze catches the webhook, finds any Compose projects running that image, and pulls + redeploys them. No cron, no polling.

Works with Forgejo and Gitea container registry webhooks out of the box, or anything that can send a JSON body with an `image` field.

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
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /path/to/your/compose/files:/path/to/your/compose/files:ro
```
```
docker compose up -d
```

### Binary

```
go build -o adze .
./adze -addr :8080 -secret your-webhook-secret
```

Both flags can also be set via environment variables (`ADDR`, `SECRET`). You can pass multiple secrets separated by commas.

Then point your registry webhook at `http://<host>:8080/webhook`.

## How it works

1. A webhook arrives with an image name
2. Adze lists running containers to find Compose projects using that image
3. Each project gets a `docker compose up` with pull policy set to `always`
4. Old containers are cleaned up

Updates are queued and processed one at a time so nothing steps on anything else.
