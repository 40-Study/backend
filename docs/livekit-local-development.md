# LiveKit local development

LiveKit and LiveKit Egress receive their configuration from Docker Compose environment variables. Secrets must stay in the ignored local `.env` file or in the deployment secret manager; do not add them to YAML files.

Required variables:

```dotenv
LIVEKIT_NODE_IP=127.0.0.1
LIVEKIT_API_KEY=<generate-a-key>
LIVEKIT_API_SECRET=<generate-a-secret>
REDIS_PASSWORD=<local-redis-password>
MINIO_ROOT_USER=<local-minio-user>
MINIO_ROOT_PASSWORD=<local-minio-password>
```

Validate the resolved Compose configuration without printing it:

```bash
docker compose config --quiet
```

The previous repository revision contained plaintext development credentials. Rotate those credentials anywhere they may have been reused before deploying this change.

## Browser QA on macOS / Docker Desktop

When the API runs directly on the Mac, set `LIVEKIT_NODE_IP=127.0.0.1` and `LIVEKIT_NODE_PORT=17880` in the local backend `.env`. `host.docker.internal` is intended for container-to-host traffic and did not resolve from the API process on the QA machine.

Start the local-only ICE override:

```bash
docker compose -f docker-compose.yaml -f docker-compose.livekit-local.yaml up -d livekit
```

The override advertises `127.0.0.1` and exposes ICE TCP 7881 / UDP 7882 on loopback, matching the ports that LiveKit sends to a browser on the same machine. The existing HTTP mapping remains 17880. This override is for a browser on the Docker host; remote students need the deployment's reachable host/ICE configuration.

Create the demo teacher and class with `go run ./cmd/seed-livestream`. In the web app, sign in as `teacher.live@demo.com` / `Demo@123`, open the event in Lịch giảng dạy, then choose **Bắt đầu và vào phòng**. Students `student1@demo.com` and `student2@demo.com` are members of the class. Their course lesson links to the same room. Re-running the seed preserves session state and does not reset existing accounts.
