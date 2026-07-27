# Blendkit-Client API

> Generated from `internal/apispec` by `cmd/apidocgen`. Do not edit by hand.

**Client version:** `1.12.2`

The Client is a local HTTP server (default port **62485**) that bridges Blendkit DCC add-ons (Blender, Godot, and embedders such as Maya and Rhino) with the Blendkit web service.

Most endpoints are registered twice: once under the bare path (e.g. `/report`) and once under a versioned prefix matching the running Client's major.minor version (e.g. `/vX.Y/report`). Both are equivalent; only the bare paths are documented below.

A machine-readable [OpenAPI 3.1 spec](openapi.json) is generated alongside this file. Import it into Postman/Insomnia, render it with Swagger UI/Redoc, or generate client SDKs from it.

## Endpoints by group

### core

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET | `/` |  |  | Client status page | — |
| POST | `/report` | ✓ |  | Poll for tasks (Blender) | `GetReportData` |
| POST | `/report_event` | ✓ |  | Report a telemetry event | `ReportEventData` |
| GET, POST | `/shutdown` | ✓ |  | Shut down the Client | — |
| GET | `/debug` | ✓ |  | Network/debug diagnostics | — |
| GET | `/dev` | ✓ |  | Developer test dashboard | — |

### settings

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET, POST | `/settings/get` | ✓ |  | Get Client settings | — |
| POST | `/settings/set` | ✓ |  | Change shared settings | `SetSettingsData` |
| POST | `/settings/set_variable` | ✓ |  | Store a variable | `SetVariableData` |
| GET | `/executable/list` | ✓ |  | List stored executables | — |
| GET | `/executable/get` | ✓ |  | Get stored executables | — |
| POST | `/executable/set` | ✓ |  | Store an executable | `SetExecutableData` |

### login

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET | `/consumer/exchange/` |  |  | OAuth2 redirect landing | — |
| POST | `/refresh_token` | ✓ |  | Refresh access token | `RefreshTokenData` |
| POST | `/oauth2/verification_data` | ✓ |  | Store OAuth2 PKCE session | `OAuth2VerificationData` |
| POST | `/oauth2/logout` | ✓ | 🔑 | Log out / revoke tokens | `RefreshTokenData` |

### assets

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/assets/search` | ✓ |  | Search assets | `SearchTaskData` |
| POST | `/assets/download` | ✓ |  | Download an asset | `DownloadData` |
| POST | `/assets/download_prxc` | ✓ |  | Download a proxy collection asset | `DownloadPrxcData` |
| POST | `/assets/upload` | ✓ | 🔑 | Upload an asset | `AssetUploadRequestData` |
| POST | `/assets/cancel_download` | ✓ |  | Cancel a download | `CancelDownloadData` |

### addons

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET, POST | `/addons/list` | ✓ |  | List subscribed add-ons | — |
| POST | `/addons/unsubscribe` | ✓ |  | Unsubscribe an add-on | `ReportData` |

### host-agnostic

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/run_blender_script` | ✓ |  | Run a Python recipe in headless Blender | `RunBlenderScriptData` |
| GET | `/tools/list` | ✓ |  | List bundled tools | — |
| POST | `/tools/run` | ✓ |  | Run a bundled tool | `RunBlenderScriptData` |

### profiles

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/profiles/download_gravatar_image` | ✓ |  | Download gravatar image | `FetchGravatarData` |
| POST | `/profiles/get_user_profile` | ✓ | 🔑 | Get user profile | `MinimalTaskData` |

### comments

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/comments/get_comments` | ✓ |  | Get asset comments | `GetCommentsData` |
| POST | `/comments/create_comment` | ✓ | 🔑 | Create a comment | `CreateCommentData` |
| POST | `/comments/feedback_comment` | ✓ | 🔑 | Like/dislike a comment | `FeedbackCommentTaskData` |
| POST | `/comments/mark_comment_private` | ✓ | 🔑 | Toggle comment privacy | `MarkCommentPrivateTaskData` |

### notifications

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/notifications/mark_notification_read` | ✓ | 🔑 | Mark notification read | `MarkNotificationReadTaskData` |

### ratings

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/ratings/get_bookmarks` | ✓ | 🔑 | Get bookmarks | `MinimalTaskData` |
| POST | `/ratings/get_rating` | ✓ | 🔑 | Get asset rating | `GetRatingData` |
| POST | `/ratings/send_rating` | ✓ | 🔑 | Send asset rating | `SendRatingData` |

### wrappers

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/wrappers/get_download_url` | ✓ |  | Resolve asset download URL | `DownloadData` |
| POST | `/wrappers/complete_upload_file_blocking` | ✓ | 🔑 | Complete a file upload (blocking) | `CompleteUploadFileBlockingData` |
| POST | `/wrappers/blocking_file_download` | ✓ |  | Download a file (blocking) | `BlockingFileDownloadTaskData` |
| POST | `/wrappers/blocking_request` | ✓ |  | Proxy a blocking HTTP request | `BlockingRequestData` |
| POST | `/wrappers/nonblocking_request` | ✓ |  | Proxy a non-blocking HTTP request | `NonblockingRequestTaskData` |

### bkclientjs

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET, OPTIONS | `/bkclientjs/status` | ✓ |  | Browser: Client status | — |
| POST, OPTIONS | `/bkclientjs/get_asset` | ✓ |  | Browser: request asset download | `bkclientjsDownloadData` |

### godot

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/godot/report` | ✓ |  | Poll for tasks (Godot) | `Software` |

### deprecated

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/blender/asset_search` | ✓ |  | Search assets (deprecated) | `SearchTaskData` |
| POST | `/blender/asset_download` | ✓ |  | Download an asset (deprecated) | `DownloadData` |
| POST | `/blender/asset_prxc_download` | ✓ |  | Download a proxy collection asset (deprecated) | `DownloadPrxcData` |
| POST | `/blender/asset_upload` | ✓ | 🔑 | Upload an asset (deprecated) | `AssetUploadRequestData` |
| POST | `/blender/cancel_download` | ✓ |  | Cancel a download (deprecated) | `CancelDownloadData` |
| POST | `/blender/unsubscribe_addon` | ✓ |  | Unsubscribe a Blender add-on (deprecated) | `ReportData` |
| POST | `/godot/unsubscribe_addon` | ✓ |  | Unsubscribe the Godot add-on (deprecated) | `ReportData` |

## Endpoint details

### core

#### `GET /`

Returns a small HTML page with the Client PID, version, platform, system ID and the add-on that started it. Any non-root path returns 404.

- **Handler:** `indexHandler`

#### `POST /report`

Primary polling endpoint for Blender add-ons. Subscribes the add-on on first call, refreshes the inactivity timer and returns the list of pending/finished/error tasks for the calling app. Rejects add-ons that do not send addon_version.

- **Handler:** `reportHandler`
- **Versioned alias:** `/vX.Y/report`
- **Request body:** JSON `GetReportData` (Go struct in package main)

#### `POST /report_event`

Fire-and-forget telemetry (e.g. login funnel events). The Client forwards the event to the server with standard headers in the background; failures are only logged, never surfaced to the UI.

- **Handler:** `ReportEventHandler`
- **Versioned alias:** `/vX.Y/report_event`
- **Request body:** JSON `ReportEventData` (Go struct in package main)

#### `GET / POST /shutdown`

Schedules a graceful exit of the Client process shortly after responding 200 OK.

- **Handler:** `shutdownHandler`
- **Versioned alias:** `/vX.Y/shutdown`

#### `GET /debug`

Returns diagnostic information about the Client's network configuration, useful for troubleshooting connectivity and proxy issues.

- **Handler:** `DebugNetworkHandler`
- **Versioned alias:** `/vX.Y/debug`

#### `GET /dev`

Serves a self-contained, same-origin HTML page with buttons to call the Client's endpoints and view their raw JSON responses. Because it is served by the Client itself, requests are same-origin and the settings endpoints (which emit no CORS headers) work directly from the browser. Manual-testing aid, not a production UI.

- **Handler:** `devDashboardHandler`
- **Versioned alias:** `/vX.Y/dev`

### settings

#### `GET / POST /settings/get`

Returns the current settings snapshot (shared settings, global and per-plugin variables) for the running Client version, together with a monotonically increasing revision. Plugins must sync to this: the same snapshot is also broadcast on every /report response, so plugins apply whenever the revision grows.

- **Handler:** `getSettingsHandler`
- **Versioned alias:** `/vX.Y/settings/get`

#### `POST /settings/set`

Applies a change to the shared settings (only the fields present in the body are modified), bumps the revision and returns the new snapshot. The change is broadcast to every connected plugin on their next /report poll.

- **Handler:** `setSettingsHandler`
- **Versioned alias:** `/vX.Y/settings/set`
- **Request body:** JSON `SetSettingsData` (Go struct in package main)

#### `POST /settings/set_variable`

Stores a free-form variable/value pair on behalf of a plugin. An empty 'plugin' stores it globally (without plugin association); a non-empty 'plugin' namespaces it under that plugin name (e.g. blender -> executable). Bumps the revision and returns the new snapshot; the change is broadcast on the next /report poll.

- **Handler:** `setVariableHandler`
- **Versioned alias:** `/vX.Y/settings/set_variable`
- **Request body:** JSON `SetVariableData` (Go struct in package main)

#### `GET /executable/list`

Returns all executables the Client keeps on behalf of plugins as name -> list of {path, version, args}. Several versions can be stored under one name (e.g. multiple Blender versions), highest version first. The same data also rides along on every /report via the settings snapshot; this endpoint is a direct query.

- **Handler:** `listExecutablesHandler`
- **Versioned alias:** `/vX.Y/executable/list`

#### `GET /executable/get`

Returns the stored executables for the 'name' query parameter (e.g. name=blender) as {"name":..., "executables":[...]}, highest version first. An optional 'version' query parameter filters to an exact match. An empty array means nothing is stored, so callers branch on presence without treating absence as an error.

- **Handler:** `getExecutableHandler`
- **Versioned alias:** `/vX.Y/executable/get`
- **Request notes:** Query parameters: name (executable key, e.g. 'blender'); version (optional exact version filter).

#### `POST /executable/set`

Registers or replaces a named executable (e.g. 'blender') with its path, version and optional default args. Multiple versions can coexist under one name — an entry with the same (name, version) is replaced, otherwise appended. Bumps the settings revision and broadcasts to every connected plugin on their next /report poll. Lets one plugin (e.g. the Blender add-on) publish a Blender path that other plugins (e.g. Maya) can reuse — including as the fallback for /tools/run and /run_blender_script.

- **Handler:** `setExecutableHandler`
- **Versioned alias:** `/vX.Y/executable/set`
- **Request body:** JSON `SetExecutableData` (Go struct in package main)

### login

#### `GET /consumer/exchange/`

Browser redirect target after the user logs in on blendkit.com. Validates the OAuth2 code and state query parameters, exchanges the code for tokens and redirects the browser to the server's oauth-landing page. Intentionally unversioned to keep the server-side redirect URL simple.

- **Handler:** `consumerExchangeHandler`
- **Request notes:** Query parameters: code (authorization code), state (CSRF/session state).

#### `POST /refresh_token`

Refreshes the access token using a refresh token. On success a 'login' task is delivered to every connected app; on failure the apps are logged out.

- **Handler:** `RefreshTokenHandler`
- **Versioned alias:** `/vX.Y/refresh_token`
- **Request body:** JSON `RefreshTokenData` (Go struct in package main)

#### `POST /oauth2/verification_data`

Stores the add-on's PKCE code_verifier and state so the later /consumer/exchange/ redirect can be verified.

- **Handler:** `OAuth2VerificationDataHandler`
- **Versioned alias:** `/vX.Y/oauth2/verification_data`
- **Request body:** JSON `OAuth2VerificationData` (Go struct in package main)

#### `POST /oauth2/logout`

Revokes the API key and refresh token on the server and logs the user out of all connected apps.

- **Handler:** `OAuth2LogoutHandler`
- **Versioned alias:** `/vX.Y/oauth2/logout`
- **Request body:** JSON `RefreshTokenData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

### assets

#### `POST /assets/search`

Starts an asynchronous asset search. Results, including thumbnail downloads, are reported back through /report tasks.

- **Handler:** `assetSearchHandler`
- **Versioned alias:** `/vX.Y/assets/search`
- **Request body:** JSON `SearchTaskData` (Go struct in package main)

#### `POST /assets/download`

Starts an asynchronous asset download. Progress and result are reported back through /report tasks.

- **Handler:** `assetDownloadHandler`
- **Versioned alias:** `/vX.Y/assets/download`
- **Request body:** JSON `DownloadData` (Go struct in package main)

#### `POST /assets/download_prxc`

Starts an asynchronous download of a proxy collection (prxc) asset.

- **Handler:** `assetPrxcDownloadHandler`
- **Versioned alias:** `/vX.Y/assets/download_prxc`
- **Request body:** JSON `DownloadPrxcData` (Go struct in package main)
- **Request notes:** Body embeds DownloadPrxcData plus an app_id field.

#### `POST /assets/upload`

Starts an asynchronous asset upload. Progress and result are reported back through /report tasks.

- **Handler:** `assetUploadHandler`
- **Versioned alias:** `/vX.Y/assets/upload`
- **Request body:** JSON `AssetUploadRequestData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /assets/cancel_download`

Cancels an in-progress asset download task.

- **Handler:** `CancelDownloadHandler`
- **Versioned alias:** `/vX.Y/assets/cancel_download`
- **Request body:** JSON `CancelDownloadData` (Go struct in package main)

### addons

#### `GET / POST /addons/list`

Returns the Client version and the list of currently subscribed softwares/plugins (Blender, Godot, embedders). Host-agnostic and not CORS-gated, unlike /bkclientjs/status.

- **Handler:** `listAddonsHandler`
- **Versioned alias:** `/vX.Y/addons/list`

#### `POST /addons/unsubscribe`

Cancels all running tasks for the calling app and removes it from the Client's task registry. Host-agnostic: works for any subscribed software (Blender, Godot, embedders).

- **Handler:** `unsubscribeAddonHandler`
- **Versioned alias:** `/vX.Y/addons/unsubscribe`
- **Request body:** JSON `ReportData` (Go struct in package main)

### host-agnostic

#### `POST /run_blender_script`

Runs a Python recipe under headless Blender. Used by external embedders (e.g. the Rhino plug-in) and available for the add-on's own background-script needs.

- **Handler:** `runBlenderScriptHandler`
- **Versioned alias:** `/vX.Y/run_blender_script`
- **Request body:** JSON `RunBlenderScriptData` (Go struct in package main)

#### `GET /tools/list`

Returns the recipes embedded in this Client binary, each with its optional parameter schema. Because the tools ship inside the binary, this list always matches exactly what /tools/run can execute. Plugins use it to discover available tools without hard-coding script IDs.

- **Handler:** `listToolsHandler`
- **Versioned alias:** `/vX.Y/tools/list`

#### `POST /tools/run`

Canonical alias for /run_blender_script: runs a bundled recipe (script_id from /tools/list) under headless Blender, forwarding params as params.json. New callers should prefer this endpoint.

- **Handler:** `runBlenderScriptHandler`
- **Versioned alias:** `/vX.Y/tools/run`
- **Request body:** JSON `RunBlenderScriptData` (Go struct in package main)

### profiles

#### `POST /profiles/download_gravatar_image`

Downloads a user's gravatar/avatar image into the Client temp directory.

- **Handler:** `DownloadGravatarImageHandler`
- **Versioned alias:** `/vX.Y/profiles/download_gravatar_image`
- **Request body:** JSON `FetchGravatarData` (Go struct in package main)

#### `POST /profiles/get_user_profile`

Fetches the logged-in user's Blendkit profile.

- **Handler:** `GetUserProfileHandler`
- **Versioned alias:** `/vX.Y/profiles/get_user_profile`
- **Request body:** JSON `MinimalTaskData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

### comments

#### `POST /comments/get_comments`

Fetches comments for an asset.

- **Handler:** `GetCommentsHandler`
- **Versioned alias:** `/vX.Y/comments/get_comments`
- **Request body:** JSON `GetCommentsData` (Go struct in package main)

#### `POST /comments/create_comment`

Posts a new comment (or reply) on an asset.

- **Handler:** `CreateCommentHandler`
- **Versioned alias:** `/vX.Y/comments/create_comment`
- **Request body:** JSON `CreateCommentData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /comments/feedback_comment`

Sends feedback (like or dislike) on a comment.

- **Handler:** `FeedbackCommentHandler`
- **Versioned alias:** `/vX.Y/comments/feedback_comment`
- **Request body:** JSON `FeedbackCommentTaskData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /comments/mark_comment_private`

Marks a comment as private or public.

- **Handler:** `MarkCommentPrivateHandler`
- **Versioned alias:** `/vX.Y/comments/mark_comment_private`
- **Request body:** JSON `MarkCommentPrivateTaskData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

### notifications

#### `POST /notifications/mark_notification_read`

Marks a server notification as read.

- **Handler:** `MarkNotificationReadHandler`
- **Versioned alias:** `/vX.Y/notifications/mark_notification_read`
- **Request body:** JSON `MarkNotificationReadTaskData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

### ratings

#### `POST /ratings/get_bookmarks`

Fetches the user's bookmarked assets.

- **Handler:** `GetBookmarksHandler`
- **Versioned alias:** `/vX.Y/ratings/get_bookmarks`
- **Request body:** JSON `MinimalTaskData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /ratings/get_rating`

Fetches the user's ratings for an asset.

- **Handler:** `GetRatingHandler`
- **Versioned alias:** `/vX.Y/ratings/get_rating`
- **Request body:** JSON `GetRatingData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /ratings/send_rating`

Submits a rating for an asset. Only POST is accepted.

- **Handler:** `SendRatingHandler`
- **Versioned alias:** `/vX.Y/ratings/send_rating`
- **Request body:** JSON `SendRatingData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

### wrappers

#### `POST /wrappers/get_download_url`

Blocking helper that resolves the direct download URL and filename for an asset file.

- **Handler:** `GetDownloadURLWrapper`
- **Versioned alias:** `/vX.Y/wrappers/get_download_url`
- **Request body:** JSON `DownloadData` (Go struct in package main)

#### `POST /wrappers/complete_upload_file_blocking`

Blocking helper that completes a multi-part file upload.

- **Handler:** `CompleteUploadFileBlocking`
- **Versioned alias:** `/vX.Y/wrappers/complete_upload_file_blocking`
- **Request body:** JSON `CompleteUploadFileBlockingData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /wrappers/blocking_file_download`

Blocking helper that downloads a file and returns once finished.

- **Handler:** `BlockingFileDownloadHandler`
- **Versioned alias:** `/vX.Y/wrappers/blocking_file_download`
- **Request body:** JSON `BlockingFileDownloadTaskData` (Go struct in package main)

#### `POST /wrappers/blocking_request`

Blocking helper that performs an arbitrary HTTP request to the Blendkit server through the Client's configured HTTP client and returns the response.

- **Handler:** `BlockingRequestHandler`
- **Versioned alias:** `/vX.Y/wrappers/blocking_request`
- **Request body:** JSON `BlockingRequestData` (Go struct in package main)

#### `POST /wrappers/nonblocking_request`

Schedules an arbitrary HTTP request to the Blendkit server; the response is reported back through /report tasks.

- **Handler:** `NonblockingRequestHandler`
- **Versioned alias:** `/vX.Y/wrappers/nonblocking_request`
- **Request body:** JSON `NonblockingRequestTaskData` (Go struct in package main)

### bkclientjs

#### `GET / OPTIONS /bkclientjs/status`

CORS-enabled endpoint used by bkclient.js in the web browser to read the Client version and the list of connected softwares. Supports OPTIONS preflight.

- **Handler:** `bkclientjsStatusHandler`
- **Versioned alias:** `/vX.Y/bkclientjs/status`

#### `POST / OPTIONS /bkclientjs/get_asset`

CORS-enabled endpoint used by bkclient.js to ask a connected software to download an asset. Supports OPTIONS preflight.

- **Handler:** `bkclientjsGetAssetHandler`
- **Versioned alias:** `/vX.Y/bkclientjs/get_asset`
- **Request body:** JSON `bkclientjsDownloadData` (Go struct in package main)

### godot

#### `POST /godot/report`

Polling endpoint for the Godot add-on. Subscribes the app on first call and returns a SoftwareResponse with the Client version, a connection message and pending tasks.

- **Handler:** `godotReportHandler`
- **Versioned alias:** `/vX.Y/godot/report`
- **Request body:** JSON `Software` (Go struct in package main)

### deprecated

#### `POST /blender/asset_search`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /assets/search. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `assetSearchHandler`
- **Versioned alias:** `/vX.Y/blender/asset_search`
- **Request body:** JSON `SearchTaskData` (Go struct in package main)

#### `POST /blender/asset_download`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /assets/download. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `assetDownloadHandler`
- **Versioned alias:** `/vX.Y/blender/asset_download`
- **Request body:** JSON `DownloadData` (Go struct in package main)

#### `POST /blender/asset_prxc_download`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /assets/download_prxc. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `assetPrxcDownloadHandler`
- **Versioned alias:** `/vX.Y/blender/asset_prxc_download`
- **Request body:** JSON `DownloadPrxcData` (Go struct in package main)
- **Request notes:** Body embeds DownloadPrxcData plus an app_id field.

#### `POST /blender/asset_upload`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /assets/upload. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `assetUploadHandler`
- **Versioned alias:** `/vX.Y/blender/asset_upload`
- **Request body:** JSON `AssetUploadRequestData` (Go struct in package main)
- **Auth:** requires a logged-in Blendkit API key

#### `POST /blender/cancel_download`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /assets/cancel_download. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `CancelDownloadHandler`
- **Versioned alias:** `/vX.Y/blender/cancel_download`
- **Request body:** JSON `CancelDownloadData` (Go struct in package main)

#### `POST /blender/unsubscribe_addon`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /addons/unsubscribe. Kept for backward compatibility with existing Blender add-ons.

- **Handler:** `blenderUnsubscribeAddonHandler`
- **Versioned alias:** `/vX.Y/blender/unsubscribe_addon`
- **Request body:** JSON `ReportData` (Go struct in package main)

#### `POST /godot/unsubscribe_addon`

> **Deprecated.** Kept for backward compatibility; use the universal endpoint noted below instead.

Deprecated alias of /addons/unsubscribe. Kept for backward compatibility with existing Godot add-ons.

- **Handler:** `godotUnsubscribeAddonHandler`
- **Versioned alias:** `/vX.Y/godot/unsubscribe_addon`
- **Request body:** JSON `ReportData` (Go struct in package main)

