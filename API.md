# Turboist API

The API is described by an OpenAPI 3.1 document. It is the single source of
truth for every endpoint, request and response schema, required auth and
API-token scope.

## Where to get the spec

- **From a running server:** `GET /api/v1/openapi.json`. Requires
  authentication — a JWT session access token or any valid API token (no
  particular scope needed). Anonymous requests get `401`.
- **From the repository:** [`api/openapi.yaml`](api/openapi.yaml). Generated
  from the route table and the Go DTO types by `just openapi`; never edit it by
  hand.

```sh
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/openapi.json"
```

API tokens are created in the web app settings (or `POST /api/v1/api-tokens`
with a JWT session) and are shown only once.

## Prompt for an AI agent

Give this to an agent that needs to work with a Turboist instance:

```text
You have access to a Turboist task manager API.
Base URL: <BASE_URL>
API token: <API_TOKEN>

Before calling any endpoint, download the OpenAPI spec:
  GET <BASE_URL>/api/v1/openapi.json
  Header: Authorization: Bearer <API_TOKEN>

Use it as the only reference for paths, parameters, request/response
schemas and required scopes. Send the same Authorization header on every
request. A 403 with "insufficient scope" means the token lacks the scope
listed for that operation in the spec — report it instead of retrying.
```
