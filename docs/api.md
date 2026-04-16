# HTTP API

When running `serve`, the daemon exposes:

- `GET /healthz`
- `GET /openapi.yaml`
- `GET /v1/timelines`
- `GET /v1/timelines/{profile}`

Response shapes:

- `GET /healthz` returns `{"status":"ok"}`
- `GET /v1/timelines` returns `{"profiles":{"<profile>":{"provider":"...","model":"...","model_vendor":"...","inference_provider":"...","entries":[TimelineEntry,...]}}}`
- `GET /v1/timelines/{profile}` returns `{"profile":"<profile>","provider":"...","model":"...","model_vendor":"...","inference_provider":"...","entries":[TimelineEntry,...]}`
- all endpoints include `Access-Control-Allow-Origin: *` for browser access
- timeline endpoints default to the latest 1024 entries and cap `limit` at 1024
- timeline endpoints support `model`, `model_vendor`, `inference_provider`, and `case_id` query filters

Each profile object includes:

- `provider`, `model`, `model_vendor`, `inference_provider`

Each `TimelineEntry` includes:

- run metadata: `run_id`, `timestamp`, `finished_at`, `case_id`, `profile`, `success`
- scores: `total_score`, `raw_score`, `max_score`, `normalized_score`
- automatic metrics: `read_file_calls`, `write_file_calls`, `list_dir_calls`, `read_write_ratio`, `pre_write_read_coverage`
- scoring details: `deductions`, `bonuses`

The API intentionally omits:

- `endpoint`
- `final_writes`
- `final_response`
- `bonuses[].description`
- `tool_calls`

Default listen address:

```text
127.0.0.1:8787
```

OpenAPI spec:

- [openapi.yaml](../internal/api/openapi.yaml)
