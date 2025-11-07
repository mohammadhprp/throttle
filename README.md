# Throttle

Throttle is a Redis-backed, distributed rate limiter written in Go. It bundles multiple limiting strategies and exposes them through transports such as HTTP and gRPC, making it easy to drop into an existing microservice stack.

## Quick Start

1. Copy the default environment and adjust anything you need:

   ```bash
   cp .env.example .env
   ```

2. Build and start the stack (app + Redis) in the background:

   ```bash
   make build
   ```

3. Access at `http://localhost:3000`.
