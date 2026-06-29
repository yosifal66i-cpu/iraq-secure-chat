# IraqSecureChat - منصة المراسلة الآمنة للجهات الحكومية

**A feature-complete, secure messaging platform for Iraqi government entities, built with military-grade encryption, AI-powered moderation, and enterprise scalability.**

## Architecture Overview

```
┌─────────────────────────────────────────────┐
│              API Gateway (Fiber)             │
│         Auth / Rate Limit / TLS 1.3          │
└──────────────┬──────────────────┬────────────┘
    ┌──────────▼────────┐  ┌─────▼──────────────┐
    │   Auth Service    │  │  Presence Service  │
    │  (JWT, OTP, 2FA)  │  │  (Online/Offline)  │
    └──────────┬────────┘  └────────┬───────────┘
    ┌──────────▼────────────────────▼───────────┐
    │         Message Service (Core)             │
    │  - Send/Receive/Deliver via Kafka          │
    │  - Fan-out to 200K members                 │
    │  - Read receipts, reactions, pins          │
    └──────────┬─────────────────────────────────┘
    ┌──────────▼────────┐  ┌────────────────────┐
    │  Media Service    │  │  WebSocket Gateway │
    │  (S3/MinIO, CDN)  │  │  (10M concurrent)  │
    └──────────┬────────┘  └────────┬───────────┘
    ┌──────────▼────────────────────▼───────────┐
    │         AI Service (Ollama/OpenAI)         │
    │  Moderation / Translation / Smart Replies  │
    └────────────────────────────────────────────┘
```

## Tech Stack

### Backend (Go)
- **Language:** Go 1.23
- **Framework:** Fiber v2
- **Real-time:** gorilla/websocket + Redis Pub/Sub
- **Message Queue:** Apache Kafka
- **Database:** PostgreSQL 16 + Cassandra/ScyllaDB
- **Cache:** Redis Cluster
- **Search:** Elasticsearch
- **Storage:** MinIO / S3-compatible
- **Monitoring:** Prometheus + Grafana + Loki + Jaeger

### Frontend (Web)
- **Framework:** React 18 + TypeScript
- **State:** Zustand + TanStack Query
- **Bundler:** Vite + PWA
- **Styling:** Tailwind CSS
- **Real-time:** WebSocket with auto-reconnect

### Mobile
- **Framework:** Flutter 3.4+ (Dart)
- **State:** Riverpod
- **Calls:** flutter_webrtc
- **Push:** Firebase + APNs

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.23+ (for local development)
- Node.js 20+ (for web client)
- Flutter 3.4+ (for mobile)

### Development (Docker)

```bash
# Clone and start all services
git clone https://github.com/your-org/iraq-secure-chat.git
cd iraq-secure-chat
docker-compose -f infra/docker-compose.yml up -d

# Access services
# Web Client: http://localhost:3000
# API: http://localhost:8090/v1
# Grafana: http://localhost:3100 (admin/admin)
```

### Local Development (Go services)

```bash
# Start dependencies
docker-compose -f infra/docker-compose.yml up -d postgres redis kafka minio

# Run a service (e.g., auth-service)
cd backend/auth-service
go run main.go
```

### Web Client Development

```bash
cd web
npm install
npm run dev  # http://localhost:3000
```

## API Documentation

### Authentication

```bash
# Send OTP
curl -X POST http://localhost:8090/v1/auth/send-otp \
  -H "Content-Type: application/json" \
  -d '{"phone": "9647XXXXXXXX"}'

# Verify OTP
curl -X POST http://localhost:8090/v1/auth/verify-otp \
  -H "Content-Type: application/json" \
  -d '{"phone": "9647XXXXXXXX", "otp": "123456", "device_info": "iPhone 15"}'

# Response
{
  "ok": true,
  "data": {
    "user_id": "uuid",
    "access_token": "jwt...",
    "refresh_token": "jwt...",
    "expires_in": 900,
    "is_new_user": false,
    "session_id": "uuid"
  }
}
```

### Messages

```bash
# Send message
curl -X POST http://localhost:8090/v1/chats/{chat_id}/messages \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{"type": "text", "text": "السلام عليكم"}'

# Get messages (cursor-based pagination)
curl http://localhost:8090/v1/chats/{chat_id}/messages?limit=50&cursor={cursor}
```

## Features

### Core Messaging
- [x] Private 1:1 chats with E2EE
- [x] Group chats (up to 200K members)
- [x] Broadcast channels (unlimited)
- [x] Message types: text, photo, video, audio, file, poll, location
- [x] Reply, forward, edit, delete
- [x] Reactions with emoji
- [x] Pinned messages
- [x] Read receipts & delivery status
- [x] Typing indicators
- [x] Online/offline presence

### AI-Powered Features
- [x] Content moderation (spam, NSFW, toxicity detection)
- [x] Smart reply suggestions
- [x] Real-time translation (Arabic, Kurdish, English)
- [x] Message summarization
- [x] AI text completion

### Security
- [x] End-to-end encryption (Signal Protocol)
- [x] Two-factor authentication (2FA)
- [x] Argon2id password hashing
- [x] JWT with refresh token rotation
- [x] Rate limiting (sliding window)
- [x] TLS 1.3 everywhere
- [x] Audit logging for compliance
- [x] Session management

### Enterprise
- [x] Multi-region deployment
- [x] Horizontal scaling (100+ nodes)
- [x] Monitoring & alerting
- [x] Structured logging
- [x] Distributed tracing
- [x] Prometheus metrics

## Security Compliance

IraqSecureChat implements security measures compliant with:
- **Iraqi National Security Standards**
- **ISO 27001** Information Security Management
- **GDPR** for European communications
- **NIST** cryptographic standards
- **OWASP** top 10 protection

### Encryption
- **In Transit:** TLS 1.3 with perfect forward secrecy
- **At Rest:** AES-256-GCM for all stored data
- **End-to-End:** Signal Protocol (X3DH + Double Ratchet)
- **Password Hashing:** Argon2id (memory-hard)

## Deployment Architecture

### Production (Kubernetes)

```bash
# Apply Kubernetes manifests
kubectl apply -f infra/k8s/ --namespace iraqchat

# Verify deployment
kubectl get pods -n iraqchat
kubectl get svc -n iraqchat
```

### Scaling Targets
- **10M** concurrent WebSocket connections
- **1B** messages/day (~11,600/sec)
- **100TB** media storage
- **99.99%** uptime (multi-region active-active)

## Monitoring

- **Metrics:** Prometheus (CPU, memory, request latency, message throughput)
- **Logs:** Loki (centralized log aggregation)
- **Tracing:** Jaeger (distributed request tracing)
- **Dashboards:** Grafana (pre-built monitoring dashboards)
- **Alerts:** Alertmanager (PagerDuty, Slack, Email)

## Bot API

IraqSecureChat supports a Telegram-compatible Bot API:

```python
# Send message as bot
POST /bot{token}/sendMessage
{
  "chat_id": "uuid",
  "text": "Hello from bot!",
  "parse_mode": "MarkdownV2"
}
```

## License

**Proprietary** - All rights reserved. This software is developed for Iraqi government entities.

---

**IraqSecureChat** - تواصل آمن، حماية كاملة، ثقة مطلقة
