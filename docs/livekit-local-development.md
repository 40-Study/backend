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
