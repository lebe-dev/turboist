# Turboist

Multiple augementations for Todoist.

**Status:** work in progress

**Contents:**
- [Frontend architecture](docs/architecture/frontend.md)

## Features

- Contexts
- Day phases
- Planning mode
- Pinned tasks
- Increment ongoing tasks: Task (1) -> Task (2)
- Troiki System support ([en](docs/troiki-system.md), [ru](docs/troiki-system.ru.md))
- Locales: en, ru

## Nginx Configuration

When running behind nginx, add WebSocket proxy support:

```nginx
location /api/ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 86400s;
    proxy_send_timeout 86400s;
}

location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

## RoadMap

- Feature: constraints
- Offline-first
- iOS Native App
