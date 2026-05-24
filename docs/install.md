# Installation

## Nginx

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}

# Server-Sent Events stream used for real-time data invalidation.
# Without these directives nginx buffers the response and closes idle
# connections after 60s, so the client reconnects every minute and the
# server-pushed events never reach the browser in time.
location /api/v1/events {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;

    proxy_http_version 1.1;
    proxy_set_header Connection "";

    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    proxy_read_timeout 1h;
}
```
